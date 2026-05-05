package env

import (
	"os"
	"path/filepath"
	"strings"

	"groot/internal/usercheck"
)

// detectDistro 检测 rootfs 是什么发行版
func detectDistro(rootfsPath string) string {
	// --- 先检查 Alpine
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "alpine-release")); err == nil {
		return "alpine"
	}
	// --- 检查 Debian 系列
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "debian_version")); err == nil {
		return "debian"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "kali_version")); err == nil {
		return "debian"
	}
	// --- 检查 Ubuntu
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "ubuntu_version")); err == nil {
		return "debian"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "lsb-release")); err == nil {
		return "debian"
	}
	// --- 检查 RedHat/Fedora 系列
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
		"PATH":     "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME":     userInfo.Home,
		"USER":     userInfo.Username,
		"LOGNAME":  userInfo.Username,
		"SHELL":    userInfo.Shell,
		"PWD":      "/",
		"HOSTNAME": hostname,
		"TERM":     "xterm-256color",
		"LANG":     "C.UTF-8",
		"LC_ALL":   "C.UTF-8",
		"EDITOR":   "vi",
		"VISUAL":   "vi",
		"PAGER":    "less",
		"LESS":     "-R",
		"TMPDIR":   "/tmp",
		"TEMP":     "/tmp",
		"TMP":      "/tmp",
	}

	// 只根据发行版添加特定的变量！
	if distro == "debian" {
		envMap["DEBIAN_FRONTEND"] = "noninteractive"
		envMap["APT_LISTCHANGES_FRONTEND"] = "none"
		envMap["DPKG_ADMINDIR"] = "/var/lib/dpkg"
		envMap["DPKG_FRONTEND"] = "noninteractive"
	}
	if distro == "redhat" {
		envMap["RPM_BUILD_ROOT"] = ""
		envMap["RPM_OPTS"] = "--quiet"
	}
	if distro == "arch" {
		envMap["PACMAN_HOOKS"] = ""
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
