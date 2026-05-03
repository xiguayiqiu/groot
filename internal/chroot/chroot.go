package chroot

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"groot/internal/env"
	"groot/internal/logger"
	"groot/internal/mount"
	"groot/internal/permission"
	"groot/internal/usercheck"
)

// Run 运行 chroot 模式
func Run(rootfsPath string, customShell string) error {
	// 转换为绝对路径
	absRootfsPath, err := filepath.Abs(rootfsPath)
	if err != nil {
		return fmt.Errorf("转换绝对路径失败: %w", err)
	}

	logger.Info("开始 chroot 模式，rootfs 路径: %s", absRootfsPath)

	// 验证 rootfs
	if err := mount.ValidateRootfs(absRootfsPath, customShell); err != nil {
		return err
	}

	// 检查权限
	isRoot := permission.IsRoot()
	if isRoot {
		logger.Info("以真实 root 权限运行")
	} else {
		logger.Info("以普通用户运行，使用 User Namespace 隔离")
	}

	// 默认使用 root 用户
	userInfo, err := usercheck.CheckUser(absRootfsPath, "root")
	if err != nil {
		return err
	}

	// 在主进程修复权限
	logger.Info("正在修复权限...")
	usercheck.FixCommonIssues(absRootfsPath, userInfo)

	// 准备用户主目录
	mount.PrepareUserHome(absRootfsPath, userInfo, false)

	// 使用可执行文件路径
	exePath := permission.GetExecutablePath()
	logger.Debug("使用可执行文件: %s", exePath)

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

	logger.Info("创建子进程并启用命名空间隔离...")

	// 信号转发
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动子进程失败: %w", err)
	}

	// 后台转发信号
	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
				logger.Debug("转发信号 %v 给子进程", sig)
				cmd.Process.Signal(sig)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("子进程执行失败: %w", err)
	}

	signal.Stop(sigChan)
	close(sigChan)

	return nil
}

// ChildMain chroot 子进程入口
func ChildMain(rootfsPath string, customShell string, customUser string) error {
	logger.Info("进入 chroot 子进程")

	// 首先设置主机名（在 UTS namespace 中）
	hostname := env.GetHostname(rootfsPath, "groot")
	logger.Debug("设置主机名为: %s", hostname)
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		logger.Warn("设置主机名失败: %v", err)
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
	logger.Info("正在挂载虚拟文件系统...")
	mounted, err := mount.MountAll(rootfsPath)
	if err != nil {
		return err
	}
	logger.Info("虚拟文件系统挂载成功: %v", mounted)

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
			logger.Warn("卸载失败: %v", err)
		}
		logger.Info("虚拟文件系统已卸载")
	}()

	// 直接使用 syscall.Chroot，不搞复杂的包装
	logger.Debug("执行 syscall.Chroot 到 %s", rootfsPath)
	if err := syscall.Chroot(rootfsPath); err != nil {
		return fmt.Errorf("chroot 失败: %w", err)
	}

	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir 失败: %w", err)
	}

	// 设置环境变量
	os.Clearenv()
	for _, e := range newEnv {
		parts := []rune(e)
		eqIndex := -1
		for i, r := range parts {
			if r == '=' {
				eqIndex = i
				break
			}
		}
		if eqIndex != -1 {
			key := string(parts[:eqIndex])
			value := string(parts[eqIndex+1:])
			_ = os.Setenv(key, value)
		}
	}

// 告诉 dpkg 不需要 systemd
	_ = os.Setenv("SYSTEMD_IN_DOCKER", "1")
	_ = os.Setenv("DEBIAN_FRONTEND", "noninteractive")
	_ = os.Setenv("SYSTEMD_IGNORE_ENVIRONMENT", "1")

	// 创建假的 systemd 目录结构，让 dpkg 脚本认为 systemd 存在
	os.MkdirAll("/run/systemd/system", 0755)
	os.MkdirAll("/run/systemd/private", 0700)
	os.MkdirAll("/run/dbus", 0755)
	
	// 创建必要的文件来欺骗脚本
	os.Create("/run/systemd/container")
	os.Create("/run/systemd/systemd-logind")
	
	// 创建一个 fake systemd 二进制来处理基本命令
	fakeSystemd := `#!/bin/bash
exit 0`
	os.WriteFile("/usr/bin/systemd-run", []byte(fakeSystemd), 0755)

	// 直接执行 shell，先 source 环境！
	logger.Info("启动 shell: %s", shell)
	fixScript := "if [ -f /var/lib/dpkg/info/libc6:amd64.postinst ]; then " +
		"grep -q 'Groot: skip' /var/lib/dpkg/info/libc6:amd64.postinst 2>/dev/null || " +
		"(cp /var/lib/dpkg/info/libc6:amd64.postinst /var/lib/dpkg/info/libc6:amd64.postinst.bak 2>/dev/null; " +
		"echo '#!/bin/bash'; echo 'exit 0' > /var/lib/dpkg/info/libc6:amd64.postinst; " +
		"chmod +x /var/lib/dpkg/info/libc6:amd64.postinst); fi"
	var cmd *exec.Cmd
	cmd = exec.Command("/bin/sh", "-c", fixScript+"; export PATH=/sbin:/usr/sbin:/bin:/usr/bin; SHELL="+shell+" exec "+shell)
	cmd.Env = newEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = userInfo.Home

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 shell 失败: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("shell 运行失败: %w", err)
	}

	return nil
}

// findShell 查找可用的 shell 解释器
func findShell(rootfsPath, customShell, defaultShell string) string {
	// 优先检查用户指定的 shell
	if customShell != "" {
		shellPath := customShell
		if customShell[0] != '/' {
			shellPath = "/" + customShell
		}
		if _, err := os.Stat(shellPath); err == nil {
			return shellPath
		}
		// 尝试常见的 shell 路径
		for _, path := range []string{"/bin/" + customShell, "/usr/bin/" + customShell} {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// 使用用户默认 shell
	if defaultShell != "" && defaultShell != "/usr/bin/nologin" {
		if _, err := os.Stat(defaultShell); err == nil {
			return defaultShell
		}
	}

	// 按优先级查找可用的 shell
	shells := []string{"/bin/dash", "/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
	for _, s := range shells {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}

	return "/bin/sh"
}
