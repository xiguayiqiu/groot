package proot

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"groot/internal/env"
	"groot/internal/logger"
	"groot/internal/usercheck"
)

// Run 运行 proot 模式 - 只使用系统的 proot 命令
func Run(rootfsPath string, customShell string) error {
	logger.Info("开始 proot 模式，rootfs 路径: %s", rootfsPath)

	// 首先查找系统的 proot 命令
	prootPath, err := exec.LookPath("proot")
	if err != nil {
		return fmt.Errorf("系统未安装 proot，请先安装：sudo apt install proot")
	}
	logger.Debug("找到系统 proot: %s", prootPath)

	// 转换为绝对路径
	absRootfsPath := rootfsPath
	if !filepath.IsAbs(rootfsPath) {
		abs, err := filepath.Abs(rootfsPath)
		if err != nil {
			return fmt.Errorf("转换绝对路径失败: %w", err)
		}
		absRootfsPath = abs
	}

	// 最简单的验证：检查 rootfs 是否存在
	if _, err := os.Stat(absRootfsPath); os.IsNotExist(err) {
		return fmt.Errorf("rootfs 不存在: %s", absRootfsPath)
	}

	// 从 rootfs 的 /etc/passwd 中读取用户信息（包括 shell）
	userInfo, err := usercheck.CheckUser(absRootfsPath, "root")
	if err != nil {
		logger.Warn("无法读取 passwd 文件，使用默认用户信息: %v", err)
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

	// 终极方案：完整修复整个 rootfs 权限，proot 里的 root = 宿主机的当前用户
	currentUid := os.Getuid()
	currentGid := os.Getgid()
	logger.Debug("终极方案：修复整个 rootfs 权限")
	fixUltimatePermissions(absRootfsPath, currentUid, currentGid)

	// 获取正确的 hostname
	hostname := env.GetHostname(absRootfsPath, "groot-proot")
	logger.Debug("使用主机名: %s", hostname)

	// 构建 proot 命令参数 - 简洁版本，直接使用检测到的 shell
	var args []string
	args = []string{
		"--kill-on-exit",
		"-0",
		"-r", absRootfsPath,
		"-w", "/",
		"-b", "/dev",
		"-b", "/proc",
		"-b", "/sys",
		"-b", "/tmp",
		shell,
	}

	logger.Info("执行 proot 命令: %s %v", prootPath, args)

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
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 proot 失败: %w", err)
	}

	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
				logger.Debug("转发信号 %v 给 proot", sig)
				cmd.Process.Signal(sig)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("proot 运行失败: %w", err)
	}

	signal.Stop(sigChan)
	close(sigChan)

	return nil
}

// fixUltimatePermissions 终极修复整个 rootfs 权限
func fixUltimatePermissions(rootfsPath string, uid, gid int) {
	filepath.Walk(rootfsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 不修改符号链接
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		// 设置所有者为当前用户
		os.Chown(path, uid, gid)
		// 设置权限
		if info.IsDir() {
			os.Chmod(path, 0755)
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
				os.Chmod(path, 0755)
			} else {
				os.Chmod(path, 0644)
			}
		}
		return nil
	})
}

// findShell 查找可用的 shell 解释器
func findShell(rootfsPath, customShell, defaultShell string) string {
	// 优先检查用户指定的 shell
	if customShell != "" {
		shellPath := customShell
		if customShell[0] != '/' {
			shellPath = "/" + customShell
		}
		fullPath := filepath.Join(rootfsPath, shellPath[1:])
		if _, err := os.Stat(fullPath); err == nil {
			return shellPath
		}
		// 尝试常见的 shell 路径
		for _, path := range []string{"/bin/" + customShell, "/usr/bin/" + customShell} {
			fullPath := filepath.Join(rootfsPath, path[1:])
			if _, err := os.Stat(fullPath); err == nil {
				return path
			}
		}
	}

	// 使用用户默认 shell
	if defaultShell != "" && defaultShell != "/usr/bin/nologin" {
		fullPath := filepath.Join(rootfsPath, defaultShell[1:])
		if _, err := os.Stat(fullPath); err == nil {
			return defaultShell
		}
	}

	// 按优先级查找可用的 shell
	shells := []string{"/bin/dash", "/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
	for _, s := range shells {
		fullPath := filepath.Join(rootfsPath, s[1:])
		if _, err := os.Stat(fullPath); err == nil {
			return s
		}
	}

	return "/bin/sh"
}
