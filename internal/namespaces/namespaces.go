package namespaces

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"github.com/moby/sys/mountinfo"
)

const (
	NetModeNat  = "nat"
	NetModeHost = "host"
)

type Mount struct {
	Source, Target, FsType, Data string
	Flags uintptr
}

type SyscallParams struct {
	Source, Target, FsType, Data *byte
	Flags uintptr
	Prefixes []*byte
	MakeNod bool
}

type Builder struct {
	Mounts []Mount
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithMount(m Mount) *Builder {
	b.Mounts = append(b.Mounts, m)
	return b
}

func (b *Builder) Build() ([]SyscallParams, error) {
	var params []SyscallParams
	for _, m := range b.Mounts {
		p, err := m.toSyscall()
		if err != nil {
			return nil, err
		}
		params = append(params, *p)
	}
	return params, nil
}

func (m *Mount) toSyscall() (*SyscallParams, error) {
	var data *byte
	source, err := syscall.BytePtrFromString(m.Source)
	if err != nil {
		return nil, err
	}
	target, err := syscall.BytePtrFromString(m.Target)
	if err != nil {
		return nil, err
	}
	fsType, err := syscall.BytePtrFromString(m.FsType)
	if err != nil {
		return nil, err
	}
	if m.Data != "" {
		data, err = syscall.BytePtrFromString(m.Data)
		if err != nil {
			return nil, err
		}
	}
	prefix := pathPrefix(m.Target)
	paths, err := arrayPtrFromStrings(prefix)
	if err != nil {
		return nil, err
	}
	return &SyscallParams{
		Source:   source,
		Target:   target,
		FsType:   fsType,
		Flags:    m.Flags,
		Data:     data,
		Prefixes: paths,
	}, nil
}

func pathPrefix(path string) []string {
	ret := make([]string, 0)
	for i := 1; i < len(path); i++ {
		if path[i] == '/' {
			ret = append(ret, path[:i])
		}
	}
	ret = append(ret, path)
	return ret
}

func arrayPtrFromStrings(str []string) ([]*byte, error) {
	bytes := make([]*byte, 0, len(str))
	for _, s := range str {
		b, err := syscall.BytePtrFromString(s)
		if err != nil {
			return nil, err
		}
		bytes = append(bytes, b)
	}
	return bytes, nil
}

type RLimit struct {
	Res int
	Rlim syscall.Rlimit
}

type RLimits struct {
	DisableCore bool
	CPU         uint64
	Data        uint64
	Memory      uint64
	OpenFile    uint64
}

func (r *RLimits) PrepareRLimit() []RLimit {
	var ret []RLimit
	if r.CPU > 0 {
		cpuHard := r.CPU
		if cpuHard < r.CPU || r.CPU == 0 {
			cpuHard = r.CPU
		}
		ret = append(ret, RLimit{
			Res:  syscall.RLIMIT_CPU,
			Rlim: syscall.Rlimit{Cur: uint64(r.CPU), Max: uint64(cpuHard)},
		})
	}
	if r.Data > 0 {
		ret = append(ret, RLimit{
			Res:  syscall.RLIMIT_DATA,
			Rlim: syscall.Rlimit{Cur: uint64(r.Data), Max: uint64(r.Data)},
		})
	}
	if r.Memory > 0 {
		ret = append(ret, RLimit{
			Res:  syscall.RLIMIT_AS,
			Rlim: syscall.Rlimit{Cur: uint64(r.Memory << 20), Max: uint64(r.Memory << 20)},
		})
	}
	if r.OpenFile > 0 {
		ret = append(ret, RLimit{
			Res:  syscall.RLIMIT_NOFILE,
			Rlim: syscall.Rlimit{Cur: uint64(r.OpenFile), Max: uint64(r.OpenFile)},
		})
	}
	if r.DisableCore {
		ret = append(ret, RLimit{
			Res:  syscall.RLIMIT_CORE,
			Rlim: syscall.Rlimit{Cur: 0, Max: 0},
		})
	}
	return ret
}

type Status int

const (
	StatusInvalid Status = iota
	StatusNormal
	StatusTimeLimitExceeded
	StatusMemoryLimitExceeded
	StatusOutputLimitExceeded
	StatusDisallowedSyscall
	StatusSignalled
	StatusNonzeroExitStatus
	StatusRunnerError
)

func (s Status) String() string {
	names := []string{
		"Invalid", "", "Time Limit Exceeded", "Memory Limit Exceeded",
		"Output Limit Exceeded", "Disallowed Syscall", "Signalled",
		"Nonzero Exit Status", "Runner Error",
	}
	if int(s) >= 0 && int(s) < len(names) {
		return names[int(s)]
	}
	return "Invalid"
}

func (s Status) Error() string {
	return s.String()
}

type Result struct {
	Status
	ExitStatus int
	Error      string
	Time       time.Duration
	Memory     uint64
	SetUpTime  time.Duration
	RunningTime time.Duration
}

func (r Result) String() string {
	switch r.Status {
	case StatusNormal:
		return fmt.Sprintf("Result[%v %v][%v %v]", r.Time, r.Memory, r.SetUpTime, r.RunningTime)
	case StatusSignalled:
		return fmt.Sprintf("Result[Signalled(%d)][%v %v][%v %v]", r.ExitStatus, r.Time, r.Memory, r.SetUpTime, r.RunningTime)
	case StatusRunnerError:
		return fmt.Sprintf("Result[RunnerFailed(%s)][%v %v][%v %v]", r.Error, r.Time, r.Memory, r.SetUpTime, r.RunningTime)
	default:
		return fmt.Sprintf("Result[%v(%s %d)][%v %v][%v %v]", r.Status, r.Error, r.ExitStatus, r.Time, r.Memory, r.SetUpTime, r.RunningTime)
	}
}

type SandboxRunner struct {
	Args                       []string
	Env                        []string
	RLimits                    []RLimit
	Mounts                     []SyscallParams
	WorkDir                    string
	HostName                   string
	DomainName                 string
	CloneFlags                 uintptr
	UIDMappings                []syscall.SysProcIDMap
	GIDMappings                []syscall.SysProcIDMap
	GIDMappingsEnableSetgroups bool
	SyncFunc                   func(pid int) error
	CTTY                       bool
	DropCaps                   bool
	NoNewPrivs                 bool
}

func (sr *SandboxRunner) Run(c context.Context) Result {
	return runSandbox(c, *sr)
}

func runSandbox(c context.Context, sr SandboxRunner) Result {
	result := Result{Status: StatusNormal}
	sTime := time.Now()
	fTime := time.Now()

	pid, err := forkSandbox(sr)
	if err != nil {
		return Result{Status: StatusRunnerError, Error: err.Error()}
	}

	var wstatus unix.WaitStatus
	var rusage unix.Rusage

	ctx, cancel := context.WithCancel(c)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigChan)
		close(sigChan)
	}()

	go func() {
		select {
		case <-ctx.Done():
		case <-sigChan:
		}
		unix.Kill(-pid, unix.SIGKILL)
	}()

	defer func() {
		unix.Kill(-pid, unix.SIGKILL)
		for {
			_, err := unix.Wait4(-pid, &wstatus, unix.WALL|unix.WNOHANG, nil)
			if err != unix.EINTR && err != nil {
				break
			}
		}
		if wstatus.Exited() {
			result.ExitStatus = wstatus.ExitStatus()
			if result.ExitStatus != 0 {
				result.Status = StatusNonzeroExitStatus
			}
		} else if wstatus.Signaled() {
			sig := wstatus.Signal()
			result.ExitStatus = int(sig)
			switch sig {
			case unix.SIGXCPU, unix.SIGKILL:
				result.Status = StatusTimeLimitExceeded
			case unix.SIGXFSZ:
				result.Status = StatusOutputLimitExceeded
			case unix.SIGSYS:
				result.Status = StatusDisallowedSyscall
			default:
				result.Status = StatusSignalled
			}
		}
	}()

	for {
		_, err := unix.Wait4(pid, &wstatus, 0, &rusage)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			result.Status = StatusRunnerError
			result.Error = err.Error()
			return result
		}
		if wstatus.Exited() {
			result.Status = StatusNormal
			result.ExitStatus = wstatus.ExitStatus()
			if result.ExitStatus != 0 {
				result.Status = StatusNonzeroExitStatus
			}
		} else if wstatus.Signaled() {
			sig := wstatus.Signal()
			result.ExitStatus = int(sig)
			switch sig {
			case unix.SIGXCPU, unix.SIGKILL:
				result.Status = StatusTimeLimitExceeded
			case unix.SIGXFSZ:
				result.Status = StatusOutputLimitExceeded
			case unix.SIGSYS:
				result.Status = StatusDisallowedSyscall
			default:
				result.Status = StatusSignalled
			}
		}
		result.Time = time.Duration(rusage.Utime.Nano())
		result.Memory = uint64(rusage.Maxrss << 10)
		result.SetUpTime = fTime.Sub(sTime)
		result.RunningTime = time.Since(fTime)
		return result
	}
}

func forkSandbox(sr SandboxRunner) (int, error) {
	exePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("get executable path failed: %w", err)
	}

	env := append([]string{}, sr.Env...)

	env = append(env, "GROOT_ARGS="+strings.Join(sr.Args, "|#|"))
	env = append(env, "GROOT_ENV="+strings.Join(sr.Env, "|#|"))
	env = append(env, "GROOT_WORKDIR="+sr.WorkDir)
	env = append(env, "GROOT_HOSTNAME="+sr.HostName)
	env = append(env, "GROOT_DOMAINNAME="+sr.DomainName)
	env = append(env, "GROOT_CLONEFLAGS="+strconv.FormatUint(uint64(sr.CloneFlags), 10))
	env = append(env, "GROOT_CTTY="+strconv.FormatUint(uint64(boolToInt(sr.CTTY)), 10))
	env = append(env, "GROOT_DROPCAPS="+strconv.FormatUint(uint64(boolToInt(sr.DropCaps)), 10))
	env = append(env, "GROOT_NONEWPRIVS="+strconv.FormatUint(uint64(boolToInt(sr.NoNewPrivs)), 10))
	env = append(env, "GROOT_UIDMAP_COUNT="+strconv.Itoa(len(sr.UIDMappings)))
	env = append(env, "GROOT_GIDMAP_COUNT="+strconv.Itoa(len(sr.GIDMappings)))
	env = append(env, "GROOT_SETGROUPS="+strconv.FormatUint(uint64(boolToInt(sr.GIDMappingsEnableSetgroups)), 10))

	for i, m := range sr.UIDMappings {
		env = append(env, fmt.Sprintf("GROOT_UIDMAP_%d=%d %d %d", i, m.ContainerID, m.HostID, m.Size))
	}
	for i, m := range sr.GIDMappings {
		env = append(env, fmt.Sprintf("GROOT_GIDMAP_%d=%d %d %d", i, m.ContainerID, m.HostID, m.Size))
	}

	for i, mp := range sr.Mounts {
		source := ""
		if mp.Source != nil {
			source = cStringToString(mp.Source)
		}
		target := ""
		if mp.Target != nil {
			target = cStringToString(mp.Target)
		}
		fsType := ""
		if mp.FsType != nil {
			fsType = cStringToString(mp.FsType)
		}
		data := ""
		if mp.Data != nil {
			data = cStringToString(mp.Data)
		}
		env = append(env, fmt.Sprintf("GROOT_MOUNT_%d=%s|#|%s|#|%s|#|%d|#|%s", i, source, target, fsType, int(mp.Flags), data))
	}
	env = append(env, "GROOT_MOUNT_COUNT="+strconv.Itoa(len(sr.Mounts)))

	if len(sr.RLimits) > 0 {
		for i, rl := range sr.RLimits {
			env = append(env, fmt.Sprintf("GROOT_RLIMIT_%d=%d %d %d", i, rl.Res, rl.Rlim.Cur, rl.Rlim.Max))
		}
		env = append(env, "GROOT_RLIMIT_COUNT="+strconv.Itoa(len(sr.RLimits)))
	}

	cmdArgs := []string{"spaces-child"}
	for _, arg := range sr.Args {
		cmdArgs = append(cmdArgs, arg)
	}

	cmd := exec.Command(exePath, cmdArgs...)
	cmd.Env = env
	cmd.Dir = sr.WorkDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	var cloneFlags uintptr = sr.CloneFlags | uintptr(syscall.SIGCHLD)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneFlags,
	}
	if len(sr.UIDMappings) > 0 {
		cmd.SysProcAttr.UidMappings = sr.UIDMappings
	}
	if len(sr.GIDMappings) > 0 {
		cmd.SysProcAttr.GidMappings = sr.GIDMappings
	}
	cmd.SysProcAttr.GidMappingsEnableSetgroups = sr.GIDMappingsEnableSetgroups

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	return cmd.Process.Pid, nil
}

func cStringToString(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	p := unsafe.Pointer(ptr)
	n := 0
	for *(*byte)(unsafe.Pointer(uintptr(p) + uintptr(n))) != 0 {
		n++
	}
	s := unsafe.Slice((*byte)(p), n)
	return string(s)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func SetupMountsInChild(mounts []SyscallParams) error {
	for _, mp := range mounts {
		source := cStringToString(mp.Source)
		target := cStringToString(mp.Target)
		fsType := cStringToString(mp.FsType)
		data := ""
		if mp.Data != nil {
			data = cStringToString(mp.Data)
		}
		if err := syscall.Mount(source, target, fsType, mp.Flags, data); err != nil {
			return err
		}
	}
	return nil
}

func SetupRlimitsInChild(rlimits []RLimit) error {
	for _, rl := range rlimits {
		if err := syscall.Setrlimit(rl.Res, &rl.Rlim); err != nil {
			return err
		}
	}
	return nil
}

func SetupHostInChild(hostname string) error {
	if hostname == "" {
		return nil
	}
	return syscall.Sethostname([]byte(hostname))
}

func DropCapabilities() {
}

func SetupUserNamespaceMappings(uidMappings, gidMappings []syscall.SysProcIDMap, enableSetgroups bool) error {
	if len(uidMappings) == 0 && len(gidMappings) == 0 {
		return nil
	}
	if len(uidMappings) > 0 {
		if err := writeIDMap("/proc/self/uid_map", uidMappings); err != nil {
			return err
		}
	}
	if len(gidMappings) > 0 {
		if err := writeIDMap("/proc/self/gid_map", gidMappings); err != nil {
			return err
		}
	}
	if enableSetgroups {
		os.WriteFile("/proc/self/setgroups", []byte("deny"), 0644)
	}
	return nil
}

func writeIDMap(path string, mappings []syscall.SysProcIDMap) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, m := range mappings {
		_, err := fmt.Fprintf(f, "%d %d %d\n", m.ContainerID, m.HostID, m.Size)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetMountInfo() ([]*mountinfo.Info, error) {
	return mountinfo.GetMounts(nil)
}

func UnmountAll(mounts []string) error {
	var lastErr error
	for i := len(mounts) - 1; i >= 0; i-- {
		path := mounts[i]
		if err := syscall.Unmount(path, syscall.MNT_DETACH); err != nil {
			lastErr = fmt.Errorf("unmount %s failed: %w", path, err)
		}
	}
	return lastErr
}

func CreateDeviceNodes(devDir string) {
	os.MkdirAll(filepath.Join(devDir, "pts"), 0755)
	os.MkdirAll(filepath.Join(devDir, "shm"), 01777)
	devices := []struct {
		name  string
		major uint32
		minor uint32
		mode  os.FileMode
	}{
		{"null", 1, 3, 0666},
		{"zero", 1, 5, 0666},
		{"random", 1, 8, 0664},
		{"urandom", 1, 9, 0664},
		{"full", 1, 7, 0666},
		{"tty", 5, 0, 0666},
		{"console", 5, 1, 0600},
	}
	for _, dev := range devices {
		path := filepath.Join(devDir, dev.name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			devNum := uint64(dev.major)<<8 | uint64(dev.minor)
			unix.Mknod(path, uint32(unix.S_IFCHR)|uint32(dev.mode), int(devNum))
		}
	}
}

func BindMountFlags() uintptr {
	return unix.MS_BIND | unix.MS_NOSUID | unix.MS_PRIVATE | unix.MS_REC
}

func BindMountFlagsRoot(isRoot bool) uintptr {
	if isRoot {
		return unix.MS_BIND | unix.MS_NOSUID | unix.MS_PRIVATE | unix.MS_REC
	}
	return unix.MS_BIND | unix.MS_PRIVATE | unix.MS_REC
}

func ValidateRootfs(rootfsPath string, customShell string) error {
	if _, err := os.Stat(rootfsPath); err != nil {
		return fmt.Errorf("rootfs not exist: %w", err)
	}
	for _, d := range []string{"bin", "etc"} {
		if _, err := os.Stat(filepath.Join(rootfsPath, d)); err != nil {
			return fmt.Errorf("missing dir: %s", d)
		}
	}
	shellPath := "bin/sh"
	if customShell != "" {
		shellPath = customShell
		if shellPath[0] == '/' {
			shellPath = shellPath[1:]
		}
	}
	if _, err := os.Lstat(filepath.Join(rootfsPath, shellPath)); err != nil {
		return fmt.Errorf("missing shell: %s", shellPath)
	}
	return nil
}



func RunChild() error {
	env := os.Environ()
	argsStr := getEnv(env, "GROOT_ARGS")
	envStr := getEnv(env, "GROOT_ENV")
	workDir := getEnv(env, "GROOT_WORKDIR")
	hostname := getEnv(env, "GROOT_HOSTNAME")
	cloneFlagsStr := getEnv(env, "GROOT_CLONEFLAGS")
	var cloneFlags uintptr
	fmt.Sscanf(cloneFlagsStr, "%d", &cloneFlags)
	dropCaps := getEnv(env, "GROOT_DROPCAPS") == "1"
	noNewPrivs := getEnv(env, "GROOT_NONEWPRIVS") == "1"
	mountCount := 0
	fmt.Sscanf(getEnv(env, "GROOT_MOUNT_COUNT"), "%d", &mountCount)

	var mounts []SyscallParams
	for i := 0; i < mountCount; i++ {
		mountStr := getEnv(env, fmt.Sprintf("GROOT_MOUNT_%d", i))
		parts := strings.Split(mountStr, "|#|")
		if len(parts) >= 5 {
			source := parts[0]
			target := parts[1]
			fsType := parts[2]
			flags, _ := strconv.ParseUint(parts[3], 10, 64)
			data := parts[4]
			mounts = append(mounts, SyscallParams{
				Source: stringToPointer(source),
				Target: stringToPointer(target),
				FsType: stringToPointer(fsType),
				Flags:  uintptr(flags),
				Data:   stringToPointer(data),
			})
		}
	}

	uidCount := 0
	fmt.Sscanf(getEnv(env, "GROOT_UIDMAP_COUNT"), "%d", &uidCount)
	gidCount := 0
	fmt.Sscanf(getEnv(env, "GROOT_GIDMAP_COUNT"), "%d", &gidCount)
	setgroups := getEnv(env, "GROOT_SETGROUPS") == "1"

	var uidMaps []syscall.SysProcIDMap
	var gidMaps []syscall.SysProcIDMap
	for i := 0; i < uidCount; i++ {
		uidMapStr := getEnv(env, fmt.Sprintf("GROOT_UIDMAP_%d", i))
		var c, h, s int
		fmt.Sscanf(uidMapStr, "%d %d %d", &c, &h, &s)
		uidMaps = append(uidMaps, syscall.SysProcIDMap{ContainerID: c, HostID: h, Size: s})
	}
	for i := 0; i < gidCount; i++ {
		gidMapStr := getEnv(env, fmt.Sprintf("GROOT_GIDMAP_%d", i))
		var c, h, s int
		fmt.Sscanf(gidMapStr, "%d %d %d", &c, &h, &s)
		gidMaps = append(gidMaps, syscall.SysProcIDMap{ContainerID: c, HostID: h, Size: s})
	}
	if len(uidMaps) > 0 || len(gidMaps) > 0 {
		if err := SetupUserNamespaceMappings(uidMaps, gidMaps, setgroups); err != nil {
			return fmt.Errorf("setup user namespace failed: %w", err)
		}
	}

	if hostname != "" {
		if err := SetupHostInChild(hostname); err != nil {
			return fmt.Errorf("set hostname failed: %w", err)
		}
	}

	if err := SetupMountsInChild(mounts); err != nil {
		return fmt.Errorf("setup mounts failed: %w", err)
	}

	rlimitCount := 0
	fmt.Sscanf(getEnv(env, "GROOT_RLIMIT_COUNT"), "%d", &rlimitCount)
	var rlimits []RLimit
	for i := 0; i < rlimitCount; i++ {
		rlimitStr := getEnv(env, fmt.Sprintf("GROOT_RLIMIT_%d", i))
		var res, cur, max int
		_, err := fmt.Sscanf(rlimitStr, "%d %d %d", &res, &cur, &max)
		if err != nil {
			continue
		}
		rlimits = append(rlimits, RLimit{Res: res, Rlim: syscall.Rlimit{Cur: uint64(cur), Max: uint64(max)}})
	}
	if len(rlimits) > 0 {
		if err := SetupRlimitsInChild(rlimits); err != nil {
			return fmt.Errorf("setup rlimits failed: %w", err)
		}
	}

	if dropCaps {
		DropCapabilities()
	}

	if noNewPrivs {
		unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
	}

	if workDir != "" {
		os.Chdir(workDir)
	}

	args := strings.Split(argsStr, "|#|")
	if len(args) == 0 || args[0] == "" {
		args = []string{"/bin/sh"}
	}
	envArgs := strings.Split(envStr, "|#|")
	for i := range envArgs {
		if envArgs[i] == "" {
			envArgs = append(envArgs[:i], envArgs[i+1:]...)
			i--
		}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = envArgs
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = workDir

	return cmd.Run()
}

func stringToPointer(s string) *byte {
	if s == "" {
		return nil
	}
	b := []byte(s + "\x00")
	return &b[0]
}

func getEnv(env []string, key string) string {
	for _, e := range env {
		if len(e) > len(key)+1 && e[:len(key)] == key && e[len(key)] == '=' {
			return e[len(key)+1:]
		}
	}
	return ""
}
