package env

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"groot/internal/i18n"
	"groot/internal/logger"
	"groot/internal/usercheck"
)

// reservedVars 由 groot 程序自身设置的变量，不从 rootfs 配置文件读取
var reservedVars = map[string]bool{
	"HOME":     true,
	"USER":     true,
	"LOGNAME":  true,
	"SHELL":    true,
	"PWD":      true,
	"HOSTNAME": true,
}

// safePreserveVars 从宿主机安全保留的变量
var safePreserveVars = []string{
	"DISPLAY", "WAYLAND_DISPLAY",
	"XDG_RUNTIME_DIR",
	"DBUS_SESSION_BUS_ADDRESS",
	"SSH_AUTH_SOCK", "SSH_AGENT_PID",
}

// detectDistro 检测 rootfs 是什么发行版
func detectDistro(rootfsPath string) string {
	osReleasePath := filepath.Join(rootfsPath, "etc", "os-release")
	if data, err := os.ReadFile(osReleasePath); err == nil {
		content := strings.ToLower(string(data))
		id := ""
		idLike := ""

		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "id=") {
				id = strings.Trim(strings.TrimPrefix(line, "id="), "\"")
			}
			if strings.HasPrefix(line, "id_like=") {
				idLike = strings.Trim(strings.TrimPrefix(line, "id_like="), "\"")
			}
		}

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

	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "alpine-release")); err == nil {
		return "alpine"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "void-release")); err == nil {
		return "void"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "debian_version")); err == nil {
		return "debian"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "redhat-release")); err == nil {
		return "redhat"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "fedora-release")); err == nil {
		return "redhat"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "centos-release")); err == nil {
		return "redhat"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "arch-release")); err == nil {
		return "arch"
	}
	if _, err := os.Stat(filepath.Join(rootfsPath, "etc", "freebsd-update.conf")); err == nil {
		return "unix"
	}

	return "generic"
}

// GetHostname 从 rootfs 获取 hostname
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

// parseKeyValueFile 解析 KEY=VALUE 格式的配置文件（locale.conf, environment 等）
func parseKeyValueFile(filePath string) map[string]string {
	result := make(map[string]string)

	file, err := os.Open(filePath)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"'")
			result[key] = value
		}
	}

	return result
}

// loadProfileEnv 通过运行 rootfs 中的 shell 来正确加载 /etc/profile 及 profile.d/*.sh。
// 这与标准 Linux login shell 的行为一致，能正确处理条件分支、变量展开、函数等 shell 逻辑。
// 找到 shell 后执行: chroot <rootfs> <shell> -c 'env' 并解析输出。
func loadProfileEnv(rootfsPath string) map[string]string {
	result := make(map[string]string)

	shell := findShell(rootfsPath)
	if shell == "" {
		logger.Debug("env: no shell found in rootfs, skipping profile loading")
		return result
	}

	// 用 chroot 在 rootfs 中执行 login shell，让它 source /etc/profile 并输出 env
	// 使用 -l 确保作为 login shell 启动，加载完整配置
	cmd := exec.Command("chroot", rootfsPath, shell, "-l", "-c", "env")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		logger.Debug("env: chroot profile load failed: %v, stderr: %s", err, stderr.String())
		return parseProfileExports(rootfsPath)
	}

	stdoutStr := stdout.String()
	logger.Debug("env: chroot profile load OK, stdout=%d bytes, stderr=%d bytes", len(stdoutStr), stderr.Len())
	if stderr.Len() > 0 {
		logger.Debug("env: chroot stderr: %s", stderr.String())
	}

	// 解析 env 输出的 KEY=VALUE 对
	for _, line := range strings.Split(stdoutStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := line[:idx]
			value := line[idx+1:]
			if key != "" && isEnvVarName(key) && !reservedVars[key] {
				result[key] = value
			}
		}
	}

	if len(result) > 0 {
		logger.Debug("env: chroot captured %d env vars", len(result))
		for _, k := range []string{"PATH", "LANG", "HOME", "TERM"} {
			if v, ok := result[k]; ok {
				logger.Debug("env:   %s=%s", k, v)
			}
		}
	} else {
		logger.Debug("env: chroot captured 0 env vars (empty output?)")
	}

	return result
}

// findShell 在 rootfs 中寻找可用的 shell
func findShell(rootfsPath string) string {
	candidates := []string{"/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
	for _, sh := range candidates {
		if _, err := os.Stat(filepath.Join(rootfsPath, sh)); err == nil {
			return sh
		}
	}
	return ""
}

// parseProfileExports 作为 loadProfileEnv 的回退方案，静态解析 profile 文件。
// 仅在 chroot 不可用时（如权限不足）使用。
func parseProfileExports(rootfsPath string) map[string]string {
	result := make(map[string]string)

	profileFiles := []string{
		filepath.Join(rootfsPath, "etc", "profile"),
	}

	profileDir := filepath.Join(rootfsPath, "etc", "profile.d")
	if entries, err := os.ReadDir(profileDir); err == nil {
		var shFiles []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sh") {
				shFiles = append(shFiles, filepath.Join(profileDir, e.Name()))
			}
		}
		sort.Strings(shFiles)
		profileFiles = append(profileFiles, shFiles...)
	}

	for _, fpath := range profileFiles {
		parseProfileFile(fpath, result)
	}

	return result
}

// parseProfileFile 静态解析单个 shell profile 文件，仅提取简单的 export KEY=VALUE 赋值。
// 跳过包含 shell 变量展开 ($...) 的值和交互式 shell 变量。
func parseProfileFile(filePath string, result map[string]string) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		assignable := line
		if strings.HasPrefix(line, "export ") {
			assignable = strings.TrimSpace(line[7:])
		}

		if idx := strings.Index(assignable, "="); idx > 0 {
			key := strings.TrimSpace(assignable[:idx])
			value := strings.TrimSpace(assignable[idx+1:])
			value = strings.Trim(value, "\"'")

			if key != "" && isEnvVarName(key) && !reservedVars[key] &&
				!strings.Contains(value, "$") {
				result[key] = value
			}
		}
	}
}

// isEnvVarName 检查是否是合法的环境变量名
func isEnvVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if i == 0 {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// SetupEnv 从 rootfs 自动加载环境变量，仅设置 groot 必需的变量
func SetupEnv(userInfo *usercheck.UserInfo, hostname string, rootfsPath ...string) []string {
	absRootfsPath := ""
	if len(rootfsPath) > 0 {
		absRootfsPath = rootfsPath[0]
	}

	// ========================================
	// 第一步：设置 groot 自身必需的变量
	// ========================================
	envMap := map[string]string{
		"HOME":     userInfo.Home,
		"USER":     userInfo.Username,
		"LOGNAME":  userInfo.Username,
		"SHELL":    userInfo.Shell,
		"PWD":      userInfo.Home,
		"HOSTNAME": hostname,
	}

	// ========================================
	// 第二步：从 rootfs 的简单配置文件加载变量
	// 只读取纯 KEY=VALUE 格式的文件
	// ========================================
	if absRootfsPath != "" {
		// 1. /etc/locale.conf - 语言环境配置
		localeConf := parseKeyValueFile(filepath.Join(absRootfsPath, "etc", "locale.conf"))
		for k, v := range localeConf {
			envMap[k] = v
		}
		// 如果 locale.conf 设置了 LANG，让其他 locale 变量跟随
		if lang, ok := localeConf["LANG"]; ok {
			if _, ok := localeConf["LC_ALL"]; !ok {
				envMap["LC_ALL"] = lang
			}
			if _, ok := localeConf["LC_CTYPE"]; !ok {
				envMap["LC_CTYPE"] = lang
			}
			if _, ok := localeConf["LANGUAGE"]; !ok {
				if idx := strings.Index(lang, "."); idx > 0 {
					envMap["LANGUAGE"] = lang[:idx]
				} else {
					envMap["LANGUAGE"] = lang
				}
			}
		}

		// 2. /etc/environment - 系统级环境变量（纯 KEY=VALUE）
		envConf := parseKeyValueFile(filepath.Join(absRootfsPath, "etc", "environment"))
		for k, v := range envConf {
			if !reservedVars[k] {
				envMap[k] = v
			}
		}

		// 3. /etc/profile 和 /etc/profile.d/*.sh - 通过运行 login shell 正确加载
		profileExports := loadProfileEnv(absRootfsPath)
		for k, v := range profileExports {
			if !reservedVars[k] {
				envMap[k] = v
			}
		}
	}

	// ========================================
	// 第三步：如果 rootfs 中缺少关键变量，提供最小默认值
	// ========================================
	if _, ok := envMap["LANG"]; !ok {
		envMap["LANG"] = "C.UTF-8"
	}
	if _, ok := envMap["TERM"]; !ok {
		envMap["TERM"] = "xterm-256color"
	}
	if _, ok := envMap["PATH"]; !ok {
		envMap["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	if _, ok := envMap["TMPDIR"]; !ok {
		envMap["TMPDIR"] = "/tmp"
	}

	// ========================================
	// 第四步：保留宿主机安全变量
	// ========================================
	for _, key := range safePreserveVars {
		if value := os.Getenv(key); value != "" {
			if _, exists := envMap[key]; !exists {
				envMap[key] = value
			}
		}
	}

	// ========================================
	// 第五步：转换为切片返回
	// ========================================
	envSlice := make([]string, 0, len(envMap))
	for key, value := range envMap {
		envSlice = append(envSlice, key+"="+value)
	}

	logger.Debug(i18n.Tf("env.loaded", len(envSlice)))

	return envSlice
}
