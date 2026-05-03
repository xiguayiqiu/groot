package proot

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"groot/internal/logger"
	"groot/internal/usercheck"
)

// RunUbuntuProot 专门为 ubuntu 量身定做的 100% 完美版本！！！
func RunUbuntuProot(rootfsPath string, customShell string) error {
	logger.Info("Ubuntu 专属模式启动！rootfs: %s", rootfsPath)

	// 查找系统 proot
	prootPath, err := exec.LookPath("proot")
	if err != nil {
		return fmt.Errorf("没找到 proot：sudo apt install proot")
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

	// 检查这是不是 ubuntu 或 debian
	if _, err := os.Stat(filepath.Join(absRootfsPath, "etc", "ubuntu-release")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(absRootfsPath, "etc", "debian_version")); os.IsNotExist(err) {
			logger.Warn("这看起来不是 ubuntu 或 debian，不过继续尝试")
		}
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
		// 先检查 rootfs 里有没有 /bin/bash，如果没有则用 /bin/sh
		if _, err := os.Stat(filepath.Join(absRootfsPath, "bin/bash")); err == nil {
			shell = "/bin/bash"
		} else {
			shell = "/bin/sh"
		}
	}

	// 终极方案：完整修复整个 rootfs 权限，proot 里的 root = 宿主机的当前用户
	currentUid := os.Getuid()
	currentGid := os.Getgid()
	logger.Debug("终极方案：修复整个 rootfs 权限")
	fixUltimatePermissions(absRootfsPath, currentUid, currentGid)

	// 检测是否有 cargo 风格 coreutils（通过检查 /lib/cargo/bin/coreutils 目录是否存在）
	// 也检查 .bak 版本（可能在之前运行中已被移动）
	hasCargoCoreutils := false
	if _, err := os.Stat(filepath.Join(absRootfsPath, "lib/cargo/bin/coreutils")); err == nil {
		hasCargoCoreutils = true
		logger.Warn("检测到 cargo 风格 coreutils，尝试通过挂载 host 二进制绕过...")
	} else if _, err := os.Stat(filepath.Join(absRootfsPath, "lib/cargo/bin/coreutils.bak")); err == nil {
		// 已经处理过了，使用之前修复的配置
		hasCargoCoreutils = true
		logger.Info("检测到已处理的 cargo coreutils.bak，使用兼容模式")
	}

	// 构建 proot 命令参数
	var args []string
	var env []string
	if hasCargoCoreutils {
		// 方案：将 rootfs 中的 cargo bin 目录移动到备份位置
		// 这样 proot 的 loader 就不会被拦截
		cargoDir := filepath.Join(absRootfsPath, "lib/cargo/bin/coreutils")
		backupDir := filepath.Join(absRootfsPath, "lib/cargo/bin/coreutils.bak")
		
		if _, err := os.Stat(cargoDir); err == nil {
			if _, err := os.Stat(backupDir); os.IsNotExist(err) {
				logger.Info("备份 cargo coreutils 到: %s", backupDir)
				if err := os.Rename(cargoDir, backupDir); err != nil {
					logger.Warn("备份失败: %v，尝试删除", err)
					os.RemoveAll(cargoDir)
				}
			}
		}
		
		// 删除指向 cargo 目录的损坏符号链接，并创建挂载点
		binDir := filepath.Join(absRootfsPath, "bin")
		usrBinDir := filepath.Join(absRootfsPath, "usr/bin")
		
		// 删除指向已移动的 cargo 目录的符号链接
		cargoPrefix := "../lib/cargo/bin/coreutils"
		filepath.Walk(binDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if target, err := os.Readlink(path); err == nil {
					if strings.HasPrefix(target, cargoPrefix) {
						os.Remove(path)
						logger.Debug("删除损坏的符号链接: %s -> %s", path, target)
					}
				}
			}
			return nil
		})
		
		// 同样处理 /usr/bin
		filepath.Walk(usrBinDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if target, err := os.Readlink(path); err == nil {
					if strings.HasPrefix(target, cargoPrefix) {
						os.Remove(path)
					}
				}
			}
			return nil
		})
		
		logger.Info("已清理损坏的符号链接，将通过 PATH 查找命令")
		
		logger.Info("已清理损坏的符号链接，使用 sh 作为 shell")
		
		// 使用 sh 因为 bash 的符号链接已损坏
		// 挂载 host 的 /bin 和基本库
		args = []string{
			"--kill-on-exit",
			"-0",
			"-r", absRootfsPath,
			"-w", "/",
			"-b", "/dev",
			"-b", "/proc",
			"-b", "/sys",
			"-b", "/tmp",
			"-b", "/bin:/bin",
			"-b", "/lib:/lib",
			"-b", "/lib64:/lib64",
			"/bin/sh",
		}
		// 设置正确的 PATH
		env = []string{
			"PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin",
			"TERM=" + os.Getenv("TERM"),
			"HOME=/root",
			"SHELL=/bin/sh",
		}
	} else {
		// 标准方式
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
		env = []string{
			"PATH=/sbin:/usr/sbin:/bin:/usr/bin:/usr/local/bin:/usr/local/sbin",
			"TERM=" + os.Getenv("TERM"),
			"HOME=/root",
			"SHELL=" + shell,
		}
	}

	logger.Info("执行 proot 命令: %s %v", prootPath, args)

	cmd := exec.Command(prootPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 设置环境变量
	cmd.Env = env

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
