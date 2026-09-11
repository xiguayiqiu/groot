package env

import (
	"os"
	"path/filepath"
	"strings"

	"groot/internal/usercheck"
)

// detectDistro 检测 rootfs 是什么发行版
func detectDistro(rootfsPath string) string {
	// 首先尝试读取 /etc/os-release（现代Linux发行版标准）
	osReleasePath := filepath.Join(rootfsPath, "etc", "os-release")
	if data, err := os.ReadFile(osReleasePath); err == nil {
		content := strings.ToLower(string(data))
		id := ""
		idLike := ""

		// 解析 ID 和 ID_LIKE
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "id=") {
				id = strings.Trim(strings.TrimPrefix(line, "id="), "\"")
			}
			if strings.HasPrefix(line, "id_like=") {
				idLike = strings.Trim(strings.TrimPrefix(line, "id_like="), "\"")
			}
		}

		// 根据 ID 检测
		switch id {
		case "alpine":
			return "alpine"
		case "arch", "manjaro", "endeavouros", "garuda", "arcolinux", "artix":
			return "arch"
		case "debian", "ubuntu", "linuxmint", "pop", "elementary", "zorin", "kali", "raspbian", "mx", "antix", "devuan":
			return "debian"
		case "fedora", "centos", "rhel", "rocky", "almalinux", "ol", "amzn", "scientific":
			return "redhat"
		case "void":
			return "void"
		case "freebsd", "openbsd", "netbsd", "dragonflybsd", "midnightbsd":
			return "unix"
		}

		// 根据 ID_LIKE 检测衍生发行版
		if strings.Contains(idLike, "alpine") {
			return "alpine"
		}
		if strings.Contains(idLike, "arch") {
			return "arch"
		}
		if strings.Contains(idLike, "debian") || strings.Contains(idLike, "ubuntu") {
			return "debian"
		}
		if strings.Contains(idLike, "fedora") || strings.Contains(idLike, "rhel") || strings.Contains(idLike, "centos") {
			return "redhat"
		}
		if strings.Contains(idLike, "void") {
			return "void"
		}
	}

	// 回退到传统检测方法
	// --- 检查 Alpine
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "alpine-release")); err == nil {
		return "alpine"
	}

	// --- 检查 Void Linux
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "void-release")); err == nil {
		return "void"
	}

	// --- 检查 Debian 系列
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "debian_version")); err == nil {
		return "debian"
	}

	// --- 检查 RedHat/Fedora/CentOS
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "redhat-release")); err == nil {
		return "redhat"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "fedora-release")); err == nil {
		return "redhat"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "centos-release")); err == nil {
		return "redhat"
	}

	// --- 检查 Arch
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "arch-release")); err == nil {
		return "arch"
	}

	// --- 检查 Unix 系统
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "freebsd-update.conf")); err == nil {
		return "unix"
	}

	return "generic"
}

// GetHostname 从 rootfs 获取 hostname，如果没有则返回默认值
func GetHostname(rootfsPath string, defaultHostname string) string {
	hostnamePath := filepath.Join(rootfsPath, "etc", "hostname")
	data, err := os.ReadFile(hostnamePath)
	if err != nil {
		return defaultHostname
	}
	hostname := strings.TrimSpace(string(data))
	if hostname == "" {
		return defaultHostname
	}
	return hostname
}

// SetupEnv 设置环境变量，根据发行版只给相应的！
func SetupEnv(userInfo *usercheck.UserInfo, hostname string, rootfsPath ...string) []string {
	// 先检测发行版
	distro := "generic"
	if len(rootfsPath) > 0 {
		distro = detectDistro(rootfsPath[0])
	}

	// 完全从零开始设置环境变量，不继承任何宿主机的变量
	envMap := map[string]string{
		// 基本用户环境
		"PATH":     "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME":     userInfo.Home,
		"USER":     userInfo.Username,
		"LOGNAME":  userInfo.Username,
		"SHELL":    userInfo.Shell,
		"PWD":      userInfo.Home,
		"HOSTNAME": hostname,

		// 终端和本地化设置
		"TERM":     "xterm-256color",
		"LANG":     "C.UTF-8",
		"LC_ALL":   "C.UTF-8",
		"LC_CTYPE": "UTF-8",

		// 编辑器和分页器
		"EDITOR":   "vi",
		"VISUAL":   "vi",
		"PAGER":    "less",
		"LESS":     "-R",

		// 临时目录
		"TMPDIR":   "/tmp",
		"TEMP":     "/tmp",
		"TMP":      "/tmp",

		// 邮件和新闻
		"MAIL":     "/var/mail/" + userInfo.Username,
		"NEWSBASE": "/var/lib/news",

		// 时区（使用UTC作为默认值）
		"TZ":       "UTC",

		// XDG 基础目录规范
		"XDG_CONFIG_HOME": userInfo.Home + "/.config",
		"XDG_CACHE_HOME":  userInfo.Home + "/.cache",
		"XDG_DATA_HOME":   userInfo.Home + "/.local/share",
		"XDG_RUNTIME_DIR": "/run/user/0",

		// 历史记录
		"HISTFILE": userInfo.Home + "/.sh_history",
		"HISTSIZE": "1000",
		"HISTFILESIZE": "2000",
	}

	// 只根据发行版添加特定的变量！
	// === Alpine Linux ===
	if distro == "alpine" {
		// Alpine 使用 busybox 和 musl libc
		envMap["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		envMap["APK_CACHE"] = "/var/cache/apk"
		envMap["APK_HOME"] = "/etc/apk"
		envMap["OPENRC"] = "1"
		envMap["BASH"] = "/bin/sh"
		envMap["SHELL"] = "/bin/sh"
		// Alpine 特定路径
		envMap["MANPATH"] = "/usr/share/man:/usr/local/share/man"
		envMap["INFODIR"] = "/usr/share/info:/usr/local/share/info"
		// musl libc 特定
		envMap["MUSL_LOCPATH"] = "/usr/share/i18n/locales/musl"
		// Busybox 特定
		envMap["BUSYBOX"] = "/bin/busybox"
	}

	// === Arch Linux ===
	if distro == "arch" {
		envMap["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		envMap["PACMAN_HOOKS"] = ""
		envMap["PACMAN"] = "/usr/bin/pacman"
		envMap["MAKEFLAGS"] = "-j$(nproc)"
		envMap["PKGEXT"] = ".pkg.tar.zst"
		envMap["SRCEXT"] = ".src.tar.gz"
		// Arch 特定路径
		envMap["MANPATH"] = "/usr/local/share/man:/usr/share/man"
		envMap["INFODIR"] = "/usr/share/info"
		// systemd 相关（Arch 使用 systemd）
		envMap["SYSTEMD_IGNORE_ENVIRONMENT"] = "1"
	}

	// === Debian/Ubuntu ===
	if distro == "debian" {
		envMap["DEBIAN_FRONTEND"] = "noninteractive"
		envMap["DEBIAN_PRIORITY"] = "critical"
		envMap["APT_LISTCHANGES_FRONTEND"] = "none"
		envMap["DPKG_ADMINDIR"] = "/var/lib/dpkg"
		envMap["DPKG_FRONTEND"] = "noninteractive"
		envMap["DPKG_COLORS"] = "never"
		envMap["APT_KEY_DONT_WARN_ON_DANGEROUS_USAGE"] = "DontWarn"
		// Debian 特定路径
		envMap["MANPATH"] = "/usr/local/share/man:/usr/share/man"
		envMap["INFODIR"] = "/usr/share/info"
		// locale
		envMap["LANG"] = "C.UTF-8"
		envMap["LANGUAGE"] = "C:en"
	}

	// === RHEL/Fedora/CentOS ===
	if distro == "redhat" {
		envMap["RPM_BUILD_ROOT"] = ""
		envMap["RPM_OPTS"] = "--quiet"
		envMap["DNF"] = "/usr/bin/dnf"
		envMap["YUM"] = "/usr/bin/yum"
		// RHEL 特定路径
		envMap["MANPATH"] = "/usr/local/share/man:/usr/share/man"
		envMap["INFODIR"] = "/usr/share/info"
		// systemd 相关
		envMap["SYSTEMD_IGNORE_ENVIRONMENT"] = "1"
		// SELinux 相关
		envMap["SELINUX"] = "permissive"
	}

	// === Void Linux ===
	if distro == "void" {
		envMap["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		envMap["XBPS_ARCH"] = "x86_64" // 默认架构，运行时检测
		envMap["XBPS_DISTDIR"] = "/var/cache/xbps"
		envMap["XBPS_REPOSITORY"] = "https://repo-default.voidlinux.org/current"
		envMap["XBPS_ALLOW_RESTRICTED"] = "yes"
		envMap["XBPS_MAKEJOBS"] = "$(nproc)"
		envMap["XBPS_SRCDISTDIR"] = "/host/srcpkgs"
		envMap["XBPS_CROSSP"] = ""
		// Void 使用 runit
		envMap["RUNIT"] = "1"
		// musl libc 支持
		envMap["MUSL_LOCPATH"] = "/usr/share/i18n/locales/musl"
		// Void 特定路径
		envMap["MANPATH"] = "/usr/share/man"
		envMap["INFODIR"] = "/usr/share/info"
	}

	// === Unix (FreeBSD, OpenBSD, NetBSD等) ===
	if distro == "unix" {
		envMap["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		envMap["MANPATH"] = "/usr/local/share/man:/usr/share/man"
		envMap["INFODIR"] = "/usr/local/share/info:/usr/share/info"
		// BSD 特定
		envMap["PKG_PATH"] = "/usr/pkg/etc/pkg.conf"
		envMap["LOCALBASE"] = "/usr/local"
		envMap["PREFIX"] = "/usr/local"
		// Unix 通常使用 /usr/local 作为本地安装路径
		envMap["C_INCLUDE_PATH"] = "/usr/local/include"
		envMap["CPLUS_INCLUDE_PATH"] = "/usr/local/include"
		envMap["LIBRARY_PATH"] = "/usr/local/lib"
		envMap["LD_LIBRARY_PATH"] = "/usr/local/lib"
	}

	// === Generic/其他 ===
	if distro == "generic" {
		envMap["MANPATH"] = "/usr/local/share/man:/usr/share/man"
		envMap["INFODIR"] = "/usr/local/share/info:/usr/share/info"
	}

	// 只选择性地保留最核心的功能变量，避免任何可能干扰的变量
	safePreserveVars := []string{
		"DISPLAY", "WAYLAND_DISPLAY",
		"XDG_RUNTIME_DIR",
		"DBUS_SESSION_BUS_ADDRESS",
		"SSH_AUTH_SOCK", "SSH_AGENT_PID",
	}

	for _, key := range safePreserveVars {
		if value := os.Getenv(key); value != "" {
			envMap[key] = value
		}
	}

	// 转换为切片
	envSlice := make([]string, 0, len(envMap))
	for key, value := range envMap {
		envSlice = append(envSlice, key+"="+value)
	}

	return envSlice
}
