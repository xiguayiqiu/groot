package proot

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"groot/internal/cleanup"
	"groot/internal/env"
	"groot/internal/i18n"
	"groot/internal/logger"
	"groot/internal/slogan"
	"groot/internal/termux"
	"groot/internal/usercheck"
)

// validateShellPath 验证 shell 路径是否安全
func validateShellPath(shell string) bool {
	if shell == "" {
		return false
	}
	if strings.Contains(shell, "..") {
		return false
	}
	// 只允许安全字符
	for _, c := range shell {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '/' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// truncateHostname 截断 hostname 到内核限制
func truncateHostname(hostname string) string {
	const maxHostname = 63
	if len(hostname) > maxHostname {
		return hostname[:maxHostname]
	}
	return hostname
}

// Run 运行 proot 模式 - 只使用系统的 proot 命令
func Run(rootfsPath string, customShell string) error {
	logger.Info(i18n.Tf("proot.start", rootfsPath))

	// 首先查找系统的 proot 命令（使用安全的 LookPath，避免 Termux 中 SIGSYS 崩溃）
	prootPath, err := termux.SafeLookPath("proot")
	if err != nil {
		return fmt.Errorf("%s", i18n.T("proot.not_installed"))
	}
	logger.Debug(i18n.Tf("proot.found", prootPath))

	// 转换为绝对路径
	absRootfsPath := rootfsPath
	if !filepath.IsAbs(rootfsPath) {
		abs, err := filepath.Abs(rootfsPath)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("proot.abs_path_fail"), err)
		}
		absRootfsPath = abs
	}

	// 最简单的验证：检查 rootfs 是否存在
	if _, err := os.Stat(absRootfsPath); os.IsNotExist(err) {
		return fmt.Errorf("%s", i18n.Tf("proot.rootfs_not_exist", absRootfsPath))
	}

	// 从 rootfs 的 /etc/passwd 中读取用户信息（包括 shell）
	userInfo, err := usercheck.CheckUser(absRootfsPath, "root")
	if err != nil {
		logger.Warn(i18n.Tf("proot.passwd_fail", err))
		userInfo = &usercheck.UserInfo{
			Username: "root",
			Uid:      0,
			Gid:      0,
			Home:     "/root",
			Shell:    "/bin/bash",
		}
	}

	// 确定要使用的 shell - 与 chroot 模式相同的逻辑
	shell := findShell(absRootfsPath, customShell, userInfo.Shell)

	// 验证 shell 路径安全性
	if !validateShellPath(shell) {
		logger.Warn("shell 路径不安全: %s, 回退到 /bin/sh", shell)
		shell = "/bin/sh"
	}

	// 终极方案：完整修复整个 rootfs 权限，proot 里的 root = 宿主机的当前用户
	currentUid := os.Getuid()
	currentGid := os.Getgid()
	logger.Debug("终极方案：修复整个 rootfs 权限")
	fixUltimatePermissions(absRootfsPath, currentUid, currentGid)

	// 在主机端清理 rootfs 中的宿主机环境痕迹
	preHostname := env.GetHostname(absRootfsPath, cleanup.DefaultHostname)
	if err := cleanup.CleanupRootfs(absRootfsPath, preHostname); err != nil {
		logger.Warn("清理 rootfs 环境失败: %v", err)
	}

	// 获取正确的 hostname
	hostname := env.GetHostname(absRootfsPath, "groot-proot")
	hostname = truncateHostname(hostname)
	logger.Debug("使用主机名: %s", hostname)

	// 构建 proot 命令参数 - 以 login shell 方式启动，自动 source /etc/profile
	// 启动前打印彩色广告横幅
	// 设置 ENV=/etc/profile 让 bash 在非登录模式下也读取 profile
	// 使用 -l 参数让 bash 作为 login shell 启动，读取 /etc/profile, ~/.bash_profile, ~/.profile
	execCmd := "export ENV=/etc/profile; " + slogan.GetBannerCmd() + "; exec " + shell + " -l"
	args := []string{
		"--kill-on-exit",
		"-0",
		"-r", absRootfsPath,
		"-w", "/",
		"-b", "/dev",
		"-b", "/proc",
		"-b", "/sys",
		"-b", "/tmp",
		"/bin/sh",
		"-c", execCmd,
	}

	logger.Info(i18n.Tf("proot.exec_cmd", prootPath, args))

	cmd := exec.Command(prootPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 使用统一的环境变量，传入 rootfs 来检测发行版
	envVars := env.SetupEnv(userInfo, hostname, absRootfsPath)
	cmd.Env = envVars

	// 确保 SHELL 变量正确设置
	for i, e := range cmd.Env {
		if strings.HasPrefix(e, "SHELL=") {
			cmd.Env[i] = fmt.Sprintf("SHELL=%s", shell)
			break
		}
	}

	// 信号转发
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	defer signal.Stop(sigChan)
	defer close(sigChan)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s", i18n.Tf("proot.start_fail", err))
	}

	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
				logger.Debug("转发信号 %v 给 proot", sig)
				if err := cmd.Process.Signal(sig); err != nil {
					logger.Debug("信号转发失败: %v", err)
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("%s", i18n.Tf("proot.run_fail", err))
	}

	return nil
}

// fixUltimatePermissions 终极修复整个 rootfs 权限
func fixUltimatePermissions(rootfsPath string, uid, gid int) {
	// Alpine 等发行版使用 busybox 硬链接，多个目录项共享同一 inode。
	// 跳过已处理的 inode 可避免重复 chmod/chown，
	// 以及因路径判定误差导致的权限覆盖问题。
	processedInodes := make(map[uint64]bool)
	filepath.Walk(rootfsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 不修改符号链接
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		// 跳过已处理的 inode，避免对硬链接重复设置权限
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			inodeKey := uint64(stat.Ino)
			if processedInodes[inodeKey] {
				return nil
			}
			processedInodes[inodeKey] = true
		}
		// 设置所有者为当前用户
		if err := os.Chown(path, uid, gid); err != nil {
			logger.Debug("chown %s failed: %v", path, err)
		}
		// 设置权限
		if info.IsDir() {
			if err := os.Chmod(path, 0755); err != nil {
				logger.Debug("chmod %s failed: %v", path, err)
			}
		} else {
			// 检查是否是可执行文件
			mode := info.Mode()
			relPath, err := filepath.Rel(rootfsPath, path)
			isExecDir := false
			if err == nil {
				relPath = "/" + relPath
				isExecDir = strings.HasPrefix(relPath, "/bin/") ||
					strings.HasPrefix(relPath, "/sbin/") ||
					strings.HasPrefix(relPath, "/usr/bin/") ||
					strings.HasPrefix(relPath, "/usr/sbin/") ||
					strings.HasPrefix(relPath, "/lib/") ||
					strings.HasPrefix(relPath, "/usr/lib/") ||
					strings.HasPrefix(relPath, "/lib64/") ||
					strings.HasPrefix(relPath, "/usr/lib64/")
			}
			if mode&0111 != 0 || isExecDir {
				if err := os.Chmod(path, 0755); err != nil {
					logger.Debug("chmod %s failed: %v", path, err)
				}
			} else {
				if err := os.Chmod(path, 0644); err != nil {
					logger.Debug("chmod %s failed: %v", path, err)
				}
			}
		}
		return nil
	})
}

// findShell 查找可用的 shell 解释器
func findShell(rootfsPath, customShell, defaultShell string) string {
	// 优先检查用户指定的 shell
	if customShell != "" {
		// 安全检查：防止路径遍历
		if strings.Contains(customShell, "..") {
			logger.Warn("customShell 包含路径遍历: %s", customShell)
		} else {
			shellPath := customShell
			if customShell[0] != '/' {
				shellPath = "/" + customShell
			}
			fullPath := filepath.Join(rootfsPath, shellPath)
			if _, err := os.Stat(fullPath); err == nil {
				return shellPath
			}
			// 尝试常见的 shell 路径
			for _, path := range []string{"/bin/" + customShell, "/usr/bin/" + customShell} {
				fullPath := filepath.Join(rootfsPath, path)
				if _, err := os.Stat(fullPath); err == nil {
					return path
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

	// 按优先级查找可用的 shell
	shells := []string{"/bin/dash", "/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
	for _, s := range shells {
		fullPath := filepath.Join(rootfsPath, s)
		if _, err := os.Stat(fullPath); err == nil {
			return s
		}
	}

	return "/bin/sh"
}
