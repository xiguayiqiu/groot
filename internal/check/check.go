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

	"groot/internal/permission"
	"groot/internal/termux"
)

type CheckMode int

const (
	ModeAll   CheckMode = iota
	ModeProot
	ModeChroot
)

type CheckResult struct {
	Name    string
	Passed  bool
	Message string
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

	info := getDeviceInfo()

	results = append(results, checkEnvironment(info)...)
	results = append(results, checkArch(info)...)
	results = append(results, checkKernel(info)...)
	results = append(results, checkStorage()...)

	switch mode {
	case ModeProot:
		results = append(results, checkProotRequirements(info)...)
	case ModeChroot:
		results = append(results, checkChrootRequirements(info)...)
	default:
		results = append(results, checkCommands(info)...)
		results = append(results, checkFilesystem(info)...)
		results = append(results, checkLibraries(info)...)
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
	cmd := exec.Command("getenforce")
	out, err := cmd.Output()
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
	cmd := exec.Command("id")
	out, err := cmd.Output()
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

func checkEnvironment(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	if info.IsTermux {
		results = append(results, CheckResult{
			Name:    "Termux 环境",
			Passed:  true,
			Message: fmt.Sprintf("v%s", info.TermuxVer),
		})
	} else if info.IsContainer {
		results = append(results, CheckResult{
			Name:    "容器环境",
			Passed:  true,
			Message: "检测到容器环境",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "运行环境",
			Passed:  true,
			Message: "标准 Linux 环境",
		})
	}

	if info.IsRoot {
		results = append(results, CheckResult{
			Name:    "Root 权限",
			Passed:  true,
			Message: "当前具有 root 权限",
		})
	} else if info.IsTermux && termux.IsRooted() {
		results = append(results, CheckResult{
			Name:    "Root 权限",
			Passed:  true,
			Message: "设备已 root（可通过 su 获取）",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Root 权限",
			Passed:  false,
			Message: "未检测到 root 权限",
		})
	}

	if info.IsTermux {
		results = append(results, CheckResult{
			Name:    "Android 版本",
			Passed:  true,
			Message: info.AndroidVer,
		})
		results = append(results, CheckResult{
			Name:    "设备型号",
			Passed:  true,
			Message: info.Model,
		})
	}

	if len(info.UserGroups) > 0 {
		results = append(results, CheckResult{
			Name:    "用户组",
			Passed:  true,
			Message: fmt.Sprintf("%d 个组", len(info.UserGroups)),
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
		Name:    "CPU 架构",
		Passed:  isSupported,
		Message: fmt.Sprintf("%s (%s)", archName, archDesc),
	})

	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		compat32 := check32BitCompat()
		results = append(results, CheckResult{
			Name:    "32位兼容",
			Passed:  compat32,
			Message: get32BitCompatMessage(),
		})
	}

	if runtime.GOARCH == "arm" || runtime.GOARCH == "arm64" {
		results = append(results, CheckResult{
			Name:    "CPU 信息",
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
		return "原生 32 位架构"
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		if _, err := os.Stat("/lib32"); err == nil {
			return "支持 (有 /lib32)"
		}
		if _, err := os.Stat("/usr/lib32"); err == nil {
			return "支持 (有 /usr/lib32)"
		}
	}

	if arch == "arm64" {
		if _, err := os.Stat("/usr/lib32"); err == nil {
			return "支持 (有 /usr/lib32)"
		}
	}

	return "未检测到 32 位兼容层"
}

func checkKernel(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	results = append(results, CheckResult{
		Name:    "系统架构",
		Passed:  true,
		Message: fmt.Sprintf("%s (%s)", runtime.GOOS, info.Arch),
	})

	results = append(results, CheckResult{
		Name:    "内核版本",
		Passed:  true,
		Message: truncateString(info.KernelVer, 60),
	})

	if info.Selinux != "unknown" {
		passed := info.Selinux == "Disabled" || info.Selinux == "Permissive"
		results = append(results, CheckResult{
			Name:    "SELinux 状态",
			Passed:  passed,
			Message: info.Selinux,
		})
	}

	if userns := checkUserNamespace(); userns != "" {
		results = append(results, CheckResult{
			Name:    "用户命名空间",
			Passed:  true,
			Message: userns,
		})
	}

	results = append(results, checkKernelConfig()...)

	return results
}

func checkUserNamespace() string {
	if permission.HasUnprivilegedUserns() {
		return "支持非特权用户命名空间"
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
		"CONFIG_NAMESPACES=y":     "命名空间支持",
		"CONFIG_USER_NS=y":        "用户命名空间",
		"CONFIG_PID_NS=y":         "PID 命名空间",
		"CONFIG_NET_NS=y":         "网络命名空间",
		"CONFIG_UTS_NS=y":         "UTS 命名空间",
		"CONFIG_IPC_NS=y":         "IPC 命名空间",
		"CONFIG_CGROUPS=y":        "cgroup 支持",
		"CONFIG_OVERLAY_FS=y":     "OverlayFS",
		"CONFIG_VETH=y":           "虚拟以太网设备",
		"CONFIG_BRIDGE=y":         "网桥支持",
		"CONFIG_MACVLAN=y":        "MACVLAN",
		"CONFIG_VXLAN=y":          "VXLAN",
	}

	for config, desc := range features {
		if strings.Contains(content, config) {
			results = append(results, CheckResult{
				Name:    desc,
				Passed:  true,
				Message: "已启用",
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
			Name:    "存储空间",
			Passed:  passed,
			Message: fmt.Sprintf("可用 %.2f GB / 总计 %.2f GB", freeGB, totalGB),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "存储空间",
			Passed:  false,
			Message: "无法获取存储信息",
		})
	}

	if info, err := os.Stat("/data/data/com.termux/files"); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			results = append(results, CheckResult{
				Name:    "Termux 数据目录",
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
			Name:    "proot",
			Passed:  true,
			Message: fmt.Sprintf("%s", path),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "proot",
			Passed:  false,
			Message: "未安装 proot",
		})
	}

	results = append(results, checkProotKernelSupport()...)
	results = append(results, checkProotSyscall()...)

	return results
}

func checkProotKernelSupport() []CheckResult {
	var results []CheckResult

	userns := checkUserNamespace()
	if userns != "" {
		results = append(results, CheckResult{
			Name:    "用户命名空间",
			Passed:  true,
			Message: userns,
		})
	} else {
		results = append(results, CheckResult{
			Name:    "用户命名空间",
			Passed:  false,
			Message: "proot 需要用户命名空间支持",
		})
	}

	configPath := "/proc/config.gz"
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err == nil {
			content := string(data)

			if strings.Contains(content, "CONFIG_SYSCTL=y") {
				results = append(results, CheckResult{
					Name:    "sysctl 支持",
					Passed:  true,
					Message: "已启用",
				})
			}

			if strings.Contains(content, "CONFIG_SECCOMP=y") {
				results = append(results, CheckResult{
					Name:    "Seccomp 支持",
					Passed:  true,
					Message: "已启用",
				})
			}
		}
	}

	return results
}

func checkProotSyscall() []CheckResult {
	var results []CheckResult

	testFile := "/tmp/.groot_proot_test"
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		os.Remove(testFile)

		results = append(results, CheckResult{
			Name:    "文件系统访问",
			Passed:  true,
			Message: "可以访问 /tmp",
		})
	}

	if _, err := os.Stat("/proc/self/exe"); err == nil {
		results = append(results, CheckResult{
			Name:    "proc 文件系统",
			Passed:  true,
			Message: "可以访问 /proc",
		})
	}

	return results
}

func checkChrootRequirements(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	if info.IsRoot {
		results = append(results, CheckResult{
			Name:    "Root 权限",
			Passed:  true,
			Message: "当前具有 root 权限",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Root 权限",
			Passed:  false,
			Message: "chroot 需要 root 权限",
		})
	}

	if path, err := termux.SafeLookPath("chroot"); err == nil {
		results = append(results, CheckResult{
			Name:    "chroot",
			Passed:  true,
			Message: fmt.Sprintf("%s", path),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "chroot",
			Passed:  false,
			Message: "未找到 chroot 命令",
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
			Name:    "Loop 设备",
			Passed:  true,
			Message: "支持 loop 设备",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Loop 设备",
			Passed:  false,
			Message: "未检测到 loop 设备支持",
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
			Message: "支持 bind mount",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Bind Mount",
			Passed:  false,
			Message: "未检测到 bind mount 支持",
		})
	}

	procMount := false
	if _, err := os.Stat("/proc/mounts"); err == nil {
		procMount = true
	}

	if procMount {
		results = append(results, CheckResult{
			Name:    "proc 挂载",
			Passed:  true,
			Message: "/proc 可用",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "proc 挂载",
			Passed:  false,
			Message: "/proc 不可用",
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
			Name:    "设备节点",
			Passed:  true,
			Message: fmt.Sprintf("找到 %d/%d 个必要设备", found, len(devNodes)),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "设备节点",
			Passed:  false,
			Message: fmt.Sprintf("仅找到 %d/%d 个必要设备", found, len(devNodes)),
		})
	}

	return results
}

func checkCommands(info *DeviceInfo) []CheckResult {
	var results []CheckResult

	essentialCmds := map[string]string{
		"tar":   "用于解压 rootfs",
		"wget":  "用于下载 rootfs 镜像",
		"curl":  "用于下载 rootfs 镜像",
		"proot": "用于非 root 模式运行",
		"chroot": "用于 root 模式运行",
	}

	for cmd, desc := range essentialCmds {
		if path, err := termux.SafeLookPath(cmd); err == nil {
			results = append(results, CheckResult{
				Name:    cmd,
				Passed:  true,
				Message: fmt.Sprintf("%s (%s)", path, desc),
			})
		} else {
			optional := cmd == "wget" || cmd == "curl"
			results = append(results, CheckResult{
				Name:    cmd,
				Passed:  optional,
				Message: desc,
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
					Message: fmt.Sprintf("%s (包管理器)", path),
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
						Message: fmt.Sprintf("%s (包管理器)", path),
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
			Name:    "Loop 设备",
			Passed:  true,
			Message: "支持 loop 设备",
		})
	} else {
		msg := "未检测到 loop 设备支持"
		if !permission.IsRoot() && !info.IsTermux {
			msg = "需要 root 权限检测"
		}
		results = append(results, CheckResult{
			Name:    "Loop 设备",
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
			Message: "支持 OverlayFS",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "OverlayFS",
			Passed:  false,
			Message: "未检测到 OverlayFS 支持",
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
			Message: "支持 bind mount",
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Bind Mount",
			Passed:  false,
			Message: "未检测到 bind mount 支持",
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
			testFile := filepath.Join(tmpDir, ".groot_test")
			if f, err := os.Create(testFile); err == nil {
				f.Close()
				os.Remove(testFile)
				results = append(results, CheckResult{
					Name:    "临时目录",
					Passed:  true,
					Message: fmt.Sprintf("%s (可写)", tmpDir),
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
			Name:    "系统库",
			Passed:  true,
			Message: fmt.Sprintf("找到 %d 个", len(foundLibs)),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "系统库",
			Passed:  false,
			Message: "未找到必要的系统库",
		})
	}

	return results
}

func printResults(results []CheckResult, info *DeviceInfo, mode CheckMode) {
	fmt.Println()
	fmt.Println("====================================")

	switch mode {
	case ModeProot:
		fmt.Println("    Proot 环境检查报告")
	case ModeChroot:
		fmt.Println("    Chroot 环境检查报告")
	default:
		fmt.Println("    Groot 设备检查报告")
	}

	fmt.Println("====================================")
	fmt.Println()

	if info.IsTermux {
		fmt.Printf("  设备: %s\n", info.Model)
		fmt.Printf("  系统: Android %s\n", info.AndroidVer)
		fmt.Printf("  环境: Termux v%s\n", info.TermuxVer)
	} else if info.IsContainer {
		fmt.Printf("  环境: 容器\n")
		fmt.Printf("  系统: %s\n", truncateString(info.KernelVer, 50))
	} else {
		fmt.Printf("  系统: %s\n", truncateString(info.KernelVer, 50))
	}
	fmt.Printf("  架构: %s (%s)\n", info.Arch, info.ArchCompat)
	fmt.Printf("  CPU:  %s\n", truncateString(info.CPUInfo, 50))
	fmt.Println()
	fmt.Println("  检查项目:")
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

		fmt.Printf("    %s %s\n", status, r.Name)
	}

	fmt.Println()
	fmt.Println("  ----------------------------------------")

	if failed == 0 {
		fmt.Printf("\033[32m  ✓ 所有检查通过 (%d/%d)\033[0m\n", passed, len(results))

		switch mode {
		case ModeProot:
			fmt.Println("\n  您的设备完全支持 proot 模式！")
		case ModeChroot:
			fmt.Println("\n  您的设备完全支持 chroot 模式！")
		default:
			fmt.Println("\n  您的设备完全支持创建 Linux Rootfs 文件系统！")
		}
	} else {
		fmt.Printf("\033[33m  ✓ 通过: %d  ✗ 失败: %d\033[0m\n", passed, failed)

		switch mode {
		case ModeProot:
			fmt.Println("\n  proot 模式可能受限，建议检查失败项。")
		case ModeChroot:
			fmt.Println("\n  chroot 模式可能受限，建议检查失败项。")
		default:
			fmt.Println("\n  部分功能可能受限，建议使用 proot 模式。")
		}
	}

	fmt.Println()

	switch mode {
	case ModeProot:
		fmt.Println("  提示：")
		fmt.Println("    - 使用 ./groot --check proot 检查 proot 环境")
		fmt.Println("    - 使用 ./groot -p <rootfs> 进入 proot 模式")
	case ModeChroot:
		fmt.Println("  提示：")
		fmt.Println("    - 使用 ./groot --check chroot 检查 chroot 环境")
		fmt.Println("    - 使用 ./groot -c <rootfs> 进入 chroot 模式")
	default:
		fmt.Println("  提示：")
		fmt.Println("    - proot 模式无需 root 权限")
		fmt.Println("    - chroot 模式需要 root 权限")
		fmt.Println("    - 使用 ./groot --check proot 检查 proot 环境")
		fmt.Println("    - 使用 ./groot --check chroot 检查 chroot 环境")
		fmt.Println("    - 使用 ./groot -p <rootfs> 进入 proot 模式")
		fmt.Println("    - 使用 ./groot -c <rootfs> 进入 chroot 模式")
	}

	fmt.Println()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
