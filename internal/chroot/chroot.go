package chroot

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"

	"litevm/internal/cleanup"
	"litevm/internal/env"
	"litevm/internal/i18n"
	"litevm/internal/logger"
	"litevm/internal/mount"
	"litevm/internal/network"
	"litevm/internal/permission"
	"litevm/internal/slogan"
	"litevm/internal/usercheck"
)

// shellPathRegex 验证 shell 路径只包含安全字符
var shellPathRegex = regexp.MustCompile(`^[a-zA-Z0-9_/\-.]+$`)

// validateShellPath 验证 shell 路径是否安全
func validateShellPath(shell string) bool {
	if shell == "" {
		return false
	}
	// 检查路径遍历
	if strings.Contains(shell, "..") {
		return false
	}
	return shellPathRegex.MatchString(shell)
}

// truncateHostname 截断 hostname 到内核限制 (HOST_NAME_MAX=64, 含null)
func truncateHostname(hostname string) string {
	const maxHostname = 63 // HOST_NAME_MAX - 1
	if len(hostname) > maxHostname {
		return hostname[:maxHostname]
	}
	return hostname
}

// isPrintable 检查字符是否可打印
func isPrintable(c rune) bool {
	return unicode.IsPrint(c)
}

// sanitizeShellArg 清理用于 shell -c 的参数，防止命令注入
func sanitizeShellArg(s string) string {
	// 只允许可打印字符
	result := make([]rune, 0, len(s))
	for _, c := range s {
		if isPrintable(c) {
			result = append(result, c)
		}
	}
	return string(result)
}

// Run 运行 chroot 模式
func Run(rootfsPath string, customShell string, netMode bool) error {
	// 转换为绝对路径
	absRootfsPath, err := filepath.Abs(rootfsPath)
	if err != nil {
		return fmt.Errorf("%s", i18n.Tf("chroot.path_fail", err))
	}

	logger.Info(i18n.Tf("chroot.start", absRootfsPath))

	// 验证 rootfs
	if err := mount.ValidateRootfs(absRootfsPath, customShell); err != nil {
		return err
	}

	// 在主机端提前清理 rootfs 中的宿主机环境痕迹
	preHostname := env.GetHostname(absRootfsPath, cleanup.DefaultHostname)
	if err := cleanup.CleanupRootfs(absRootfsPath, preHostname); err != nil {
		logger.Warn(i18n.Tf("chroot.cleanup_warn", err))
	}

	// 检查权限
	isRoot := permission.IsRoot()
	if isRoot {
		logger.Info(i18n.T("chroot.running_as_root"))
	} else {
		logger.Info(i18n.T("chroot.running_user_ns"))
	}

	// 默认使用 root 用户
	userInfo, err := usercheck.CheckUser(absRootfsPath, "root")
	if err != nil {
		return err
	}

	// 在主进程修复权限
	logger.Info(i18n.T("chroot.fixing_perms"))
	usercheck.FixCommonIssues(absRootfsPath, userInfo)

	// 准备用户主目录
	mount.PrepareUserHome(absRootfsPath, userInfo, false)

	// 检测并应用 Arch Linux 特定配置
	if IsArchLinux(absRootfsPath) {
		SetupArchSpecific(absRootfsPath)
		FixArchPacmanIssues(absRootfsPath)
	}

	// 使用可执行文件路径
	exePath := permission.GetExecutablePath()
	logger.Debug(i18n.Tf("chroot.using_exe", exePath))

	// 构建子进程参数
	args := []string{exePath, "chroot-child", absRootfsPath}
	if customShell != "" {
		args = append(args, customShell)
	} else {
		args = append(args, "")
	}
	if userInfo.Username != "" {
		args = append(args, userInfo.Username)
	} else {
		args = append(args, "")
	}
	if netMode {
		args = append(args, "net")
	} else {
		args = append(args, "")
	}

	// 创建子进程
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 设置命名空间
	var cloneFlags uintptr = syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC
	if !isRoot {
		cloneFlags |= syscall.CLONE_NEWUSER
	}
	if netMode {
		cloneFlags |= syscall.CLONE_NEWNET
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneFlags,
	}

	if !isRoot {
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Geteuid(), Size: 1},
		}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getegid(), Size: 1},
		}
		cmd.SysProcAttr.GidMappingsEnableSetgroups = false
	}

	logger.Info(i18n.T("chroot.creating_child"))

	// 信号转发
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	defer signal.Stop(sigChan)
	defer close(sigChan)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s", i18n.Tf("chroot.start_fail", err))
	}

	// 使用轮询等待网络命名空间就绪，替代固定 sleep
	if netMode {
		ready := make(chan struct{})
		go func() {
			for i := 0; i < 100; i++ {
				if _, err := os.Stat(fmt.Sprintf("/proc/%d/ns/net", cmd.Process.Pid)); err == nil {
					close(ready)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			close(ready) // 超时也关闭，继续尝试
		}()

		<-ready

		if err := network.SetupNetworkInChildNs(cmd.Process.Pid); err != nil {
			logger.Warn(i18n.Tf("chroot.net_setup_fail", err))
		} else {
			_ = network.SetupChildDns(absRootfsPath)
			logger.Info(i18n.T("chroot.net_setup_done"))
		}
	}

	// 后台转发信号
	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
				logger.Debug("转发信号 %v 给子进程", sig)
				if err := cmd.Process.Signal(sig); err != nil {
					logger.Debug("信号转发失败（进程可能已退出）: %v", err)
				}
			}
		}
	}()

	err = cmd.Wait()

	// 统一清理网络资源
	if netMode {
		network.CleanupNetworkOnHost()
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Info(i18n.Tf("chroot.shell_exit", exitErr.ExitCode()))
		} else {
			logger.Warn(i18n.Tf("chroot.shell_error", err))
		}
	}

	return nil
}

// ChildMain chroot 子进程入口
func ChildMain(rootfsPath string, customShell string, customUser string, netMode string) error {
	logger.Info(i18n.T("chroot.entering"))

	// 首先设置主机名（在 UTS namespace 中）
	hostname := env.GetHostname(rootfsPath, "litevm")
	hostname = truncateHostname(hostname)
	logger.Debug(i18n.Tf("chroot.hostname_set", hostname))
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		logger.Warn(i18n.Tf("chroot.hostname_fail", err))
	}

	// 保存原根目录的文件描述符，用于恢复
	rootFd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("无法打开原根目录: %w", err)
	}
	defer syscall.Close(rootFd)

	// 获取用户信息
	userInfo, err := usercheck.CheckUser(rootfsPath, "root")
	if err != nil {
		return err
	}

	// 构建环境变量，传入 rootfs 来检测发行版
	newEnv := env.SetupEnv(userInfo, hostname, rootfsPath)

	// 确定要使用的 shell
	shell := findShell(rootfsPath, customShell, userInfo.Shell)

	// 验证 shell 路径安全性
	if !validateShellPath(shell) {
		logger.Warn("shell 路径不安全: %s, 回退到 /bin/sh", shell)
		shell = "/bin/sh"
	}

	// 更新环境变量中的 SHELL
	for i, e := range newEnv {
		if len(e) > 6 && e[:6] == "SHELL=" {
			newEnv[i] = "SHELL=" + shell
			break
		}
	}

	// 关键：在新的 mount namespace 中重新挂载根目录为私有，确保不影响宿主机
	logger.Debug("将根目录重新挂载为私有...")
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		logger.Warn("重新挂载根目录失败: %v", err)
	}

	// 挂载虚拟文件系统
	logger.Info(i18n.T("chroot.mounting_vfs"))
	mounted, err := mount.MountAll(rootfsPath)
	if err != nil {
		return err
	}
	logger.Info(i18n.Tf("chroot.mount_done", mounted))

	// 创建必要的设备节点（在chroot之前，确保/dev中有基本设备）
	createDevices(rootfsPath)

	defer func() {
		// 恢复到原根目录
		logger.Debug("恢复到原根目录...")
		if err := syscall.Fchdir(rootFd); err != nil {
			logger.Warn("恢复原根目录失败: %v", err)
		} else {
			if err := syscall.Chroot("."); err != nil {
				logger.Warn("chroot 恢复失败: %v", err)
			}
		}

		// 卸载所有挂载点
		logger.Debug("卸载虚拟文件系统...")
		if err := mount.UnmountAll(mounted); err != nil {
			logger.Warn(i18n.Tf("chroot.unmount_fail", err))
		}
		logger.Info(i18n.T("chroot.unmounted"))
	}()

	// 直接使用 syscall.Chroot，不搞复杂的包装
	logger.Debug("执行 syscall.Chroot 到 %s", rootfsPath)
	if err := syscall.Chroot(rootfsPath); err != nil {
		return fmt.Errorf("chroot 失败: %w", err)
	}

	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir 失败: %w", err)
	}

	// 挂载 devpts 文件系统（必须在chroot之后，shell启动之前）
	if err := syscall.Mount("devpts", "/dev/pts", "devpts", syscall.MS_NOEXEC|syscall.MS_NOSUID, ""); err != nil {
		logger.Warn("挂载 devpts 失败: %v", err)
	}

	// 确保 /dev/pts/ptmx 设备存在
	if _, err := os.Stat("/dev/pts/ptmx"); os.IsNotExist(err) {
		devNum := (5 << 8) | 2
		if err := syscall.Mknod("/dev/pts/ptmx", syscall.S_IFCHR|0666, int(devNum)); err != nil {
			logger.Debug("创建 /dev/pts/ptmx 失败: %v", err)
		}
	}

	// 确保 /dev/ptmx 存在（符号链接或设备）
	if _, err := os.Lstat("/dev/ptmx"); os.IsNotExist(err) {
		os.Symlink("/dev/pts/ptmx", "/dev/ptmx")
	}

	// 修复rootfs中的符号链接
	fixSymLinks(rootfsPath)

	// 创建必要的目录结构以欺骗需要 systemd 的脚本
	os.MkdirAll("/run/systemd/system", 0755)
	os.MkdirAll("/run/systemd/private", 0700)
	os.MkdirAll("/run/dbus", 0755)
	os.Create("/run/systemd/container")
	os.Create("/run/systemd/systemd-logind")

	// 如果启用了网络模式，在子进程中设置 DNS
	if netMode == "net" {
		logger.Info("正在设置网络 DNS...")
		if err := network.SetupChildDns(rootfsPath); err != nil {
			logger.Warn("设置 DNS 失败: %v", err)
		}
	}

	// 检查是否是 Arch Linux 并修复 pacman 问题
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		logger.Debug("Arch Linux: 设置 pacman 运行环境...")

		// 1. 尝试卸载 /etc/mtab 的 bind mount（兼容旧版本）
		_ = syscall.Unmount("/etc/mtab", syscall.MNT_DETACH)

		// 2. 创建静态 /etc/mtab，pacman 依赖它来确定根挂载点
		mtabContent := `rootfs / rootfs rw 0 0
/dev/root / ext4 rw,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
sys /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
dev /dev devtmpfs rw,nosuid,relatime 0 0
devpts /dev/pts devpts rw,nosuid,noexec,relatime 0 0
shm /dev/shm tmpfs rw,nosuid,nodev,noexec 0 0
tmpfs /tmp tmpfs rw,nosuid,nodev 0 0
tmpfs /run tmpfs rw,nosuid,nodev,noexec,mode=755 0 0
`
		_ = os.WriteFile("/etc/mtab", []byte(mtabContent), 0644)

		// 3. 确保必要目录存在且权限正确
		os.MkdirAll("/var/cache/pacman/pkg", 0755)
		os.MkdirAll("/var/lib/pacman/sync", 0755)
		os.MkdirAll("/var/lib/pacman/local", 0755)
		os.MkdirAll("/tmp", 01777)
		_ = os.Chmod("/tmp", 01777)

		// 4. 环境变量
		_ = os.Setenv("TMPDIR", "/tmp")
		_ = os.Setenv("PACMAN_CACHE", "/var/cache/pacman/pkg")

		// 5. 创建 pacman 包装器（如果已经备份过原始 pacman）
		if _, err := os.Stat("/usr/bin/pacman.original"); err == nil {
			wrapperScript := `#!/bin/bash
export TMPDIR=/tmp
export PACMAN_CACHE=/var/cache/pacman/pkg

mkdir -p /var/cache/pacman/pkg /var/lib/pacman/sync /var/lib/pacman/local /tmp
chmod 1777 /tmp 2>/dev/null

if [ -w /etc/mtab ]; then
cat > /etc/mtab <<'MTAB'
rootfs / rootfs rw 0 0
/dev/root / ext4 rw,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
sys /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
dev /dev devtmpfs rw,nosuid,relatime 0 0
devpts /dev/pts devpts rw,nosuid,noexec,relatime 0 0
shm /dev/shm tmpfs rw,nosuid,nodev,noexec 0 0
tmpfs /tmp tmpfs rw,nosuid,nodev 0 0
tmpfs /run tmpfs rw,nosuid,nodev,noexec,mode=755 0 0
MTAB
fi

exec /usr/bin/pacman.original "$@"
`
			_ = os.WriteFile("/usr/bin/pacman", []byte(wrapperScript), 0755)
		}
	}

	// 直接执行 shell，先 source 环境！
	logger.Info(i18n.Tf("chroot.starting_shell", shell))

	// 为 Arch Linux 创建 pacman wrapper，解决挂载点检测问题
	pacmanWrapper := ""
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		wrapperScript := `#!/bin/bash
# Groot pacman wrapper: 解决 chroot 环境中的挂载点检测问题

# 强制设置 PATH，确保优先使用 /dev/shm
export PATH="/dev/shm:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

# 确保 /dev/shm 挂载
if ! mountpoint -q /dev/shm 2>/dev/null; then
    mount -t tmpfs tmpfs /dev/shm 2>/dev/null || true
fi

mkdir -p /dev/shm

# 创建自绑定让 / 看起来像挂载点
mount --bind / / 2>/dev/null || true

# 同时让 /var/cache 也成为挂载点
mount --bind /var/cache /var/cache 2>/dev/null || true

export TMPDIR=/tmp
export PACMAN_CACHE=/var/cache/pacman/pkg

mkdir -p /tmp 2>/dev/null
chmod 1777 /tmp 2>/dev/null
mkdir -p /var/cache/pacman/pkg 2>/dev/null
chmod 755 /var/cache/pacman/pkg 2>/dev/null

if [ ! -e /etc/mtab ]; then
    ln -s /proc/mounts /etc/mtab 2>/dev/null
fi

if [ ! -d /proc/self ]; then
    mount -t proc proc /proc 2>/dev/null
fi

# 执行原始 pacman
exec /usr/bin/pacman "$@"
`
		wrapperPath := "/dev/shm/pacman"
		_ = os.WriteFile(wrapperPath, []byte(wrapperScript), 0755)
		logger.Info("Created pacman wrapper at %s", wrapperPath)
		pacmanWrapper = "; export PATH=/dev/shm:/sbin:/usr/sbin:/bin:/usr/bin; echo 'Wrapper PATH: $PATH'"
	}

	// 检测是否是非 POSIX shell（fish 等不支持 bash 语法）
	shellBase := filepath.Base(shell)
	isNonPOSIXShell := shellBase == "fish" || shellBase == "csh" || shellBase == "tcsh" || shellBase == "ksh"

	// fixScript 只对 POSIX shell（bash/dash/sh/ash）执行
	fixScript := ""
	if !isNonPOSIXShell {
		fixScript = "if [ -f /var/lib/dpkg/info/libc6:amd64.postinst ]; then " +
			"grep -q 'Groot: skip' /var/lib/dpkg/info/libc6:amd64.postinst 2>/dev/null || " +
			"(cp /var/lib/dpkg/info/libc6:amd64.postinst /var/lib/dpkg/info/libc6:amd64.postinst.bak 2>/dev/null; " +
			"echo '#!/bin/bash'; echo 'exit 0' > /var/lib/dpkg/info/libc6:amd64.postinst; " +
			"chmod +x /var/lib/dpkg/info/libc6:amd64.postinst); fi"
	}

	// 为 Arch Linux 添加 /dev/shm 到 PATH
	if pacmanWrapper != "" {
		for i, e := range newEnv {
			if strings.HasPrefix(e, "PATH=") {
				newEnv[i] = e + ":/dev/shm"
				break
			}
		}
	}

	// 以 login shell 方式启动，自动 source /etc/profile（覆盖 PS1、PATH 等）
	// 启动前打印彩色广告横幅
	bannerCmd := slogan.GetBannerCmd() + "; "

	// 非 POSIX shell（fish等）不能使用 bash 语法的 fixScript/pacmanWrapper
	fixCmdStr := ""
	if !isNonPOSIXShell {
		fixCmdStr = fixScript + pacmanWrapper
	}

	// 检查是否有 script 命令（用于提供伪终端支持）
	hasScript := false
	if _, err := os.Stat("/usr/bin/script"); err == nil {
		hasScript = true
	} else if _, err := os.Stat("/bin/script"); err == nil {
		hasScript = true
	}

	// 检查是否有 cttyhack 命令（Alpine特有）
	hasCttyhack := false
	if _, err := os.Stat("/usr/bin/cttyhack"); err == nil {
		hasCttyhack = true
	} else if _, err := os.Stat("/bin/cttyhack"); err == nil {
		hasCttyhack = true
	}

	// 对 shell 参数进行安全检查
	safeShell := sanitizeShellArg(shell)

	var cmd *exec.Cmd
	if hasScript {
		// 使用 script 命令提供伪终端支持
		// 使用安全的 shell 参数
		execStr := fixCmdStr + "; " + bannerCmd + " SHELL=" + safeShell + " exec " + safeShell + " -l"
		cmd = exec.Command("/usr/bin/script", "-qc", execStr, "/dev/null")
	} else if hasCttyhack {
		// 使用 cttyhack（Alpine特有）提供tty支持
		cmd = exec.Command("/usr/bin/cttyhack", shell, "-c",
			sanitizeShellArg(fixCmdStr+"; "+bannerCmd+" SHELL="+safeShell+" exec "+safeShell+" -l"))
	} else {
		// 直接执行目标shell，不使用 /bin/sh -c
		// 先执行修复脚本，然后启动shell
		// 使用 SHELL=$shell exec $shell -l 确保SHELL环境变量正确
		startupCmd := fixCmdStr + "; " + bannerCmd + " SHELL=" + safeShell + " exec " + safeShell + " -l"
		cmd = exec.Command("/bin/sh", "-c", startupCmd)
	}

	cmd.Env = newEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = userInfo.Home

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s", i18n.Tf("chroot.shell_fail", err))
	}

	if err := cmd.Wait(); err != nil {
		// 不立即退出litevm，而是检查退出原因
		if exitErr, ok := err.(*exec.ExitError); ok {
			// 如果是信号导致的退出（如Ctrl+C），不退出litevm
			logger.Info(i18n.Tf("chroot.shell_exit", exitErr.ExitCode()))
		} else {
			logger.Warn(i18n.Tf("chroot.shell_error", err))
		}
	}

	return nil
}

// findShell 查找可用的 shell 解释器（在rootfs中查找）
func findShell(rootfsPath, customShell, defaultShell string) string {
	// 优先检查用户指定的 shell
	if customShell != "" {
		// 安全检查：防止路径遍历
		if strings.Contains(customShell, "..") {
			logger.Warn("customShell 包含路径遍历: %s", customShell)
		} else {
			shellPath := customShell
			// 如果是相对路径，添加 / 前缀
			if customShell[0] != '/' {
				shellPath = "/" + customShell
			}
			// 在rootfs中检查绝对路径
			fullPath := filepath.Join(rootfsPath, shellPath)
			if _, err := os.Stat(fullPath); err == nil {
				return shellPath
			}
			// 如果是相对路径，尝试常见的 shell 路径
			if customShell[0] != '/' {
				for _, path := range []string{"/bin/" + customShell, "/usr/bin/" + customShell} {
					fullPath := filepath.Join(rootfsPath, path)
					if _, err := os.Stat(fullPath); err == nil {
						return path
					}
				}
			}
		}
	}

	// 使用用户默认 shell
	if defaultShell != "" && defaultShell != "/usr/bin/nologin" {
		fullPath := filepath.Join(rootfsPath, defaultShell)
		if _, err := os.Stat(fullPath); err == nil {
			return defaultShell
		}
	}

	// 按优先级查找可用的 shell（在rootfs中查找）
	// 优先查找轻量级shell（ash/dash/sh），然后是bash
	shells := []string{"/bin/ash", "/bin/dash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh", "/bin/bash"}
	for _, s := range shells {
		fullPath := filepath.Join(rootfsPath, s)
		if _, err := os.Stat(fullPath); err == nil {
			return s
		}
	}

	return "/bin/sh"
}

// createDevices 在 rootfs 中创建必要的设备节点，确保终端和系统功能正常
func createDevices(rootfsPath string) {
	devPath := filepath.Join(rootfsPath, "dev")

	// 确保 /dev 目录存在
	os.MkdirAll(devPath, 0755)

	// 定义需要创建的设备节点
	devices := []struct {
		path  string
		major uint32
		minor uint32
		mode  uint32
	}{
		// 标准字符设备
		{"null", 1, 3, 0666},    // /dev/null - 空设备
		{"zero", 1, 5, 0666},    // /dev/zero - 零设备
		{"random", 1, 8, 0666},  // /dev/random - 随机数设备
		{"urandom", 1, 9, 0666}, // /dev/urandom - 非阻塞随机数设备
		{"full", 1, 7, 0666},    // /dev/full - 满设备
		{"tty", 5, 0, 0666},     // /dev/tty - 当前终端
		{"console", 5, 1, 0600}, // /dev/console - 系统控制台
		{"tty0", 4, 0, 0620},    // /dev/tty0 - 虚拟终端0
		{"tty1", 4, 1, 0620},    // /dev/tty1 - 虚拟终端1
		{"tty2", 4, 2, 0620},    // /dev/tty2 - 虚拟终端2
		{"tty3", 4, 3, 0620},    // /dev/tty3 - 虚拟终端3
		{"tty4", 4, 4, 0620},    // /dev/tty4 - 虚拟终端4
		{"tty5", 4, 5, 0620},    // /dev/tty5 - 虚拟终端5
		{"tty6", 4, 6, 0620},    // /dev/tty6 - 虚拟终端6
		{"tty7", 4, 7, 0620},    // /dev/tty7 - 虚拟终端7
		{"tty8", 4, 8, 0620},    // /dev/tty8 - 虚拟终端8
		{"tty9", 4, 9, 0620},    // /dev/tty9 - 虚拟终端9
		{"tty10", 4, 10, 0620},  // /dev/tty10 - 虚拟终端10
		{"tty11", 4, 11, 0620},  // /dev/tty11 - 虚拟终端11
		{"tty12", 4, 12, 0620},  // /dev/tty12 - 虚拟终端12
		{"tty13", 4, 13, 0620},  // /dev/tty13 - 虚拟终端13
		{"tty14", 4, 14, 0620},  // /dev/tty14 - 虚拟终端14
		{"tty15", 4, 15, 0620},  // /dev/tty15 - 虚拟终端15
		{"tty16", 4, 16, 0620},  // /dev/tty16 - 虚拟终端16
		{"tty17", 4, 17, 0620},  // /dev/tty17 - 虚拟终端17
		{"tty18", 4, 18, 0620},  // /dev/tty18 - 虚拟终端18
		{"tty19", 4, 19, 0620},  // /dev/tty19 - 虚拟终端19
		{"tty20", 4, 20, 0620},  // /dev/tty20 - 虚拟终端20
		{"tty21", 4, 21, 0620},  // /dev/tty21 - 虚拟终端21
		{"tty22", 4, 22, 0620},  // /dev/tty22 - 虚拟终端22
		{"tty23", 4, 23, 0620},  // /dev/tty23 - 虚拟终端23
		{"tty24", 4, 24, 0620},  // /dev/tty24 - 虚拟终端24
		{"tty25", 4, 25, 0620},  // /dev/tty25 - 虚拟终端25
		{"tty26", 4, 26, 0620},  // /dev/tty26 - 虚拟终端26
		{"tty27", 4, 27, 0620},  // /dev/tty27 - 虚拟终端27
		{"tty28", 4, 28, 0620},  // /dev/tty28 - 虚拟终端28
		{"tty29", 4, 29, 0620},  // /dev/tty29 - 虚拟终端29
		{"tty30", 4, 30, 0620},  // /dev/tty30 - 虚拟终端30
		{"tty31", 4, 31, 0620},  // /dev/tty31 - 虚拟终端31
		{"tty32", 4, 32, 0620},  // /dev/tty32 - 虚拟终端32
		{"tty33", 4, 33, 0620},  // /dev/tty33 - 虚拟终端33
		{"tty34", 4, 34, 0620},  // /dev/tty34 - 虚拟终端34
		{"tty35", 4, 35, 0620},  // /dev/tty35 - 虚拟终端35
		{"tty36", 4, 36, 0620},  // /dev/tty36 - 虚拟终端36
		{"tty37", 4, 37, 0620},  // /dev/tty37 - 虚拟终端37
		{"tty38", 4, 38, 0620},  // /dev/tty38 - 虚拟终端38
		{"tty39", 4, 39, 0620},  // /dev/tty39 - 虚拟终端39
		{"tty40", 4, 40, 0620},  // /dev/tty40 - 虚拟终端40
		{"tty41", 4, 41, 0620},  // /dev/tty41 - 虚拟终端41
		{"tty42", 4, 42, 0620},  // /dev/tty42 - 虚拟终端42
		{"tty43", 4, 43, 0620},  // /dev/tty43 - 虚拟终端43
		{"tty44", 4, 44, 0620},  // /dev/tty44 - 虚拟终端44
		{"tty45", 4, 45, 0620},  // /dev/tty45 - 虚拟终端45
		{"tty46", 4, 46, 0620},  // /dev/tty46 - 虚拟终端46
		{"tty47", 4, 47, 0620},  // /dev/tty47 - 虚拟终端47
		{"tty48", 4, 48, 0620},  // /dev/tty48 - 虚拟终端48
		{"tty49", 4, 49, 0620},  // /dev/tty49 - 虚拟终端49
		{"tty50", 4, 50, 0620},  // /dev/tty50 - 虚拟终端50
		{"tty51", 4, 51, 0620},  // /dev/tty51 - 虚拟终端51
		{"tty52", 4, 52, 0620},  // /dev/tty52 - 虚拟终端52
		{"tty53", 4, 53, 0620},  // /dev/tty53 - 虚拟终端53
		{"tty54", 4, 54, 0620},  // /dev/tty54 - 虚拟终端54
		{"tty55", 4, 55, 0620},  // /dev/tty55 - 虚拟终端55
		{"tty56", 4, 56, 0620},  // /dev/tty56 - 虚拟终端56
		{"tty57", 4, 57, 0620},  // /dev/tty57 - 虚拟终端57
		{"tty58", 4, 58, 0620},  // /dev/tty58 - 虚拟终端58
		{"tty59", 4, 59, 0620},  // /dev/tty59 - 虚拟终端59
		{"tty60", 4, 60, 0620},  // /dev/tty60 - 虚拟终端60
		{"tty61", 4, 61, 0620},  // /dev/tty61 - 虚拟终端61
		{"tty62", 4, 62, 0620},  // /dev/tty62 - 虚拟终端62
		{"tty63", 4, 63, 0620},  // /dev/tty63 - 虚拟终端63
		{"ttyS0", 4, 64, 0660},  // /dev/ttyS0 - 串口0
		{"ttyS1", 4, 65, 0660},  // /dev/ttyS1 - 串口1
		{"ttyS2", 4, 66, 0660},  // /dev/ttyS2 - 串口2
		{"ttyS3", 4, 67, 0660},  // /dev/ttyS3 - 串口3
		{"ttyAMA0", 204, 64, 0660}, // /dev/ttyAMA0 - ARM串口0
		{"ttyAMA1", 204, 65, 0660}, // /dev/ttyAMA1 - ARM串口1
		{"ttyV0", 236, 0, 0666},  // /dev/ttyV0 - 虚拟串口0
		{"ttyV1", 236, 1, 0666},  // /dev/ttyV1 - 虚拟串口1
		{"ttyV2", 236, 2, 0666},  // /dev/ttyV2 - 虚拟串口2
		{"ttyV3", 236, 3, 0666},  // /dev/ttyV3 - 虚拟串口3
		{"hvc0", 229, 0, 0660},   // /dev/hvc0 - 虚拟控制台0
		{"hvc1", 229, 1, 0660},   // /dev/hvc1 - 虚拟控制台1
		{"hvc2", 229, 2, 0660},   // /dev/hvc2 - 虚拟控制台2
		{"hvc3", 229, 3, 0660},   // /dev/hvc3 - 虚拟控制台3
		{"xvc0", 204, 128, 0660}, // /dev/xvc0 - Xen虚拟控制台
		{"uvhvc0", 241, 0, 0660}, // /dev/uvhvc0 - Xen半虚拟化串口
	}

	for _, dev := range devices {
		path := filepath.Join(devPath, dev.path)
		// 检查设备是否已存在（可能通过bind mount从宿主机继承）
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// 创建设备节点，使用 mknod 系统调用
			// 设备号 = (major << 8) | minor
			devNum := (dev.major << 8) | dev.minor
			if err := syscall.Mknod(path, syscall.S_IFCHR|dev.mode, int(devNum)); err != nil {
				logger.Debug("创建设备节点 %s 失败: %v", path, err)
			}
		}
	}

	// 创建 /dev/pts 目录（用于伪终端）
	ptsPath := filepath.Join(devPath, "pts")
	os.MkdirAll(ptsPath, 0755)

	// 创建 /dev/ptmx 设备（伪终端多路复用器）
	ptmxPath := filepath.Join(devPath, "ptmx")
	if _, err := os.Stat(ptmxPath); os.IsNotExist(err) {
		// ptmx: major 5, minor 2
		devNum := (5 << 8) | 2
		if err := syscall.Mknod(ptmxPath, syscall.S_IFCHR|0666, int(devNum)); err != nil {
			// 如果创建失败，尝试创建符号链接到 /dev/pts/ptmx
			os.Symlink("/dev/pts/ptmx", ptmxPath)
		}
	}

	// 创建 /dev/shm 目录（共享内存）
	shmPath := filepath.Join(devPath, "shm")
	os.MkdirAll(shmPath, 1777)

	// 创建 /dev/fd 符号链接（文件描述符）
	fdPath := filepath.Join(devPath, "fd")
	if _, err := os.Lstat(fdPath); os.IsNotExist(err) {
		os.Symlink("/proc/self/fd", fdPath)
	}

	// 创建 /dev/stdin, /dev/stdout, /dev/stderr 符号链接
	stdPaths := map[string]string{
		"stdin":  "/proc/self/fd/0",
		"stdout": "/proc/self/fd/1",
		"stderr": "/proc/self/fd/2",
	}
	for name, target := range stdPaths {
		path := filepath.Join(devPath, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			os.Symlink(target, path)
		}
	}

	// 创建 /dev/core 符号链接
	corePath := filepath.Join(devPath, "core")
	if _, err := os.Lstat(corePath); os.IsNotExist(err) {
		os.Symlink("/proc/kcore", corePath)
	}

	// 创建 /dev/pts/ptmx 设备（如果不存在）
	ptsPtmxPath := filepath.Join(ptsPath, "ptmx")
	if _, err := os.Stat(ptsPtmxPath); os.IsNotExist(err) {
		devNum := (5 << 8) | 2
		if err := syscall.Mknod(ptsPtmxPath, syscall.S_IFCHR|0666, int(devNum)); err != nil {
			logger.Debug("创建 /dev/pts/ptmx 失败: %v", err)
		}
	}

	logger.Debug("设备节点创建完成")
}

// fixSymLinks 修复rootfs中的符号链接
// 在chroot环境中，符号链接可能指向宿主机的路径，需要修复
func fixSymLinks(rootfsPath string) {
	// 常见的需要修复的链接路径
	commonLinks := []struct {
		linkPath string   // 链接路径
		targets  []string // 可能的目标文件
	}{
		{"/usr/bin/lua", []string{"/usr/bin/lua5.4", "/usr/bin/lua5.3", "/usr/bin/lua5.1", "/usr/bin/luajit"}},
		{"/usr/bin/python", []string{"/usr/bin/python3", "/usr/bin/python3.11", "/usr/bin/python3.10"}},
		{"/usr/bin/python3", []string{"/usr/bin/python3.11", "/usr/bin/python3.10", "/usr/bin/python3.9"}},
		{"/bin/sh", []string{"/bin/bash", "/bin/dash", "/bin/ash"}},
		{"/bin/ash", []string{"/bin/busybox"}},
		{"/bin/dash", []string{"/bin/busybox"}},
		{"/usr/bin/cls", []string{"/usr/bin/clear", "/usr/sbin/clear", "/bin/clear"}},
		{"/usr/bin/clear", []string{"/usr/bin/clear", "/usr/sbin/clear", "/bin/clear"}},
		{"/usr/bin/python2", []string{"/usr/bin/python2.7"}},
		{"/usr/bin/perl", []string{"/usr/bin/perl5.38", "/usr/bin/perl5.36"}},
		{"/usr/bin/pwd", []string{"/bin/pwd", "/usr/bin/pwd"}},
		{"/usr/bin/vi", []string{"/usr/bin/vim", "/usr/bin/nvim", "/bin/vi"}},
		{"/usr/bin/vim", []string{"/usr/bin/vim", "/usr/bin/nvim", "/bin/vi"}},
	}

	for _, link := range commonLinks {
		// 检查链接是否存在且是符号链接
		linkInfo, err := os.Lstat(link.linkPath)
		if err != nil {
			continue // 链接不存在，跳过
		}
		if linkInfo.Mode()&os.ModeSymlink == 0 {
			continue // 不是符号链接，跳过
		}

		// 检查链接目标是否存在
		target, err := os.Readlink(link.linkPath)
		if err != nil {
			continue
		}

		// 如果目标是绝对路径，检查目标是否存在
		if target[0] == '/' {
			if _, err := os.Stat(target); os.IsNotExist(err) {
				// 目标不存在，尝试找到正确的目标
				for _, altTarget := range link.targets {
					if _, err := os.Stat(altTarget); err == nil {
						// 找到存在的目标，修复链接
						os.Remove(link.linkPath)
						os.Symlink(altTarget, link.linkPath)
						logger.Debug("修复符号链接: %s -> %s", link.linkPath, altTarget)
						break
					}
				}
			}
		}
	}
}
