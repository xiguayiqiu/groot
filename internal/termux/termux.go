// Package termux 提供 Termux 环境检测和适配功能。
//
// Termux 是 Android 上的终端模拟器，其文件系统布局和运行环境与标准 Linux 不同。
// 本包提供：
//   - 检测当前环境是否为 Termux 或基于 Termux 的改版
//   - 检测 Android 设备是否已 root
//   - 清理 Termux 环境变量（如 LD_PRELOAD）以避免与 chroot/proot 冲突
package termux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"litevm/internal/slogan"

	"litevm/internal/i18n"
	"litevm/internal/logger"
)

// IsTermux 检测当前环境是否是 Termux 或基于 Termux 的改版。
// 判断依据：TERMUX_VERSION 环境变量和 PREFIX 环境变量。
func IsTermux() bool {
	if _, ok := os.LookupEnv("TERMUX_VERSION"); ok {
		return true
	}
	prefix := os.Getenv("PREFIX")
	if prefix != "" && strings.Contains(prefix, "com.termux") {
		return true
	}
	// 某些 Termux 改版可能不设 TERMUX_VERSION，检查 PREFIX 下是否有 termux 特有文件
	if prefix != "" {
		if _, err := os.Stat(filepath.Join(prefix, "bin", "termux-info")); err == nil {
			return true
		}
	}
	return false
}

// IsRooted 检测 Android 设备是否具有 root 权限。
// 返回 true 表示可以执行 chroot 等需要 root 的操作。
func IsRooted() bool {
	// 方法 1: 检查 su 是否存在且可执行
	suPaths := []string{"/system/bin/su", "/system/xbin/su", "/sbin/su", "/su/bin/su"}
	for _, p := range suPaths {
		if info, err := os.Stat(p); err == nil && info.Mode()&0111 != 0 {
			return true
		}
	}

	// 方法 2: 检查 PATH 中是否有 su
	if path := os.Getenv("PATH"); path != "" {
		for _, dir := range filepath.SplitList(path) {
			if info, err := os.Stat(filepath.Join(dir, "su")); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
				return true
			}
		}
	}

	// 方法 3: 尝试执行 id 并检查输出是否包含 uid=0（当前进程本身就是 root）
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		return true
	}

	// 方法 4: 尝试通过 su -c id 检测
	cmd := exec.Command("/system/bin/sh", "-c", "su -c id 2>/dev/null")
	output, err := cmd.Output()
	if err == nil && strings.Contains(string(output), "uid=0") {
		return true
	}

	return false
}

// UnsetLDPreloadHelper 返回一个 sh -c 包装函数，适合在 Termux 环境下调用 proot/chroot 前清除 LD_PRELOAD。
// Termux 默认设置了 LD_PRELOAD（指向 libtermux-exec.so），
// 这个库在 chroot/proot 内部会破坏容器内程序的执行，
// 必须在启动 proot/chroot 前 unset。
//
// 返回的是可在主机端执行的命令字符串前缀。
func GetUnsetLDPreloadCmd() string {
	// 直接在 exec.Command 层面 unset，不需要通过 shell 字符串
	return ""
}

// SafeLookPath 使用 os.Stat + PATH 遍历查找可执行文件，避免使用 exec.LookPath。
//
// Termux 的 proot 嵌套环境中 seccomp 拦截了 faccessat2 系统调用，
// 而 exec.LookPath 底层使用了 Faccessat → faccessat2，会导致 SIGSYS 崩溃。
// SafeLookPath 用 os.Stat（仅需 access 系统调用）替代，在 Termux 环境中正常工作。
func SafeLookPath(name string) (string, error) {
	// 如果 name 已经是路径，直接检查
	if strings.Contains(name, "/") {
		if info, err := os.Stat(name); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			abs, _ := filepath.Abs(name)
			return abs, nil
		}
		return "", fmt.Errorf("%s", i18n.Tf("termux.lookpath_not_found", name))
	}

	// 在 PATH 中搜索
	path := os.Getenv("PATH")
	if path == "" {
		return "", fmt.Errorf("%s", i18n.Tf("termux.lookpath_empty_path", name))
	}
	for _, dir := range filepath.SplitList(path) {
		fullPath := filepath.Join(dir, name)
		if info, err := os.Stat(fullPath); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return filepath.Clean(fullPath), nil
		}
	}
	return "", fmt.Errorf("%s", i18n.Tf("termux.lookpath_not_in_path", name))
}

// Command 等价于 exec.Command，但会先用 SafeLookPath 解析命令的绝对路径，
// 避免 exec.Command 内部对命令名调用 LookPath → faccessat2 触发 SIGSYS。
// 仅当 name 看起来是命令名（不含 "/"）时才解析，绝对路径/变量直接透传。
func Command(name string, args ...string) (*exec.Cmd, error) {
	if !strings.Contains(name, "/") {
		resolved, err := SafeLookPath(name)
		if err != nil {
			return nil, err
		}
		name = resolved
	}
	return exec.Command(name, args...), nil
}

// CommandSlice 等价于 exec.Command(args[0], args[1:]...)，但会先用
// SafeLookPath 解析 args[0] 的绝对路径，避免内部 LookPath → faccessat2 → SIGSYS。
func CommandSlice(args []string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("termux.empty_args"))
	}
	name := args[0]
	if !strings.Contains(name, "/") {
		resolved, err := SafeLookPath(name)
		if err != nil {
			return nil, err
		}
		name = resolved
	}
	return exec.Command(name, args[1:]...), nil
}

// CleanupEnv 清理当前进程环境变量中的 Termux 残留，返回清理前的 LD_PRELOAD 值。
// 在 Termux 环境下启动 chroot/proot 之前调用。
func CleanupEnv() string {
	oldLDPreload := os.Getenv("LD_PRELOAD")
	if oldLDPreload != "" {
		logger.Debug(fmt.Sprintf(i18n.Tf("termux.ld_preload_clean", "%s"), oldLDPreload))
		os.Unsetenv("LD_PRELOAD")
	}
	return oldLDPreload
}

// RestoreEnv 恢复被 CleanupEnv 清理的环境变量。
func RestoreEnv(oldLDPreload string) {
	if oldLDPreload != "" {
		os.Setenv("LD_PRELOAD", oldLDPreload)
	}
}

// RunWithCleanEnv 在 Termux 环境下，清理 LD_PRELOAD 后执行 fn，执行完毕后恢复。
// 对非 Termux 环境直接执行 fn。
func RunWithCleanEnv(fn func() error) error {
	oldLDPreload := ""
	if IsTermux() {
		oldLDPreload = CleanupEnv()
	}
	err := fn()
	if oldLDPreload != "" {
		RestoreEnv(oldLDPreload)
	}
	return err
}

// EnsureProotInstalled 检测 proot 是否已安装，未安装则自动在 Termux 中安装。
func EnsureProotInstalled() error {
	if _, err := SafeLookPath("proot"); err == nil {
		return nil
	}
	if !IsTermux() {
		return fmt.Errorf("%s", i18n.T("termux.proot_not_installed"))
	}
	logger.Warn(i18n.T("termux.proot_installing"))
	// 使用 SafeLookPath 找到 pkg 的完整路径，避免 exec.Command 内部调用 LookPath 触发 SIGSYS
	pkgPath, err := SafeLookPath("pkg")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("termux.no_pkg"), err)
	}
	cmd := exec.Command(pkgPath, "install", "-y", "proot")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("termux.proot_install_fail"), err)
	}
	logger.Warn(i18n.T("termux.proot_installed"))
	return nil
}

// EnsureChrootInstalled 检测 chroot 是否可用，未安装则自动在 Termux 中安装 proot（作为回退）。
func EnsureChrootInstalled() error {
	if _, err := SafeLookPath("chroot"); err == nil {
		return nil
	}
	if !IsTermux() {
		return fmt.Errorf("%s", i18n.T("termux.chroot_not_installed"))
	}
	// Termux 中 chroot 在 proot 包中
	logger.Warn(i18n.T("termux.chroot_installing"))
	pkgPath, err := SafeLookPath("pkg")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("termux.no_pkg"), err)
	}
	cmd := exec.Command(pkgPath, "install", "-y", "proot")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("termux.proot_install_fail"), err)
	}
	logger.Warn(i18n.T("termux.proot_installed"))
	return nil
}

// ShellBanner 返回启动 shell 前的横幅命令（打印彩色广告后 exec shell）。
func ShellBanner(shell string) string {
	bannerCmd := slogan.GetBannerCmd()
	// 带颜色转义序列的 printf，兼容 busybox sh
	return bannerCmd + "; exec " + shell + " -l"
}
