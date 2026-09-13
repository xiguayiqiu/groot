package rootless

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"groot/internal/logger"
)

const (
	RootlessKitStateDir = "/tmp/groot-rootlesskit"
	PipeFDEnvKey        = "ROOTLESSKIT_PIPE_FD"
	ChildUseActivation  = "ROOTLESSKIT_CHILD_USE_ACTIVATION"
	StateDirEnvKey      = "ROOTLESSKIT_STATE_DIR"
	NetDriverEnvKey     = "ROOTLESSKIT_NET_DRIVER"
	ChildIPEnvKey       = "ROOTLESSKIT_CHILD_IP"
	ChildDevEnvKey      = "ROOTLESSKIT_CHILD_DEV"
)

type RootlessConfig struct {
	RootfsPath    string
	Shell         string
	Command       string
	Hostname      string
	WorkDir       string
	NetMode       string
	Readonly      bool
	EnableOverlay bool
	EnableSeccomp bool
	CPUQuota      int64
	MemoryLimit   int64
	PidsLimit     int64
}

type rootlessContext struct {
	config     *RootlessConfig
	stateDir   string
	childPID   int
	cleanupFns []func()
}

func NewRootlessContext(cfg *RootlessConfig) *rootlessContext {
	return &rootlessContext{
		config:   cfg,
		stateDir: filepath.Join(RootlessKitStateDir, fmt.Sprintf("groot-%d", os.Getpid())),
	}
}

func (rc *rootlessContext) cleanup() {
	for i := len(rc.cleanupFns) - 1; i >= 0; i-- {
		rc.cleanupFns[i]()
	}
}

func (rc *rootlessContext) addCleanup(fn func()) {
	rc.cleanupFns = append(rc.cleanupFns, fn)
}

func (rc *rootlessContext) SetupStateDir() error {
	if err := os.MkdirAll(rc.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state dir %s: %w", rc.stateDir, err)
	}
	return nil
}

func IsRootlessSupported() bool {
	if os.Geteuid() == 0 {
		return true
	}
	if _, err := os.Stat("/proc/self/ns/user"); err != nil {
		return false
	}
	return true
}

func RunRootless(cfg *RootlessConfig) error {
	if os.Geteuid() == 0 {
		return runRootlessAsRoot(cfg)
	}
	return runRootlessUnprivileged(cfg)
}

func runRootlessAsRoot(cfg *RootlessConfig) error {
	ctx := NewRootlessContext(cfg)
	defer ctx.cleanup()

	if err := ctx.SetupStateDir(); err != nil {
		return err
	}

	cloneFlags := uintptr(unix.CLONE_NEWNS | unix.CLONE_NEWPID | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC)
	if cfg.NetMode == "nat" {
		cloneFlags |= unix.CLONE_NEWNET
	}

	envVars := buildEnv(cfg)
	mounts := buildRootfsMounts(cfg)

	return forkAndExec(cfg, cloneFlags, mounts, envVars)
}

func runRootlessUnprivileged(cfg *RootlessConfig) error {
	ctx := NewRootlessContext(cfg)
	defer ctx.cleanup()

	if err := ctx.SetupStateDir(); err != nil {
		return err
	}

	hasSubid, err := checkSubidConfig()
	if err != nil || !hasSubid {
		logger.Warn("subuid/subgid not configured, falling back to single UID mapping")
		return runRootlessFallback(cfg)
	}

	return runRootlessWithRootlessKit(cfg)
}

func runRootlessFallback(cfg *RootlessConfig) error {
	cloneFlags := uintptr(unix.CLONE_NEWNS | unix.CLONE_NEWPID | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC | unix.CLONE_NEWUSER)

	euid := os.Geteuid()
	egid := os.Getegid()
	uidMappings := []syscall.SysProcIDMap{{ContainerID: 0, HostID: euid, Size: 1}}
	gidMappings := []syscall.SysProcIDMap{{ContainerID: 0, HostID: egid, Size: 1}}

	if cfg.NetMode == "nat" {
		cloneFlags |= unix.CLONE_NEWNET
	}

	envVars := buildEnv(cfg)
	mounts := buildRootfsMounts(cfg)

	return forkWithMappings(cfg, cloneFlags, mounts, envVars, uidMappings, gidMappings, false)
}

func runRootlessWithRootlessKit(cfg *RootlessConfig) error {
	stateDir := filepath.Join(RootlessKitStateDir, fmt.Sprintf("rk-%d", os.Getpid()))
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create rootlesskit state dir: %w", err)
	}

	args := []string{"--state-dir", stateDir}

	if cfg.NetMode == "nat" {
		args = append(args, "--net=slirp4netns")
	} else {
		args = append(args, "--net=host")
	}

	args = append(args, "--")

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	childArgs := []string{"rootless-child"}
	if cfg.Command != "" {
		childArgs = append(childArgs, "--command", cfg.Command)
	}
	if cfg.Shell != "" {
		childArgs = append(childArgs, "--shell", cfg.Shell)
	}
	childArgs = append(childArgs, cfg.RootfsPath)

	cmd := exec.Command(exePath, childArgs...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		"ROOTLESS_ROOTFS="+cfg.RootfsPath,
		"ROOTLESS_HOSTNAME="+cfg.Hostname,
		"ROOTLESS_NET_MODE="+cfg.NetMode,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info("running via rootlesskit: state-dir=%s", stateDir)
	return cmd.Run()
}

func checkSubidConfig() (bool, error) {
	u, err := user.Current()
	if err != nil {
		return false, err
	}

	subuidContent, err := os.ReadFile("/etc/subuid")
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(subuidContent), "\n")
	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) >= 3 && parts[0] == u.Username {
			return true, nil
		}
	}
	return false, nil
}

func setupUIDGIDMappings(pid int) error {
	u, err := user.Current()
	if err != nil {
		return err
	}

	subuidRanges, err := getSubIDRanges(u.Uid)
	if err != nil {
		return fmt.Errorf("failed to get subuid ranges: %w", err)
	}

	subgidRanges, err := getSubIDRanges(u.Gid)
	if err != nil {
		return fmt.Errorf("failed to get subgid ranges: %w", err)
	}

	uidArgs := buildUIDMapArgs(u.Uid, subuidRanges)
	gidArgs := buildGIDMapArgs(u.Gid, subgidRanges)

	pidStr := strconv.Itoa(pid)

	if out, err := exec.Command("newuidmap", append([]string{pidStr}, uidArgs...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("newuidmap failed: %s: %w", string(out), err)
	}

	if out, err := exec.Command("newgidmap", append([]string{pidStr}, gidArgs...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("newgidmap failed: %s: %w", string(out), err)
	}

	return nil
}

func getSubIDRanges(id string) ([][2]int, error) {
	content, err := os.ReadFile("/etc/subuid")
	if err != nil {
		return nil, err
	}

	var ranges [][2]int
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) >= 3 && parts[0] == id {
			start, _ := strconv.Atoi(parts[1])
			length, _ := strconv.Atoi(parts[2])
			if start > 0 && length > 0 {
				ranges = append(ranges, [2]int{start, length})
			}
		}
	}
	return ranges, nil
}

func buildUIDMapArgs(ownerUID string, ranges [][2]int) []string {
	args := []string{"0", ownerUID, "1"}
	lastID := 1
	for _, r := range ranges {
		args = append(args, strconv.Itoa(lastID), strconv.Itoa(r[0]), strconv.Itoa(r[1]))
		lastID += r[1]
	}
	return args
}

func buildGIDMapArgs(ownerGID string, ranges [][2]int) []string {
	args := []string{"0", ownerGID, "1"}
	lastID := 1
	for _, r := range ranges {
		args = append(args, strconv.Itoa(lastID), strconv.Itoa(r[0]), strconv.Itoa(r[1]))
		lastID += r[1]
	}
	return args
}

func forkWithMappings(cfg *RootlessConfig, cloneFlags uintptr, mounts []MountSpec, envVars []string, uidMappings, gidMappings []syscall.SysProcIDMap, setgroups bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exePath, "rootless-child")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                cloneFlags,
		UidMappings:               uidMappings,
		GidMappings:               gidMappings,
		GidMappingsEnableSetgroups: setgroups,
	}

	serializedEnv := serializeEnv(cfg, mounts, envVars)
	cmd.Env = append(os.Environ(), serializedEnv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func forkAndExec(cfg *RootlessConfig, cloneFlags uintptr, mounts []MountSpec, envVars []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exePath, "rootless-child")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneFlags,
	}

	serializedEnv := serializeEnv(cfg, mounts, envVars)
	cmd.Env = append(os.Environ(), serializedEnv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func RunChild() error {
	cfg := deserializeConfig()
	if cfg == nil {
		return fmt.Errorf("failed to deserialize rootless child config")
	}

	logger.Info("rootless child: rootfs=%s", cfg.RootfsPath)

	if err := applyMounts(cfg); err != nil {
		return fmt.Errorf("failed to setup mounts: %w", err)
	}

	hostname := cfg.Hostname
	if hostname == "" {
		hostname = "groot"
	}
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		logger.Debug("sethostname failed: %v", err)
	}

	unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "/"
	}
	os.Chdir(workDir)

	args := []string{cfg.Shell}
	if cfg.Command != "" {
		args = []string{cfg.Shell, "-c", cfg.Command}
	} else {
		args = []string{cfg.Shell, "-l"}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = workDir

	return cmd.Run()
}

func buildEnv(cfg *RootlessConfig) []string {
	return []string{
		"HOME=/root",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=" + os.Getenv("TERM"),
		"LANG=en_US.UTF-8",
	}
}

func buildRootfsMounts(cfg *RootlessConfig) []MountSpec {
	var mounts []MountSpec
	rootfs := cfg.RootfsPath

	mounts = append(mounts, MountSpec{
		Source:  rootfs,
		Target:  "/",
		FsType:  "",
		Flags:   unix.MS_BIND | unix.MS_REC | unix.MS_NOSUID,
		Options: "rw",
	})

	mounts = append(mounts,
		MountSpec{Source: "proc", Target: "/proc", FsType: "proc", Flags: unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV},
		MountSpec{Source: "sysfs", Target: "/sys", FsType: "sysfs", Flags: unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV},
		MountSpec{Source: "tmpfs", Target: "/dev/shm", FsType: "tmpfs", Flags: unix.MS_NOSUID | unix.MS_NODEV, Options: "mode=1777"},
		MountSpec{Source: "tmpfs", Target: "/tmp", FsType: "tmpfs", Flags: unix.MS_NOSUID | unix.MS_NODEV, Options: "mode=1777"},
		MountSpec{Source: "tmpfs", Target: "/run", FsType: "tmpfs", Flags: unix.MS_NOSUID | unix.MS_NODEV, Options: "mode=755"},
	)

	return mounts
}

type MountSpec struct {
	Source  string
	Target  string
	FsType  string
	Flags   uintptr
	Options string
}

func serializeEnv(cfg *RootlessConfig, mounts []MountSpec, envVars []string) []string {
	var env []string

	env = append(env, "GROOT_MODE=rootless")
	env = append(env, "GROOT_ROOTFS="+cfg.RootfsPath)
	env = append(env, "GROOT_SHELL="+cfg.Shell)
	env = append(env, "GROOT_COMMAND="+cfg.Command)
	env = append(env, "GROOT_HOSTNAME="+cfg.Hostname)
	env = append(env, "GROOT_WORKDIR="+cfg.WorkDir)
	env = append(env, "GROOT_NET_MODE="+cfg.NetMode)
	env = append(env, fmt.Sprintf("GROOT_READONLY=%v", cfg.Readonly))
	env = append(env, fmt.Sprintf("GROOT_OVERLAY=%v", cfg.EnableOverlay))
	env = append(env, fmt.Sprintf("GROOT_SECCOMP=%v", cfg.EnableSeccomp))
	env = append(env, fmt.Sprintf("GROOT_CPU_QUOTA=%d", cfg.CPUQuota))
	env = append(env, fmt.Sprintf("GROOT_MEMORY=%d", cfg.MemoryLimit))
	env = append(env, fmt.Sprintf("GROOT_PIDS=%d", cfg.PidsLimit))
	env = append(env, fmt.Sprintf("GROOT_MOUNT_COUNT=%d", len(mounts)))

	for i, m := range mounts {
		env = append(env, fmt.Sprintf("GROOT_MOUNT_%d=%s|%s|%s|%d|%s",
			i, m.Source, m.Target, m.FsType, m.Flags, m.Options))
	}

	for _, e := range envVars {
		env = append(env, e)
	}

	return env
}

func deserializeConfig() *RootlessConfig {
	cfg := &RootlessConfig{
		RootfsPath:    os.Getenv("GROOT_ROOTFS"),
		Shell:         os.Getenv("GROOT_SHELL"),
		Command:       os.Getenv("GROOT_COMMAND"),
		Hostname:      os.Getenv("GROOT_HOSTNAME"),
		WorkDir:       os.Getenv("GROOT_WORKDIR"),
		NetMode:       os.Getenv("GROOT_NET_MODE"),
		EnableOverlay: os.Getenv("GROOT_OVERLAY") == "true",
		EnableSeccomp: os.Getenv("GROOT_SECCOMP") == "true",
	}

	fmt.Sscanf(os.Getenv("GROOT_READONLY"), "%v", &cfg.Readonly)
	fmt.Sscanf(os.Getenv("GROOT_CPU_QUOTA"), "%d", &cfg.CPUQuota)
	fmt.Sscanf(os.Getenv("GROOT_MEMORY"), "%d", &cfg.MemoryLimit)
	fmt.Sscanf(os.Getenv("GROOT_PIDS"), "%d", &cfg.PidsLimit)

	return cfg
}

func applyMounts(cfg *RootlessConfig) error {
	mountCount := 0
	fmt.Sscanf(os.Getenv("GROOT_MOUNT_COUNT"), "%d", &mountCount)

	for i := 0; i < mountCount; i++ {
		mountStr := os.Getenv(fmt.Sprintf("GROOT_MOUNT_%d", i))
		parts := strings.Split(mountStr, "|")
		if len(parts) < 5 {
			continue
		}

		source := parts[0]
		target := parts[1]
		fsType := parts[2]
		var flags uintptr
		fmt.Sscanf(parts[3], "%d", &flags)
		options := parts[4]

		targetDir := filepath.Join(cfg.RootfsPath, strings.TrimPrefix(target, "/"))
		if target == "/" {
			targetDir = cfg.RootfsPath
		}

		if fsType == "" {
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				logger.Debug("mkdir %s failed: %v", targetDir, err)
				continue
			}
			if err := unix.Mount(source, targetDir, "", flags, ""); err != nil {
				logger.Debug("bind mount %s -> %s failed: %v", source, targetDir, err)
				continue
			}
		} else {
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				logger.Debug("mkdir %s failed: %v", targetDir, err)
				continue
			}
			if err := unix.Mount(source, targetDir, fsType, flags, options); err != nil {
				logger.Debug("mount %s (%s) -> %s failed: %v", source, fsType, targetDir, err)
				continue
			}
		}

		logger.Debug("mounted: %s -> %s (%s)", source, target, fsType)
	}

	return nil
}
