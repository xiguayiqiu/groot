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

// isRootOwned 检查 rootfs 是否由 root 拥有
func isRootOwned(rootfsPath string) bool {
	info, err := os.Stat(rootfsPath)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Uid == 0
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

	// 非 root 用户 + root 拥有的 rootfs：提示修复权限
	currentUid := os.Getuid()
	needsPermFix := currentUid != 0 && isRootOwned(absRootfsPath)
	if needsPermFix {
		relPath := absRootfsPath
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, absRootfsPath); err == nil {
				relPath = rel
			}
		}
		logger.Warn(i18n.T("proot.perm_owned_by_root"))
		logger.Warn("  → sudo find %s/ -mindepth 1 -maxdepth 1 ! -name proc ! -name sys -exec chown -R %d:%d {} +",
			relPath, currentUid, os.Getgid())
	}

	// 权限修复（root 或已修复后生效）
	if !needsPermFix {
		fixUltimatePermissions(absRootfsPath, currentUid, os.Getgid())
	}

	// 修复 dpkg 状态文件（之前 --link2symlink 可能损坏了符号链接）
	fixDpkgStatusFiles(absRootfsPath)

	// 清理宿主机痕迹（仅 root 或非 root 拥有时才需要——后者清理可能部分失败）
	preHostname := env.GetHostname(absRootfsPath, cleanup.DefaultHostname)
	if err := cleanup.CleanupRootfs(absRootfsPath, preHostname); err != nil {
		logger.Warn("清理 rootfs 环境失败: %v", err)
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

	// 确定要使用的 shell
	shell := findShell(absRootfsPath, customShell, userInfo.Shell)

	// 验证 shell 路径安全性
	if !validateShellPath(shell) {
		logger.Warn("shell 路径不安全: %s, 回退到 /bin/sh", shell)
		shell = "/bin/sh"
	}

	// 获取正确的 hostname
	hostname := env.GetHostname(absRootfsPath, "groot-proot")
	hostname = truncateHostname(hostname)
	logger.Debug("使用主机名: %s", hostname)

	// 构建 proot 命令参数
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
	// Termux 需要 --link2symlink 处理硬链接
	if termux.IsTermux() {
		args = append(args[:1], append([]string{"--link2symlink"}, args[1:]...)...)
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

// fixUltimatePermissions 修复整个 rootfs 权限
func fixUltimatePermissions(rootfsPath string, uid, gid int) {
	processedInodes := make(map[uint64]bool)
	filepath.Walk(rootfsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			inodeKey := uint64(stat.Ino)
			if processedInodes[inodeKey] {
				return nil
			}
			processedInodes[inodeKey] = true
		}
		if err := os.Chown(path, uid, gid); err != nil {
			logger.Debug("chown %s failed: %v", path, err)
		}
		if info.IsDir() {
			if err := os.Chmod(path, 0755); err != nil {
				logger.Debug("chmod %s failed: %v", path, err)
			}
		} else {
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
	if customShell != "" {
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
			for _, path := range []string{"/bin/" + customShell, "/usr/bin/" + customShell} {
				fullPath := filepath.Join(rootfsPath, path)
				if _, err := os.Stat(fullPath); err == nil {
					return path
				}
			}
		}
	}

	if defaultShell != "" && defaultShell != "/usr/bin/nologin" {
		fullPath := filepath.Join(rootfsPath, defaultShell)
		if _, err := os.Stat(fullPath); err == nil {
			return defaultShell
		}
	}

	shells := []string{"/bin/dash", "/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
	for _, s := range shells {
		fullPath := filepath.Join(rootfsPath, s)
		if _, err := os.Stat(fullPath); err == nil {
			return s
		}
	}

	return "/bin/sh"
}

// fixDpkgStatusFiles 修复 --link2symlink 损坏的 dpkg 状态文件
func fixDpkgStatusFiles(rootfsPath string) {
	dpkgDir := filepath.Join(rootfsPath, "var/lib/dpkg")
	if _, err := os.Stat(dpkgDir); os.IsNotExist(err) {
		return
	}

	// 修复 status 和 status-old：如果是指向同一目标的符号链接，还原为真实文件
	for _, name := range []string{"status", "status-old"} {
		path := filepath.Join(dpkgDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		// 是符号链接，读取目标
		target, err := os.Readlink(path)
		if err != nil {
			continue
		}
		// 如果目标不存在或是另一个符号链接，修复它
		targetInfo, err := os.Stat(target)
		if err != nil || targetInfo.Mode()&os.ModeSymlink != 0 {
			// 尝试从 status-new 恢复
			newPath := filepath.Join(dpkgDir, "status-new")
			newData, readErr := os.ReadFile(newPath)
			if readErr != nil {
				// 尝试读取符号链接指向的内容
				newData, readErr = os.ReadFile(path)
			}
			os.Remove(path)
			if readErr == nil {
				os.WriteFile(path, newData, 0644)
				logger.Debug("修复 dpkg %s: 符号链接还原为真实文件", name)
			}
		}
	}

	// 清理残留的 dpkg 锁文件
	for _, lock := range []string{"lock", "lock-frontend", "lock-arch"} {
		lockPath := filepath.Join(dpkgDir, lock)
		if _, err := os.Stat(lockPath); err == nil {
			os.Remove(lockPath)
			logger.Debug("清理 dpkg 锁文件: %s", lockPath)
		}
	}
}
