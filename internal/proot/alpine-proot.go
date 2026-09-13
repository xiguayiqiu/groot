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

// RunAlpineProot 专门为 alpine 量身定做的版本
func RunAlpineProot(rootfsPath string, customShell string) error {
	logger.Info(i18n.Tf("alpine.start", rootfsPath))

	// 查找系统 proot（使用安全的 LookPath，避免 Termux 中 SIGSYS 崩溃）
	prootPath, err := termux.SafeLookPath("proot")
	if err != nil {
		return fmt.Errorf("%s", i18n.T("alpine.no_proot"))
	}

	// 转换绝对路径
	absRootfsPath := rootfsPath
	if !filepath.IsAbs(rootfsPath) {
		abs, err := filepath.Abs(rootfsPath)
		if err != nil {
			return err
		}
		absRootfsPath = abs
	}

	// 检查这是不是 alpine
	if _, err := os.Stat(filepath.Join(absRootfsPath, "etc", "alpine-release")); os.IsNotExist(err) {
		logger.Warn(i18n.T("alpine.not_alpine"))
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

	// 权限修复和清理（root 时执行）
	if currentUid == 0 {
		logger.Info(i18n.T("alpine.fix_rootfs_perm"))
		fixAlpineRootfs(absRootfsPath, currentUid, os.Getgid())
	}

	// 清理宿主机痕迹
	preHostname := env.GetHostname(absRootfsPath, cleanup.DefaultHostname)
	if err := cleanup.CleanupRootfs(absRootfsPath, preHostname); err != nil {
		logger.Warn(i18n.Tf("alpine.cleanup_fail", err))
	}

	// 从 /etc/passwd 中读取用户信息
	userInfo, err := usercheck.CheckUser(absRootfsPath, "root")
	if err != nil {
		logger.Warn(i18n.Tf("alpine.cannot_read_passwd", err))
		userInfo = &usercheck.UserInfo{
			Username: "root",
			Uid:      0,
			Gid:      0,
			Home:     "/root",
			Shell:    "/bin/sh",
		}
	}

	// 确定 shell
	shell := findShell(absRootfsPath, customShell, userInfo.Shell)

	// 验证 shell 路径安全性
	if !validateShellPath(shell) {
		logger.Warn("shell 路径不安全: %s, 回退到 /bin/sh", shell)
		shell = "/bin/sh"
	}

	// 构建 proot 命令参数
	execCmd := "export PATH=/sbin:/usr/sbin:/bin:/usr/bin; export ENV=/etc/profile; " + slogan.GetBannerCmd() + "; exec " + shell + " -l"
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

	logger.Info(i18n.Tf("alpine.exec_cmd", prootPath, args))

	cmd := exec.Command(prootPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 使用统一的环境变量，传入 rootfs 来检测发行版
	envVars := env.SetupEnv(userInfo, truncateHostname(env.GetHostname(absRootfsPath, "groot-proot")), absRootfsPath)
	cmd.Env = envVars

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	defer signal.Stop(sigChan)
	defer close(sigChan)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
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
		return err
	}

	return nil
}

func fixAlpineRootfs(rootfsPath string, uid, gid int) {
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
			relPath, err := filepath.Rel(rootfsPath, path)
			isExec := false
			if err == nil {
				relPath = "/" + relPath
				isExec = strings.HasPrefix(relPath, "/bin/") ||
					strings.HasPrefix(relPath, "/sbin/") ||
					strings.HasPrefix(relPath, "/usr/bin/") ||
					strings.HasPrefix(relPath, "/usr/sbin/") ||
					strings.HasPrefix(relPath, "/lib/") ||
					strings.HasPrefix(relPath, "/usr/lib/") ||
					strings.HasPrefix(relPath, "/lib64/") ||
					strings.HasPrefix(relPath, "/usr/lib64/")
			}
			mode := info.Mode()
			if mode&0111 != 0 || isExec {
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
