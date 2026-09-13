package rootless

import (
	"fmt"
	"syscall"

	seccomp "github.com/seccomp/libseccomp-golang"
	"groot/internal/logger"
)

type SeccompConfig struct {
	DefaultAction seccomp.ScmpAction
	Architectures []seccomp.ScmpArch
	AllowSyscalls []string
	DenySyscalls  []string
	_errno        int16
}

type SeccompFilter struct {
	config *SeccompConfig
	filter *seccomp.ScmpFilter
}

func DefaultSeccompConfig() *SeccompConfig {
	return &SeccompConfig{
		DefaultAction: seccomp.ActErrno,
		Architectures: []seccomp.ScmpArch{seccomp.ArchAMD64},
		AllowSyscalls: defaultAllowedSyscalls(),
	}
}

func ContainerSeccompConfig() *SeccompConfig {
	return &SeccompConfig{
		DefaultAction: seccomp.ActErrno,
		Architectures: []seccomp.ScmpArch{seccomp.ArchAMD64},
		AllowSyscalls: containerAllowedSyscalls(),
		DenySyscalls:  containerDeniedSyscalls(),
	}
}

func NewSeccompFilter(cfg *SeccompConfig) (*SeccompFilter, error) {
	if cfg == nil {
		cfg = DefaultSeccompConfig()
	}

	filter, err := seccomp.NewFilter(cfg.DefaultAction)
	if err != nil {
		return nil, fmt.Errorf("failed to create seccomp filter: %w", err)
	}

	for _, arch := range cfg.Architectures {
		if err := filter.AddArch(arch); err != nil {
			filter.Release()
			return nil, fmt.Errorf("failed to add arch %v: %w", arch, err)
		}
	}

	if err := filter.SetNoNewPrivsBit(true); err != nil {
		filter.Release()
		return nil, fmt.Errorf("failed to set no_new_privs: %w", err)
	}

	for _, name := range cfg.AllowSyscalls {
		syscallNum, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			logger.Debug("seccomp: syscall %s not found, skipping", name)
			continue
		}
		if err := filter.AddRule(syscallNum, seccomp.ActAllow); err != nil {
			logger.Debug("seccomp: failed to add rule for %s: %v", name, err)
		}
	}

	for _, name := range cfg.DenySyscalls {
		syscallNum, err := seccomp.GetSyscallFromName(name)
		if err != nil {
			logger.Debug("seccomp: syscall %s not found, skipping", name)
			continue
		}
		if err := filter.AddRule(syscallNum, seccomp.ActErrno.SetReturnCode(int16(syscall.EPERM))); err != nil {
			logger.Debug("seccomp: failed to add deny rule for %s: %v", name, err)
		}
	}

	return &SeccompFilter{
		config: cfg,
		filter: filter,
	}, nil
}

func (sf *SeccompFilter) Load() error {
	if sf.filter == nil {
		return fmt.Errorf("seccomp filter not initialized")
	}

	if err := sf.filter.Load(); err != nil {
		return fmt.Errorf("failed to load seccomp filter: %w", err)
	}

	logger.Info("seccomp filter loaded")
	return nil
}

func (sf *SeccompFilter) Release() {
	if sf.filter != nil {
		sf.filter.Release()
		sf.filter = nil
	}
}

func (sf *SeccompFilter) GetFilter() *seccomp.ScmpFilter {
	return sf.filter
}

func ApplySeccompDefault() error {
	filter, err := NewSeccompFilter(DefaultSeccompConfig())
	if err != nil {
		return err
	}
	defer filter.Release()

	return filter.Load()
}

func ApplyContainerSeccomp() error {
	filter, err := NewSeccompFilter(ContainerSeccompConfig())
	if err != nil {
		return err
	}
	defer filter.Release()

	return filter.Load()
}

func defaultAllowedSyscalls() []string {
	return []string{
		"read", "write", "open", "close", "stat", "fstat", "lstat",
		"poll", "lseek", "mmap", "mprotect", "munmap", "brk",
		"rt_sigaction", "rt_sigprocmask", "ioctl", "access",
		"pipe", "select", "sched_yield", "mremap", "msync",
		"mincore", "madvise", "shmget", "shmat", "shmctl",
		"dup", "dup2", "pause", "nanosleep", "getitimer",
		"alarm", "setitimer", "getpid", "sendfile", "socket",
		"connect", "accept", "sendto", "recvfrom", "sendmsg",
		"recvmsg", "shutdown", "bind", "listen", "getsockname",
		"getpeername", "socketpair", "setsockopt", "getsockopt",
		"clone", "fork", "vfork", "execve", "exit",
		"wait4", "kill", "uname", "semget", "semop",
		"semctl", "shmdt", "msgget", "msgsnd", "msgrcv",
		"msgctl", "fcntl", "flock", "fsync", "fdatasync",
		"truncate", "ftruncate", "getdents", "getcwd",
		"chdir", "fchdir", "rename", "mkdir", "rmdir",
		"creat", "link", "unlink", "symlink", "readlink",
		"chmod", "fchmod", "chown", "fchown", "lchown",
		"umask", "gettimeofday", "getuid", "getgid",
		"geteuid", "getegid", "setpgid", "getppid", "getpgrp",
		"setsid", "setreuid", "setregid",
		"getgroups", "setgroups", "setresuid", "getresuid",
		"setresgid", "getresgid", "getpgid", "setfsuid",
		"setfsgid", "getsid", "capget", "capset",
		"rt_sigpending", "rt_sigtimedwait", "rt_sigqueueinfo",
		"rt_sigsuspend", "sigaltstack",
		"statfs", "fstatfs", "sysfs",
		"getpriority", "setpriority",
		"sched_setparam", "sched_getparam",
		"sched_setscheduler", "sched_getscheduler",
		"sched_get_priority_max", "sched_get_priority_min",
		"sched_rr_get_interval",
		"mlock", "munlock", "mlockall", "munlockall",
		"vhangup", "modify_ldt", "pivot_root",
		"_sysctl", "prctl", "arch_prctl",
		"adjtimex", "setrlimit", "chroot", "sync",
		"acct", "settimeofday", "mount",
		"umount2", "swapon", "swapoff",
		"reboot", "sethostname", "setdomainname",
		"iopl", "ioperm", "create_module", "init_module",
		"delete_module", "get_kernel_syms", "query_module",
		"quotactl", "nfsservctl",
		"readahead", "setxattr", "lsetxattr", "fsetxattr",
		"getxattr", "lgetxattr", "fgetxattr",
		"listxattr", "llistxattr", "flistxattr",
		"removexattr", "lremovexattr", "fremovexattr",
		"tkill", "time", "futex",
		"sched_setaffinity", "sched_getaffinity",
		"set_thread_area", "io_setup", "io_destroy",
		"io_getevents", "io_submit", "io_cancel",
		"get_thread_area", "lookup_dcookie",
		"epoll_create", "epoll_ctl_old", "epoll_wait_old",
		"remap_file_pages", "getdents64",
		"set_tid_address", "restart_syscall",
		"semtimedop", "fadvise64",
		"timer_create", "timer_settime", "timer_gettime",
		"timer_getoverrun", "timer_delete", "clock_settime",
		"clock_gettime", "clock_getres", "clock_nanosleep",
		"exit_group", "epoll_wait",
		"epoll_ctl", "tgkill",
		"utimes", "mbind", "set_mempolicy",
		"get_mempolicy", "mq_open", "mq_unlink",
		"mq_timedsend", "mq_timedreceive", "mq_notify",
		"mq_getsetattr", "kexec_load", "waitid",
		"add_key", "request_key", "keyctl",
		"ioprio_set", "ioprio_get",
		"inotify_init", "inotify_add_watch", "inotify_rm_watch",
		"migrate_pages", "openat", "mkdirat", "mknodat",
		"fchownat", "futimesat", "newfstatat", "unlinkat",
		"renameat", "linkat", "symlinkat", "readlinkat",
		"fchmodat", "faccessat", "pselect6", "ppoll",
		"unshare", "set_robust_list", "get_robust_list",
		"splice", "tee", "sync_file_range",
		"vmsplice", "move_pages",
		"utimensat", "epoll_pwait",
		"signalfd", "timerfd_create", "eventfd",
		"fallocate", "timerfd_settime", "timerfd_gettime",
		"accept4", "signalfd4", "eventfd2",
		"epoll_create1", "dup3", "pipe2",
		"inotify_init1", "preadv", "pwritev",
		"rt_tgsigqueueinfo", "perf_event_open",
		"recvmmsg", "fanotify_init", "fanotify_mark",
		"prlimit64", "name_to_handle_at", "open_by_handle_at",
		"clock_adjtime", "syncfs", "sendmmsg",
		"setns", "getcpu", "process_vm_readv",
		"process_vm_writev", "kcmp",
		"finit_module", "sched_setattr", "sched_getattr",
		"renameat2", "seccomp", "getrandom",
		"memfd_create", "kexec_file_load",
		"bpf", "execveat",
		"userfaultfd", "membarrier",
		"mlock2", "copy_file_range",
		"preadv2", "pwritev2",
		"pkey_mprotect", "pkey_alloc", "pkey_free",
		"statx", "io_pgetevents", "rseq",
		"pidfd_send_signal", "io_uring_setup",
		"io_uring_enter", "io_uring_register",
		"open_tree", "move_mount",
		"fsopen", "fsconfig", "fsmount",
		"fspick", "pidfd_open",
		"clone3", "close_range",
		"openat2", "pidfd_getfd",
		"faccessat2", "process_madvise",
		"epoll_pwait2", "mount_setattr",
		"quotactl_fd", "landlock_create_ruleset",
		"landlock_add_rule", "landlock_restrict_self",
		"memfd_secret", "process_mrelease",
		"futex_waitv", "set_mempolicy_home_node",
	}
}

func containerAllowedSyscalls() []string {
	return []string{
		"read", "write", "open", "close", "stat", "fstat", "lstat",
		"poll", "lseek", "mmap", "mprotect", "munmap", "brk",
		"rt_sigaction", "rt_sigprocmask", "rt_sigreturn",
		"ioctl", "access", "pipe", "select", "sched_yield",
		"mremap", "msync", "mincore", "madvise",
		"dup", "dup2", "pause", "nanosleep",
		"getpid", "sendfile", "socket", "connect", "accept",
		"sendto", "recvfrom", "sendmsg", "recvmsg",
		"shutdown", "bind", "listen", "getsockname",
		"getpeername", "socketpair", "setsockopt", "getsockopt",
		"clone", "fork", "vfork", "execve", "exit",
		"wait4", "kill", "uname",
		"fcntl", "flock", "fsync", "fdatasync",
		"truncate", "ftruncate", "getdents", "getcwd",
		"chdir", "fchdir", "rename", "mkdir", "rmdir",
		"creat", "link", "unlink", "symlink", "readlink",
		"chmod", "fchmod", "chown", "fchown", "lchown",
		"umask", "gettimeofday", "getuid", "getgid",
		"geteuid", "getegid", "setpgid", "getppid", "getpgrp",
		"setsid", "getgroups", "setgroups",
		"setresuid", "getresuid", "setresgid", "getresgid",
		"getpgid", "setfsuid", "setfsgid", "getsid",
		"capget", "capset", "rt_sigpending", "rt_sigtimedwait",
		"rt_sigqueueinfo", "rt_sigsuspend", "sigaltstack",
		"statfs", "fstatfs", "sysfs",
		"getpriority", "setpriority",
		"sched_setparam", "sched_getparam",
		"sched_setscheduler", "sched_getscheduler",
		"sched_get_priority_max", "sched_get_priority_min",
		"sched_rr_get_interval",
		"mlock", "munlock", "mlockall", "munlockall",
		"vhangup", "pivot_root",
		"_sysctl", "prctl", "arch_prctl",
		"setrlimit", "sync",
		"mount", "umount2",
		"sethostname", "setdomainname",
		"getrlimit", "getrusage",
		"prlimit64", "getrandom",
		"memfd_create", "getrandom",
		"rseq", "close_range",
		"openat", "mkdirat", "mknodat",
		"fchownat", "futimesat", "newfstatat", "unlinkat",
		"renameat", "linkat", "symlinkat", "readlinkat",
		"fchmodat", "faccessat", "pselect6", "ppoll",
		"unshare", "set_robust_list", "get_robust_list",
		"splice", "tee",
		"utimensat", "epoll_pwait",
		"signalfd", "timerfd_create", "eventfd",
		"fallocate", "timerfd_settime", "timerfd_gettime",
		"accept4", "signalfd4", "eventfd2",
		"epoll_create1", "dup3", "pipe2",
		"inotify_init1", "preadv", "pwritev",
		"rt_tgsigqueueinfo", "perf_event_open",
		"fanotify_init", "fanotify_mark",
		"clock_adjtime", "syncfs", "sendmmsg",
		"setns", "getcpu", "process_vm_readv",
		"process_vm_writev", "pidfd_send_signal",
		"io_uring_setup", "io_uring_enter", "io_uring_register",
		"pidfd_open", "clone3",
		"openat2", "pidfd_getfd",
		"faccessat2",
		"epoll_pwait2",
	}
}

func containerDeniedSyscalls() []string {
	return []string{
		"mount", "umount2", "pivot_root",
		"chroot", "reboot",
		"swapon", "swapoff",
		"sethostname", "setdomainname",
		"iopl", "ioperm",
		"delete_module", "init_module",
		"quotactl", "nfsservctl",
		"get_kernel_syms", "query_module",
		"settimeofday", "adjtimex",
		"setrlimit",
		"acct",
		"settimeofday",
		"mount_setattr",
		"open_tree", "move_mount",
		"fsopen", "fsconfig", "fsmount", "fspick",
		"pidfd_open",
	}
}
