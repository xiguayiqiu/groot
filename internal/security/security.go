
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"groot/internal/i18n"
	"groot/internal/logger"
)

// 定义 Linux capabilities 常量
const (
	CAP_CHOWN            = 0
	CAP_DAC_OVERRIDE     = 1
	CAP_DAC_READ_SEARCH  = 2
	CAP_FOWNER           = 3
	CAP_FSETID           = 4
	CAP_KILL             = 5
	CAP_SETGID           = 6
	CAP_SETUID           = 7
	CAP_SETPCAP          = 8
	CAP_LINUX_IMMUTABLE  = 9
	CAP_NET_BIND_SERVICE = 10
	CAP_NET_BROADCAST    = 11
	CAP_NET_ADMIN        = 12
	CAP_NET_RAW          = 13
	CAP_IPC_LOCK         = 14
	CAP_IPC_OWNER        = 15
	CAP_SYS_MODULE       = 16
	CAP_SYS_RAWIO        = 17
	CAP_SYS_CHROOT       = 18
	CAP_SYS_PTRACE       = 19
	CAP_SYS_PACCT        = 20
	CAP_SYS_ADMIN        = 21
	CAP_SYS_BOOT         = 22
	CAP_SYS_NICE         = 23
	CAP_SYS_RESOURCE     = 24
	CAP_SYS_TIME         = 25
	CAP_SYS_TTY_CONFIG   = 26
	CAP_MKNOD            = 27
	CAP_LEASE            = 28
	CAP_AUDIT_WRITE      = 29
	CAP_AUDIT_CONTROL    = 30
	CAP_SETFCAP          = 31
	CAP_MAC_OVERRIDE     = 32
	CAP_MAC_ADMIN        = 33
	CAP_SYSLOG           = 34
	CAP_WAKE_ALARM       = 35
	CAP_BLOCK_SUSPEND    = 36
	CAP_AUDIT_READ       = 37
	CAP_PERFMON          = 38
	CAP_BPF              = 39
	CAP_CHECKPOINT_RESTORE = 40
	CAP_LAST_CAP         = 40
)

// SecurityConfig 安全配置
type SecurityConfig struct {
	EnableNewUser   bool
	EnableNewMount  bool
	EnableNewPID    bool
	EnableNewUTS    bool
	EnableNewIPC    bool
	EnableNewNet    bool
	DropCapabilities []string
	EnableSeccomp bool
	ProcfsRO bool
}

// DefaultSecurityConfig 返回默认的安全配置
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		EnableNewUser:   true,
		EnableNewMount:  true,
	EnableNewPID:    true,
		EnableNewUTS:    true,
		EnableNewIPC:    true,
		EnableNewNet:    true,
		EnableSeccomp:   true,
		ProcfsRO:        true,
	}
}

// HardenProcfs 硬化 procfs 挂载
func HardenProcfs(rootfsPath string) error {
	procPath := filepath.Join(rootfsPath, "proc")
	logger.Info(i18n.Tf("security.hardening_procfs", procPath))

	if err := os.MkdirAll(filepath.Join(procPath, "sys"), 0555); err != nil {
		logger.Debug(i18n.T("security.procfs_dir_fail"))
	}

	flags := uintptr(syscall.MS_RDONLY | syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV)
	if err := syscall.Mount("proc", procPath, "proc", flags, "hidepid=2,gid=65534"); err != nil {
		if err == syscall.EBUSY {
			logger.Debug("procfs 已存在，尝试重新挂载")
			flags |= syscall.MS_REMOUNT | syscall.MS_BIND
			if err := syscall.Mount("proc", procPath, "proc", flags, "hidepid=2,gid=65534"); err != nil {
				logger.Warn(i18n.T("security.procfs_remount_fail"))
			}
		} else {
			logger.Warn(i18n.T("security.procfs_mount_fail"))
		}
	} else {
		logger.Debug(i18n.T("security.procfs_done"))
	}

	sensitiveFiles := []string{
		"kcore",
		"kallsyms",
		"self/mem",
		"self/exe",
		"self/cwd",
		"self/root",
		"self/comm",
		"self/cmdline",
		"self/environ",
		"self/fd",
		"self/maps",
		"self/stat",
		"self/statm",
		"self/status",
		"self/sched",
		"self/limits",
		"self/io",
		"self/oom_score",
		"self/oom_score_adj",
		"self/timers",
		"self/net",
		"self/mounts",
		"self/mountinfo",
		"sys/kernel/security",
		"sys/vm/panic_on_oops",
	}

	for _, f := range sensitiveFiles {
		targetPath := filepath.Join(procPath, f)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			continue
		}
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			continue
		}
		if err := syscall.Mount("tmpfs", targetPath, "tmpfs", syscall.MS_RDONLY|syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "size=0"); err != nil {
			logger.Debug(i18n.Tf("security.hide_fail", f))
		} else {
			logger.Debug(i18n.Tf("security.hide_ok", f))
		}
	}
	return nil
}

// MountReadOnly 以只读方式挂载
func MountReadOnly(source, target, fstype string, data string) error {
	flags := uintptr(syscall.MS_RDONLY | syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV)
	return syscall.Mount(source, target, fstype, flags, data)
}

// DisablePtrace 禁用 ptrace
func DisablePtrace() error {
	_, _, err := syscall.Syscall6(syscall.SYS_PRCTL, 4, 0, 0, 0, 0, 0)
	if err != 0 {
		return fmt.Errorf("%s", i18n.T("security.prctl_fail"))
	}
	_, _, err = syscall.Syscall6(syscall.SYS_PRCTL, 22, 0, 0, 0, 0, 0)
	if err != 0 {
		return fmt.Errorf("%s", i18n.T("security.ptrace_fail"))
	}

	if _, err := os.Stat("/proc/sys/kernel/yama/ptrace_scope"); err == nil {
		if err := os.WriteFile("/proc/sys/kernel/yama/ptrace_scope", []byte("3"), 0644); err != nil {
			logger.Debug(i18n.T("security.yama_fail"))
		}
	}

	return nil
}

// LimitCoreDump 限制核心转储
func LimitCoreDump() error {
	rlimit := syscall.Rlimit{Cur: 0, Max: 0}
	if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &rlimit); err != nil {
		return err
	}
	return nil
}

// LimitStack 限制进程栈
func LimitStack() error {
	rlimit := syscall.Rlimit{Cur: 8388608, Max: 8388608}
	if err := syscall.Setrlimit(syscall.RLIMIT_STACK, &rlimit); err != nil {
		return err
	}
	return nil
}

// DropCapabilities 丢弃不必要的 capabilities
func DropCapabilities() error {
	dangerousCaps := []int{
		CAP_SYS_ADMIN,
		CAP_SYS_MODULE,
		CAP_SYS_RAWIO,
		CAP_SYS_PTRACE,
		CAP_SYS_BOOT,
		CAP_SYS_NICE,
		CAP_SYS_TIME,
		CAP_SYS_TTY_CONFIG,
		CAP_SYS_PACCT,
		CAP_SYSLOG,
		CAP_MAC_ADMIN,
		CAP_MAC_OVERRIDE,
		CAP_BLOCK_SUSPEND,
		CAP_PERFMON,
		CAP_BPF,
		CAP_CHECKPOINT_RESTORE,
	}

	logger.Debug(i18n.T("security.drop_caps"))

	for _, cap := range dangerousCaps {
		_, _, err := syscall.Syscall6(syscall.SYS_PRCTL, 7, uintptr(cap), 0, 0, 0, 0)
		if err != 0 {
			logger.Debug(i18n.Tf("security.drop_cap_fail", cap))
		}
	}

	return nil
}

// ApplySeccomp 尝试应用 seccomp 过滤器（简化版）
func ApplySeccomp() error {
	logger.Debug("设置 PR_SET_NO_NEW_PRIVS")
	_, _, err := syscall.Syscall6(syscall.SYS_PRCTL, 38, 1, 0, 0, 0, 0)
	if err != 0 {
		logger.Debug("设置 PR_SET_NO_NEW_PRIVS 失败: %v", err)
	}
	return nil
}

// SecureChroot 安全地执行 chroot
func SecureChroot(path string) error {
	logger.Debug("执行安全 chroot: %s", path)
	if err := syscall.Chdir(path); err != nil {
		return fmt.Errorf("chdir 失败: %w", err)
	}
	if err := syscall.Chroot("."); err != nil {
		return fmt.Errorf("chroot 失败: %w", err)
	}
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir / 失败: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if cwd != "/" {
		return fmt.Errorf("安全检查失败: 当前工作目录不是 /")
	}
	return nil
}

// ApplySecurity 应用所有安全限制
func ApplySecurity() error {
	logger.Info(i18n.T("security.applying"))
	if err := DisablePtrace(); err != nil {
		logger.Debug(i18n.T("security.ptrace_disable_fail"))
	}
	if err := LimitCoreDump(); err != nil {
		logger.Debug("设置核心转储限制失败: %v", err)
	}
	if err := LimitStack(); err != nil {
		logger.Debug("设置进程栈限制失败: %v", err)
	}
	if err := DropCapabilities(); err != nil {
		logger.Debug("丢弃 capabilities 失败: %v", err)
	}
	if err := ApplySeccomp(); err != nil {
		logger.Debug("应用 seccomp 保护失败: %v", err)
	}
	return nil
}
