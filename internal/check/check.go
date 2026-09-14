package check

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"litevm/internal/i18n"
	"litevm/internal/permission"
	"litevm/internal/termux"
)

type CheckMode int

const (
	ModeAll   CheckMode = iota
	ModeProot
	ModeChroot
	ModeVMM
)

type CheckResult struct {
	Name     string
	Passed   bool
	Message  string
	Critical bool // 重点要求，失败时显示 [!]
}

type DeviceInfo struct {
	IsTermux     bool
	IsRoot       bool
	Arch         string
	ArchCompat   string
	KernelVer    string
	KernelConfig string
	Selinux      string
	TermuxVer    string
	AndroidVer   string
	Model        string
	CPUInfo      string
	UserGroups   []string
	IsContainer  bool
	HasSystemd   bool
}

func RunCheck(mode CheckMode) []CheckResult {
	var results []CheckResult

	// 捕获 panic，防止程序崩溃
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("\033[31m[ERROR] Check panicked: %v\033[0m\n", r)
		}
	}()

	info := getDeviceInfo()

	// proot 模式不需要检查 Root 权限和用户命名空间
	skipRoot := mode == ModeProot || (mode == ModeAll && !info.IsRoot)
	skipUserNS := mode == ModeProot || (mode == ModeAll && !info.IsRoot)
	results = append(results, checkEnvironment(info, skipRoot)...)
	results = append(results, checkArch(info)...)
	results = append(results, checkKernel(info, skipUserNS)...)
	results = append(results, checkStorage()...)

	switch mode {
	case ModeProot:
		results = append(results, checkProotRequirements(info)...)
	case ModeChroot:
		results = append(results, checkChrootRequirements(info)...)
	case ModeVMM:
		results = append(results, checkVMMRequirements(info)...)
	default:
		// 无 root 只检查 proot，有 root 检查 proot + chroot
		results = append(results, checkProotRequirements(info)...)
		if info.IsRoot {
			results = append(results, checkChrootRequirements(info)...)
		}
		results = append(results, checkVMMRequirements(info)...)
	}

	printResults(results, info, mode)

	return results
}

func getDeviceInfo() *DeviceInfo {
	info := &DeviceInfo{
		IsTermux:    termux.IsTermux(),
		IsRoot:      permission.IsRoot(),
		Arch:        runtime.GOARCH,
		ArchCompat:  getArchCompat(),
		IsContainer: detectContainer(),
		HasSystemd:  detectSystemd(),
		UserGroups:  getUserGroups(),
	}

	data, err := os.ReadFile("/proc/version")
	if err == nil {
		info.KernelVer = strings.TrimSpace(string(data))
	}

	info.Selinux = getSelinuxStatus()
	info.CPUInfo = getCPUInfo()

	if info.IsTermux {
		info.TermuxVer = os.Getenv("TERMUX_VERSION")
		info.AndroidVer = getAndroidVersion()
		info.Model = getDeviceModel()
	}

	return info
}

func getArchCompat() string {
	arch := runtime.GOARCH
	switch arch {
	case "arm":
		return "armv7l/armhf (32位)"
	case "arm64":
		return "aarch64 (64位)"
	case "amd64":
		return "x86_64 (64位)"
	case "386":
		return "i386/i686 (32位)"
	case "mips", "mipsle":
		return "MIPS (32位)"
	case "mips64", "mips64le":
		return "MIPS64 (64位)"
	case "ppc64", "ppc64le":
		return "PowerPC64"
	case "s390x":
		return "IBM System z"
	case "riscv64":
		return "RISC-V (64位)"
	default:
		return arch
	}
}

func getCPUInfo() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Model") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	if runtime.GOARCH == "arm" || runtime.GOARCH == "arm64" {
		return "ARM CPU"
	}
	return "x86 CPU"
}

func getSelinuxStatus() string {
	path, err := termux.SafeLookPath("getenforce")
	if err != nil {
		return "unknown"
	}
	out, err := exec.Command(path).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getAndroidVersion() string {
	val := os.Getenv("ANDROID_VERSION")
	if val != "" {
		return val
	}
	data, err := os.ReadFile("/system/build.prop")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "ro.build.version.release=") {
				return strings.TrimPrefix(line, "ro.build.version.release=")
			}
		}
	}
	return "unknown"
}

func getDeviceModel() string {
	val := os.Getenv("MODEL")
	if val != "" {
		return val
	}
	data, err := os.ReadFile("/system/build.prop")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "ro.product.model=") {
				return strings.TrimPrefix(line, "ro.product.model=")
			}
		}
	}
	return "unknown"
}

func detectContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "lxc") ||
			strings.Contains(content, "kubepods") || strings.Contains(content, "containerd") {
			return true
		}
	}

	if _, err := os.Stat("/run/host/container-manager"); err == nil {
		return true
	}

	return false
}

func detectSystemd() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	if _, err := termux.SafeLookPath("systemctl"); err == nil {
		return true
	}
	return false
}

func getUserGroups() []string {
	path, err := termux.SafeLookPath("id")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path).Output()
	if err != nil {
		return nil
	}

	groups := []string{}
	parts := strings.Split(string(out), "=")
	if len(parts) > 1 {
		groupPart := parts[len(parts)-1]
		for _, g := range strings.Split(groupPart, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				groups = append(groups, g)
			}
		}
	}
	return groups
}

func checkEnvironment(info *DeviceInfo, skipRootCheck bool) []CheckResult {
	var results []CheckResult

	if info.IsTermux {
		results = append(results, CheckResult{
			Name:    i18n.T("check.termux_env"),
			Passed:  true,
			Message: fmt.Sprintf("v%s", info.TermuxVer),
		})
	} else if info.IsContainer {
		results = append(results, CheckResult{
			Name:    i18n.T("check.container_env"),
			Passed:  true,
			Message: i18n.T("check.detected_container"),
		})
	} else {
		results = append(results, CheckResult{
			Name:    i18n.T("check.run_env"),
			Passed:  true,
			Message: i18n.T("check.standard_linux"),
		})
	}

	if !skipRootCheck {
		if info.IsRoot {
			results = append(results, CheckResult{
				Name:    i18n.T("check.root_perm"),
				Passed:  true,
				Message: i18n.T("check.has_root"),
			})
		} else if info.IsTermux && termux.IsRooted() {
			results = append(results, CheckResult{
				Name:    i18n.T("check.root_perm"),
				Passed:  true,
				Message: i18n.T("check.device_rooted"),
			})
		} else {
			results = append(results, CheckResult{
				Name:    i18n.T("check.root_perm"),
				Passed:  false,
				Message: i18n.T("check.no_root"),
			})
		}
	}

	if info.IsTermux {
		results = append(results, CheckResult{
			Name:    i18n.T("check.android_ver"),
			Passed:  true,
			Message: info.AndroidVer,
		})
		results = append(results, CheckResult{
			Name:    i18n.T("check.device_model"),
			Passed:  true,
			Message: info.Model,
		})
	}

	if len(info.UserGroups) > 0 {
		results = append(results, CheckResult{
			Name:    i18n.T("check.user_groups"),
			Passed:  true,
			Message: fmt.Sprintf("%s: %d", i18n.T("check.group_count"), len(info.UserGroups)),
		})
	}

	return results
}

func checkArch(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	archName := info.Arch
	archDesc := info.ArchCompat

	isSupported := false
	supportedArchs := []string{"amd64", "arm64", "arm", "386"}
	for _, a := range supportedArchs {
		if archName == a {
			isSupported = true
			break
		}
	}

	results = append(results, CheckResult{
		Name:    i18n.T("check.cpu_arch"),
		Passed:  isSupported,
		Message: fmt.Sprintf("%s (%s)", archName, archDesc),
	})

	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		compat32 := check32BitCompat()
		results = append(results, CheckResult{
			Name:    i18n.T("check.arch_32bit_compat"),
			Passed:  compat32,
			Message: get32BitCompatMessage(),
		})
	}

	if runtime.GOARCH == "arm" || runtime.GOARCH == "arm64" {
		results = append(results, CheckResult{
			Name:    i18n.T("check.cpu_info"),
			Passed:  true,
			Message: truncateString(info.CPUInfo, 50),
		})
	}

	return results
}

func check32BitCompat() bool {
	if runtime.GOARCH == "386" || runtime.GOARCH == "arm" {
		return true
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		if _, err := os.Stat("/lib32"); err == nil {
			return true
		}
		if _, err := os.Stat("/usr/lib32"); err == nil {
			return true
		}
	}

	if arch == "arm64" {
		if _, err := os.Stat("/usr/lib32"); err == nil {
			return true
		}
	}

	return false
}

func get32BitCompatMessage() string {
	if runtime.GOARCH == "386" || runtime.GOARCH == "arm" {
		return i18n.T("check.native_32bit")
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		if _, err := os.Stat("/lib32"); err == nil {
			return i18n.T("check.has_lib32")
		}
		if _, err := os.Stat("/usr/lib32"); err == nil {
			return i18n.T("check.has_usrlib32")
		}
	}

	if arch == "arm64" {
		if _, err := os.Stat("/usr/lib32"); err == nil {
			return i18n.T("check.has_usrlib32")
		}
	}

	return i18n.T("check.no_32bit_compat")
}

func checkKernel(info *DeviceInfo, skipUserNS bool) []CheckResult {
	var results []CheckResult

	results = append(results, CheckResult{
		Name:    i18n.T("check.sys_arch"),
		Passed:  true,
		Message: fmt.Sprintf("%s (%s)", runtime.GOOS, info.Arch),
	})

	results = append(results, CheckResult{
		Name:    i18n.T("check.kernel_ver"),
		Passed:  true,
		Message: truncateString(info.KernelVer, 60),
	})

	if info.Selinux != "unknown" {
		passed := info.Selinux == "Disabled" || info.Selinux == "Permissive"
		results = append(results, CheckResult{
			Name:    i18n.T("check.selinux"),
			Passed:  passed,
			Message: info.Selinux,
		})
	}

	if !skipUserNS {
		if userns := checkUserNamespace(); userns != "" {
			results = append(results, CheckResult{
				Name:    i18n.T("check.user_ns"),
				Passed:  true,
				Message: userns,
			})
		}
	}

	results = append(results, checkKernelConfig()...)

	return results
}

func checkUserNamespace() string {
	if permission.HasUnprivilegedUserns() {
		return i18n.T("check.unpriv_ns_support")
	}

	data, err := os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err == nil {
		val := strings.TrimSpace(string(data))
		if val != "0" && val != "" {
			return fmt.Sprintf("最大用户命名空间数: %s", val)
		}
	}

	return ""
}

func checkKernelConfig() []CheckResult {
	var results []CheckResult

	configPath := "/proc/config.gz"
	if _, err := os.Stat(configPath); err != nil {
		return results
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return results
	}

	content := string(data)

	features := map[string]string{
		"CONFIG_NAMESPACES=y":     i18n.T("check.ns_support"),
		"CONFIG_USER_NS=y":        i18n.T("check.user_ns"),
		"CONFIG_PID_NS=y":         i18n.T("check.pid_ns"),
		"CONFIG_NET_NS=y":         i18n.T("check.net_ns"),
		"CONFIG_UTS_NS=y":         i18n.T("check.uts_ns"),
		"CONFIG_IPC_NS=y":         i18n.T("check.ipc_ns"),
		"CONFIG_CGROUPS=y":        i18n.T("check.cgroup"),
		"CONFIG_OVERLAY_FS=y":     "OverlayFS",
		"CONFIG_VETH=y":           i18n.T("check.veth"),
		"CONFIG_BRIDGE=y":         i18n.T("check.bridge"),
		"CONFIG_MACVLAN=y":        "MACVLAN",
		"CONFIG_VXLAN=y":          "VXLAN",
	}

	for config, desc := range features {
		if strings.Contains(content, config) {
			results = append(results, CheckResult{
				Name:    desc,
				Passed:  true,
				Message: i18n.T("check.enabled"),
			})
		}
	}

	return results
}

func checkStorage() []CheckResult {
	var results []CheckResult

	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		totalGB := float64(stat.Blocks*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
		freeGB := float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
		passed := freeGB >= 1.0

		results = append(results, CheckResult{
			Name:    i18n.T("check.storage"),
			Passed:  passed,
			Message: i18n.Tf("check.storage_msg", freeGB, totalGB),
		})
	} else {
		results = append(results, CheckResult{
			Name:    i18n.T("check.storage"),
			Passed:  false,
			Message: i18n.T("check.storage_fail"),
		})
	}

	if info, err := os.Stat("/data/data/com.termux/files"); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			results = append(results, CheckResult{
				Name:    i18n.T("check.termux_data"),
				Passed:  true,
				Message: fmt.Sprintf("UID: %d, GID: %d", stat.Uid, stat.Gid),
			})
		}
	}

	return results
}

func checkProotRequirements(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	if path, err := termux.SafeLookPath("proot"); err == nil {
		results = append(results, CheckResult{
			Name:     "proot",
			Passed:   true,
			Message:  fmt.Sprintf("%s", path),
			Critical: true,
		})
	} else {
		results = append(results, CheckResult{
			Name:     "proot",
			Passed:   false,
			Message:  i18n.T("check.no_proot"),
			Critical: true,
		})
	}

	results = append(results, checkProotSyscall()...)

	return results
}

func checkProotKernelSupport() []CheckResult {
	var results []CheckResult

	userns := checkUserNamespace()
	if userns != "" {
		results = append(results, CheckResult{
			Name:     i18n.T("check.user_ns"),
			Passed:   true,
			Message:  userns,
			Critical: true,
		})
	} else {
		results = append(results, CheckResult{
			Name:     i18n.T("check.user_ns"),
			Passed:   false,
			Message:  i18n.T("check.proot_ns"),
			Critical: true,
		})
	}

	configPath := "/proc/config.gz"
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err == nil {
			content := string(data)

			if strings.Contains(content, "CONFIG_SYSCTL=y") {
				results = append(results, CheckResult{
					Name:    i18n.T("check.sysctl"),
					Passed:  true,
					Message: i18n.T("check.enabled"),
				})
			}

			if strings.Contains(content, "CONFIG_SECCOMP=y") {
				results = append(results, CheckResult{
					Name:    i18n.T("check.seccomp"),
					Passed:  true,
					Message: i18n.T("check.enabled"),
				})
			}
		}
	}

	return results
}

func checkProotSyscall() []CheckResult {
	var results []CheckResult

	testFile := "/tmp/.litevm_proot_test"
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		os.Remove(testFile)

		results = append(results, CheckResult{
			Name:    i18n.T("check.fs_access"),
			Passed:  true,
			Message: i18n.T("check.tmp_ok"),
		})
	}

	if _, err := os.Stat("/proc/self/exe"); err == nil {
		results = append(results, CheckResult{
			Name:    i18n.T("check.proc_fs"),
			Passed:  true,
			Message: i18n.T("check.proc_ok"),
		})
	}

	return results
}

func checkChrootRequirements(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	if info.IsRoot {
		results = append(results, CheckResult{
			Name:     i18n.T("check.root_perm"),
			Passed:   true,
			Message:  i18n.T("check.has_root"),
			Critical: true,
		})
	} else {
		results = append(results, CheckResult{
			Name:     i18n.T("check.root_perm"),
			Passed:   false,
			Message:  i18n.T("check.chroot_needs_root"),
			Critical: true,
		})
	}

	if path, err := termux.SafeLookPath("chroot"); err == nil {
		results = append(results, CheckResult{
			Name:     "chroot",
			Passed:   true,
			Message:  fmt.Sprintf("%s", path),
			Critical: true,
		})
	} else {
		results = append(results, CheckResult{
			Name:     "chroot",
			Passed:   false,
			Message:  i18n.T("check.chroot_cmd"),
			Critical: true,
		})
	}

	results = append(results, checkChrootMountSupport()...)
	results = append(results, checkChrootDeviceNodes()...)

	return results
}

func checkChrootMountSupport() []CheckResult {
	var results []CheckResult

	loopSupport := false
	if _, err := os.Stat("/dev/loop0"); err == nil {
		loopSupport = true
	} else if _, err := os.Stat("/dev/block/loop0"); err == nil {
		loopSupport = true
	} else if _, err := os.Stat("/dev/block"); err == nil {
		entries, err := os.ReadDir("/dev/block")
		if err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "loop") {
					loopSupport = true
					break
				}
			}
		}
	}

	if loopSupport {
		results = append(results, CheckResult{
			Name:    i18n.T("check.loop_device"),
			Passed:  true,
			Message: i18n.T("check.loop_ok"),
		})
	} else {
		results = append(results, CheckResult{
			Name:    i18n.T("check.loop_device"),
			Passed:  false,
			Message: i18n.T("check.loop_fail"),
		})
	}

	bindMount := false
	if _, err := os.Stat("/proc/self/mountinfo"); err == nil {
		mountInfo, err := os.ReadFile("/proc/self/mountinfo")
		if err == nil {
			if strings.Contains(string(mountInfo), "bind") {
				bindMount = true
			}
		}
	}

	if bindMount {
		results = append(results, CheckResult{
			Name:    "Bind Mount",
			Passed:  true,
			Message: i18n.T("check.bind_mount"),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Bind Mount",
			Passed:  false,
			Message: i18n.T("check.bind_mount_fail"),
		})
	}

	procMount := false
	if _, err := os.Stat("/proc/mounts"); err == nil {
		procMount = true
	}

	if procMount {
		results = append(results, CheckResult{
			Name:    i18n.T("check.proc_mount"),
			Passed:  true,
			Message: i18n.T("check.proc_mount_ok"),
		})
	} else {
		results = append(results, CheckResult{
			Name:    i18n.T("check.proc_mount"),
			Passed:  false,
			Message: i18n.T("check.proc_mount_fail"),
		})
	}

	return results
}

func checkChrootDeviceNodes() []CheckResult {
	var results []CheckResult

	devNodes := []string{
		"/dev/null",
		"/dev/zero",
		"/dev/full",
		"/dev/random",
		"/dev/urandom",
	}

	found := 0
	for _, node := range devNodes {
		if _, err := os.Stat(node); err == nil {
			found++
		}
	}

	if found == len(devNodes) {
		results = append(results, CheckResult{
			Name:    i18n.T("check.dev_nodes"),
			Passed:  true,
			Message: i18n.Tf("check.dev_nodes_found", found, len(devNodes)),
		})
	} else {
		results = append(results, CheckResult{
			Name:    i18n.T("check.dev_nodes"),
			Passed:  false,
			Message: i18n.Tf("check.dev_nodes_partial", found, len(devNodes)),
		})
	}

	return results
}

func checkVMMRequirements(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	kvmSupported := false
	if _, err := os.Stat("/dev/kvm"); err == nil {
		if info.IsRoot {
			kvmSupported = true
		} else {
			// 非 root 用户需要检查 KVM 组权限
			for _, g := range info.UserGroups {
				if g == "kvm" {
					kvmSupported = true
					break
				}
			}
		}
	}

	if kvmSupported {
		results = append(results, CheckResult{
			Name:     "KVM",
			Passed:   true,
			Message:  i18n.T("check.kvm_available"),
			Critical: true,
		})
	} else {
		msg := i18n.T("check.no_kvm")
		if _, err := os.Stat("/dev/kvm"); err == nil {
			// /dev/kvm 存在但权限不足
			if !info.IsRoot {
				msg = i18n.T("check.kvm_perm_denied")
			}
		} else {
			msg = i18n.T("check.no_kvm_device")
		}
		results = append(results, CheckResult{
			Name:     "KVM",
			Passed:   false,
			Message:  msg,
			Critical: true,
		})
	}

	firecrackerPath := findFirecracker()
	if firecrackerPath != "" {
		results = append(results, CheckResult{
			Name:     "firecracker",
			Passed:   true,
			Message:  firecrackerPath,
			Critical: true,
		})
	} else {
		results = append(results, CheckResult{
			Name:     "firecracker",
			Passed:   false,
			Message:  i18n.T("check.no_firecracker"),
			Critical: true,
		})
	}

	return results
}

func findFirecracker() string {
	searchPaths := []string{
		"/usr/local/bin/firecracker",
		"/usr/bin/firecracker",
		filepath.Join(os.Getenv("HOME"), "bin/firecracker"),
		filepath.Join(os.Getenv("HOME"), ".local/bin/firecracker"),
	}

	for _, p := range searchPaths {
		if info, err := os.Stat(p); err == nil {
			// 检查是否可执行
			if info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				return p
			}
		}
	}

	if path, err := termux.SafeLookPath("firecracker"); err == nil {
		return path
	}

	return ""
}

func checkCommands(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	type cmdInfo struct {
		desc     string
		critical bool
	}
	essentialCmds := map[string]cmdInfo{
		"tar":    {desc: i18n.T("check.desc_tar")},
		"wget":   {desc: i18n.T("check.desc_wget")},
		"curl":   {desc: i18n.T("check.desc_curl")},
		"proot":  {desc: i18n.T("check.desc_proot"), critical: true},
		"chroot": {desc: i18n.T("check.desc_chroot"), critical: true},
	}

	for cmd, info := range essentialCmds {
		if path, err := termux.SafeLookPath(cmd); err == nil {
			results = append(results, CheckResult{
				Name:     cmd,
				Passed:   true,
				Message:  fmt.Sprintf("%s (%s)", path, info.desc),
				Critical: info.critical,
			})
		} else {
			optional := cmd == "wget" || cmd == "curl"
			results = append(results, CheckResult{
				Name:     cmd,
				Passed:   optional,
				Message:  info.desc,
				Critical: info.critical,
			})
		}
	}

	if termux.IsTermux() {
		aptCmds := []string{"apt", "pkg", "apk", "pacman", "xbps-install", "dnf", "yum"}
		for _, cmd := range aptCmds {
			if path, err := termux.SafeLookPath(cmd); err == nil {
				results = append(results, CheckResult{
					Name:    cmd,
					Passed:  true,
					Message: fmt.Sprintf("%s (%s)", path, i18n.T("check.pkg_manager")),
				})
				break
			}
		}
	} else {
		pkgCmds := []string{"apt", "apt-get", "yum", "dnf", "pacman", "zypper", "apk", "xbps-install", "emerge", "nix"}
		found := false
		for _, cmd := range pkgCmds {
			if path, err := termux.SafeLookPath(cmd); err == nil {
				if !found {
					results = append(results, CheckResult{
						Name:    cmd,
						Passed:  true,
						Message: fmt.Sprintf("%s (%s)", path, i18n.T("check.pkg_manager")),
					})
					found = true
				}
			}
		}
	}

	return results
}

func checkFilesystem(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	loopSupport := false
	if _, err := os.Stat("/dev/loop0"); err == nil {
		loopSupport = true
	} else if _, err := os.Stat("/dev/block/loop0"); err == nil {
		loopSupport = true
	} else if _, err := os.Stat("/dev/block"); err == nil {
		entries, err := os.ReadDir("/dev/block")
		if err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "loop") {
					loopSupport = true
					break
				}
			}
		}
	}

	if loopSupport {
		results = append(results, CheckResult{
			Name:    i18n.T("check.loop_device"),
			Passed:  true,
			Message: i18n.T("check.loop_ok"),
		})
	} else {
		msg := i18n.T("check.loop_fail")
		if !permission.IsRoot() && !info.IsTermux {
			msg = i18n.T("check.need_root")
		}
		results = append(results, CheckResult{
			Name:    i18n.T("check.loop_device"),
			Passed:  false,
			Message: msg,
		})
	}

	overlaySupport := false
	data, err := os.ReadFile("/proc/filesystems")
	if err == nil {
		if strings.Contains(string(data), "overlay") {
			overlaySupport = true
		}
	}

	if overlaySupport {
		results = append(results, CheckResult{
			Name:    "OverlayFS",
			Passed:  true,
			Message: i18n.T("check.overlay"),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "OverlayFS",
			Passed:  false,
			Message: i18n.T("check.overlay_fail"),
		})
	}

	bindMount := false
	if _, err := os.Stat("/proc/self/mountinfo"); err == nil {
		mountInfo, err := os.ReadFile("/proc/self/mountinfo")
		if err == nil {
			if strings.Contains(string(mountInfo), "bind") {
				bindMount = true
			}
		}
	}

	if bindMount {
		results = append(results, CheckResult{
			Name:    "Bind Mount",
			Passed:  true,
			Message: i18n.T("check.bind_mount"),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Bind Mount",
			Passed:  false,
			Message: i18n.T("check.bind_mount_fail"),
		})
	}

	tmpDir := "/tmp"
	if termux.IsTermux() {
		tmpDir = os.Getenv("TMPDIR")
		if tmpDir == "" {
			tmpDir = filepath.Join(os.Getenv("HOME"), ".tmp")
		}
	}

	if tmpDir != "" {
		if err := os.MkdirAll(tmpDir, 0755); err == nil {
			testFile := filepath.Join(tmpDir, ".litevm_test")
			if f, err := os.Create(testFile); err == nil {
				f.Close()
				os.Remove(testFile)
				results = append(results, CheckResult{
					Name:    i18n.T("check.tmp_dir"),
					Passed:  true,
					Message: fmt.Sprintf("%s (%s)", tmpDir, i18n.T("check.writable")),
				})
			}
		}
	}

	return results
}

func checkLibraries(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	libs := []string{
		"libc.so.6",
		"libc.musl-",
		"libpthread.so.0",
		"libdl.so.2",
		"librt.so.1",
		"ld-linux-",
		"ld-musl-",
	}

	foundLibs := []string{}

	searchPaths := []string{
		"/lib",
		"/lib64",
		"/usr/lib",
		"/usr/lib64",
		"/lib/aarch64-linux-gnu",
		"/lib/x86_64-linux-gnu",
		"/lib/arm-linux-gnueabihf",
		"/lib/arm-linux-gnueabi",
		"/usr/lib/aarch64-linux-gnu",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib/arm-linux-gnueabihf",
		"/usr/lib/arm-linux-gnueabi",
	}

	for _, lib := range libs {
		for _, path := range searchPaths {
			fullPath := filepath.Join(path, lib)
			if _, err := os.Stat(fullPath); err == nil {
				foundLibs = append(foundLibs, lib)
				break
			}
		}
	}

	if len(foundLibs) > 0 {
		results = append(results, CheckResult{
			Name:    i18n.T("check.sys_libs"),
			Passed:  true,
			Message: i18n.Tf("check.sys_libs_found", len(foundLibs)),
		})
	} else {
		results = append(results, CheckResult{
			Name:    i18n.T("check.sys_libs"),
			Passed:  false,
			Message: i18n.T("check.sys_libs_missing"),
		})
	}

	return results
}

func printResults(results []CheckResult, info *DeviceInfo, mode CheckMode) {
	fmt.Println()
	fmt.Println("====================================")

	switch mode {
	case ModeProot:
		fmt.Println("    " + i18n.T("check.proot_report"))
	case ModeChroot:
		fmt.Println("    " + i18n.T("check.chroot_report"))
	case ModeVMM:
		fmt.Println("    " + i18n.T("check.vmm_report"))
	default:
		fmt.Println("    " + i18n.T("check.full_report"))
	}

	fmt.Println("====================================")
	fmt.Println()

	if info.IsTermux {
		fmt.Printf("  %s: %s\n", i18n.T("check.device"), info.Model)
		fmt.Printf("  %s: Android %s\n", i18n.T("check.system"), info.AndroidVer)
		fmt.Printf("  %s: Termux v%s\n", i18n.T("check.environment"), info.TermuxVer)
	} else if info.IsContainer {
		fmt.Printf("  %s: %s\n", i18n.T("check.environment"), i18n.T("check.container"))
		fmt.Printf("  %s: %s\n", i18n.T("check.system"), truncateString(info.KernelVer, 50))
	} else {
		fmt.Printf("  %s: %s\n", i18n.T("check.system"), truncateString(info.KernelVer, 50))
	}
	fmt.Printf("  %s: %s (%s)\n", i18n.T("check.arch"), info.Arch, info.ArchCompat)
	fmt.Printf("  CPU:  %s\n", truncateString(info.CPUInfo, 50))
	fmt.Println()
	fmt.Printf("  %s:\n", i18n.T("check.items"))
	fmt.Println("  ----------------------------------------")

	passed := 0
	failed := 0

	for _, r := range results {
		var status string
		if r.Passed {
			status = "\033[32m[✓]\033[0m"
			passed++
		} else {
			status = "\033[31m[✗]\033[0m"
			failed++
		}

		if r.Critical {
			fmt.Printf("    \033[33m!\033[0m%s %s\n", status, r.Name)
		} else {
			fmt.Printf("    %s %s\n", status, r.Name)
		}
	}

	fmt.Println()
	fmt.Println("  ----------------------------------------")

	if failed == 0 {
		fmt.Printf("\033[32m  ✓ %s (%d/%d)\033[0m\n", i18n.T("check.all_pass"), passed, len(results))

		switch mode {
		case ModeProot:
			fmt.Printf("\n  %s\n", i18n.T("check.proot_supported"))
		case ModeChroot:
			fmt.Printf("\n  %s\n", i18n.T("check.chroot_supported"))
		case ModeVMM:
			fmt.Printf("\n  %s\n", i18n.T("check.vmm_supported"))
		default:
			fmt.Printf("\n  %s\n", i18n.T("check.full_support_msg"))
		}
	} else {
		fmt.Printf("\033[33m  ✓ %s: %d  ✗ %s: %d\033[0m\n", i18n.T("check.pass"), passed, i18n.T("check.fail"), failed)

		switch mode {
		case ModeProot:
			fmt.Printf("\n  %s\n", i18n.T("check.proot_limited_hint"))
		case ModeChroot:
			fmt.Printf("\n  %s\n", i18n.T("check.chroot_limited_hint"))
		case ModeVMM:
			fmt.Printf("\n  %s\n", i18n.T("check.vmm_limited_hint"))
		default:
			fmt.Printf("\n  %s\n", i18n.T("check.partial_limited_hint"))
		}
	}

	fmt.Println()

	switch mode {
	case ModeProot:
		fmt.Printf("  %s\n", i18n.T("check.hint"))
		fmt.Println("    - 使用 ./litevm --check proot 检查 proot 环境")
		fmt.Println("    - 使用 ./litevm -p <rootfs> 进入 proot 模式")
	case ModeChroot:
		fmt.Printf("  %s\n", i18n.T("check.hint"))
		fmt.Println("    - 使用 ./litevm --check chroot 检查 chroot 环境")
		fmt.Println("    - 使用 ./litevm -c <rootfs> 进入 chroot 模式")
	case ModeVMM:
		fmt.Printf("  %s\n", i18n.T("check.hint"))
		fmt.Println("    - 使用 ./litevm --check vmm 检查 VMM 环境")
		fmt.Println("    - 使用 ./litevm vmm run --kernel <path> --rootfs <path> --net 启动 VM")
	default:
		fmt.Printf("  %s\n", i18n.T("check.hint"))
		if info.IsRoot {
			fmt.Println("    - 使用 ./litevm --check proot 检查 proot 环境")
			fmt.Println("    - 使用 ./litevm --check chroot 检查 chroot 环境")
			fmt.Println("    - 使用 ./litevm --check vmm 检查 VMM 环境")
			fmt.Println("    - 使用 ./litevm -p <rootfs> 进入 proot 模式")
			fmt.Println("    - 使用 ./litevm -c <rootfs> 进入 chroot 模式")
			fmt.Println("    - 使用 ./litevm vmm run --kernel <path> --rootfs <path> --net 启动 VM")
		} else {
			fmt.Println("    - 使用 ./litevm --check proot 检查 proot 环境")
			fmt.Println("    - 使用 ./litevm --check vmm 检查 VMM 环境")
			fmt.Println("    - 使用 ./litevm -p <rootfs> 进入 proot 模式")
			fmt.Println("    - 使用 ./litevm vmm run --kernel <path> --rootfs <path> --net 启动 VM")
		}
	}

	fmt.Println()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
