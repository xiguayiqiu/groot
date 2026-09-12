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
	"groot/internal/termux"
	"groot/internal/usercheck"
)

// RunAlpineProot 专门为 alpine 量身定做的 100% 完美版本！！！
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

	// 从 rootfs 的 /etc/passwd 中读取用户信息（包括 shell）
	userInfo, err := usercheck.CheckUser(absRootfsPath, "root")
	if err != nil {
		logger.Warn(i18n.Tf("alpine.cannot_read_passwd", err))
		userInfo = &usercheck.UserInfo{
			Username: "root",
			Uid:      0,
			Gid:      0,
			Home:     "/root",
			Shell:    "/bin/bash",
		}
	}

	// 终极权限修复：确保整个 rootfs 属于当前用户！！！
	currentUid := os.Getuid()
	currentGid := os.Getgid()
	logger.Info(i18n.T("alpine.fix_rootfs_perm"))
	fixAlpineRootfs(absRootfsPath, currentUid, currentGid)

	// 在主机端清理 rootfs 中的宿主机环境痕迹
	preHostname := env.GetHostname(absRootfsPath, cleanup.DefaultHostname)
	if err := cleanup.CleanupRootfs(absRootfsPath, preHostname); err != nil {
		logger.Warn(i18n.Tf("alpine.cleanup_fail", err))
	}

	// 确定 shell - 与 chroot 模式相同的逻辑
	shell := "/bin/sh"
	if customShell != "" {
		if customShell[0] != '/' {
			shell = "/" + customShell
		} else {
			shell = customShell
		}
	} else if userInfo.Shell != "" && userInfo.Shell != "/usr/bin/nologin" {
		// 优先使用 passwd 中设置的 shell（即 chsh 设置的）
		shell = userInfo.Shell
	} else {
		// 回退到检测 bash/sh
		if _, err := os.Stat(filepath.Join(absRootfsPath, "bin/bash")); err == nil {
			shell = "/bin/bash"
		} else {
			shell = "/bin/sh"
		}
	}

	// 终极挂载参数 - 以 login shell 方式启动，自动 source /etc/profile
	// 启动前打印彩色广告横幅
	execCmd := "export PATH=/sbin:/usr/sbin:/bin:/usr/bin; export ENV=/etc/profile; printf '\\033[36m[Groot]\\033[0m \\033[32m如果你喜欢groot的话，请前往 https://gyscan.space 下载gyscan吧 [qwq]\\033[0m\\n'; exec " + shell + " -l"
	// Alpine 默认使用 busybox 硬链接（多个目录项共享同一 inode）。
	// 在 Termux/proot 环境下，文件系统可能不支持硬链接，
	// 必须通过 --link2symlink 让 proot 把 link() 转换为 symlink()，
	// 否则会出现 "command not found"（错误 127）。
	// 参考：termux/proot#57, termux/proot#284, Alpine Linux 论坛
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
		"/bin/sh",
		"-c", execCmd,
	}
	if termux.IsTermux() {
		// Termux/proot 需要 --link2symlink 处理 Alpine 的 busybox 硬链接
		args = append(args[:1], append([]string{"--link2symlink"}, args[1:]...)...)
	}

	logger.Info(i18n.Tf("alpine.exec_cmd", prootPath, args))

	cmd := exec.Command(prootPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 环境变量：简单干净
	cmd.Env = []string{
		"PATH=/sbin:/usr/sbin:/bin:/usr/bin",
		"TERM=" + os.Getenv("TERM"),
		"HOME=/root",
		"SHELL=" + shell,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
				cmd.Process.Signal(sig)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	signal.Stop(sigChan)
	close(sigChan)
	return nil
}

func fixAlpineRootfs(rootfsPath string, uid, gid int) {
	// Alpine 默认使用 busybox 硬链接：多个目录项（/bin/sh、/bin/cat、
	// /bin/ls 等）共享同一 inode。filepath.Walk 会访问每个硬链接路径，
	// 导致对同一 inode 重复执行 chmod/chown。此外，若某个硬链接位于
	// 非可执行目录，错误的 0644 权限会覆盖整个 inode，使 busybox 失效。
	processedInodes := make(map[uint64]bool)
	filepath.Walk(rootfsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
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

		os.Chown(path, uid, gid)
		if info.IsDir() {
			os.Chmod(path, 0755)
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
				os.Chmod(path, 0755)
			} else {
				os.Chmod(path, 0644)
			}
		}
		return nil
	})
}
