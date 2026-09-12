// Package cleanup 提供清理 rootfs 中宿主机环境痕迹的功能。
//
// 设计原则：保留 rootfs 自己的环境配置，仅清理其中可能携带的宿主机残留信息，
// 让容器最终使用 rootfs 自带的干净环境。
package cleanup

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"

	"groot/internal/i18n"
	"groot/internal/logger"
)

// DefaultHostname 默认容器主机名（仅在 rootfs 中无 hostname 时使用）
const DefaultHostname = "groot"

// DefaultPublicDNS 公共 DNS（仅在 rootfs 的 resolv.conf 没有可用 nameserver 时补充）
var DefaultPublicDNS = []string{
	"8.8.8.8",
	"1.1.1.1",
	"114.114.114.114",
}

// CleanupRootfs 在主机端清理 rootfs 中残留的宿主机环境痕迹。
// 保留 rootfs 自己的环境配置（/etc/hosts、/etc/resolv.conf、/etc/hostname 等），
// 仅清理其中可能泄漏宿主机信息的内容。
func CleanupRootfs(rootfsPath string, hostname string) error {
	if hostname == "" {
		hostname = DefaultHostname
	}
	logger.Info(i18n.Tf("cleanup.cleaning", rootfsPath))

	// 1. 清理 /etc/hosts 中的宿主机条目，保留 rootfs 自己的本地回环条目
	if err := cleanEtcHosts(filepath.Join(rootfsPath, "etc", "hosts")); err != nil {
		logger.Warn(i18n.T("cleanup.hosts_fail"))
	}

	// 2. 处理 /etc/resolv.conf：移除宿主机私有 DNS，保留 rootfs 自己的公共 DNS
	if err := cleanEtcResolvConf(filepath.Join(rootfsPath, "etc", "resolv.conf")); err != nil {
		logger.Warn("%s", i18n.Tf("cleanup.resolv_conf_fail", err))
	}

	// 3. 处理 /etc/hostname：保留 rootfs 自己的，仅在缺失时补充
	if err := ensureHostname(filepath.Join(rootfsPath, "etc", "hostname"), hostname); err != nil {
		logger.Warn("%s", i18n.Tf("cleanup.hostname_fail", err))
	}

	// 4. 清空 /etc/machine-id（避免与宿主机冲突）
	if err := writeIfDir(filepath.Join(rootfsPath, "etc", "machine-id"), ""); err != nil {
		logger.Warn("%s", i18n.Tf("cleanup.machine_id_fail", err))
	}

	// 5. 清理 shell 历史文件
	cleanShellHistory(rootfsPath)

	// 6. 清理 SSH known_hosts
	cleanSSHKnownHosts(rootfsPath)

	// 7. 截断 /var/log 下的日志文件
	truncateLogs(rootfsPath)

	// 8. 清理 /tmp 下残留临时文件
	cleanTmpDir(rootfsPath)

	logger.Info(i18n.T("cleanup.done"))
	return nil
}

// writeIfDir 在父目录存在时写入文件
func writeIfDir(path, content string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// cleanEtcHosts 清理 /etc/hosts：保留标准本地回环条目，删除宿主机条目
func cleanEtcHosts(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeIfDir(path, defaultHostsContent())
		}
		return err
	}

	var kept []string
	changed := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// 保留空行和注释
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		// 判断是否是宿主机条目
		if isHostEntry(trimmed) {
			kept = append(kept, line)
		} else {
			logger.Debug("移除 /etc/hosts 宿主机条目: %s", line)
			changed = true
		}
	}

	if !changed {
		// rootfs 自己的 /etc/hosts 是干净的，无需修改
		return nil
	}

	// 写入清理后的内容
	if len(kept) == 0 {
		return os.WriteFile(path, []byte(defaultHostsContent()), 0644)
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0644)
}

// isHostEntry 判断是否是 rootfs 自己的标准本地回环条目
func isHostEntry(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}
	ip := fields[0]
	// 标准回环地址
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	// IPv6 多播前缀（ff00::/8, fe00::/8 等）配合标准名称
	if strings.HasPrefix(ip, "ff02::") || strings.HasPrefix(ip, "ff00::") ||
		strings.HasPrefix(ip, "fe00::") {
		for _, name := range fields[1:] {
			if !isStandardHostName(name) {
				return false
			}
		}
		return true
	}
	// 其他 IP（如 192.168.x.x）视为宿主机条目
	return false
}

func isStandardHostName(name string) bool {
	switch name {
	case "localhost", "ip6-localhost", "ip6-loopback",
		"ip6-localnet", "ip6-mcastprefix", "ip6-allnodes",
		"ip6-allrouters", "ip6-allhosts", "ip6-router":
		return true
	}
	return false
}

func defaultHostsContent() string {
	return `127.0.0.1	localhost
::1	localhost ip6-localhost ip6-loopback
ff02::1	ip6-allnodes
ff02::2	ip6-allrouters
`
}

// cleanEtcResolvConf 处理 /etc/resolv.conf：删除宿主机私有 DNS，保留公共 DNS
func cleanEtcResolvConf(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeIfDir(path, defaultResolvConfContent())
		}
		return err
	}

	// 如果是符号链接（通常指向 /run/systemd/resolve/stub-resolv.conf 等），
	// 说明 rootfs 没有自带配置，删除并写入默认公共 DNS
	if info.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(path)
		logger.Debug("/etc/resolv.conf 是符号链接 -> %s，删除并使用默认配置", target)
		_ = os.Remove(path)
		return writeIfDir(path, defaultResolvConfContent())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var kept []string
	changed := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "nameserver") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && isPrivateIP(fields[1]) {
				logger.Debug("移除 /etc/resolv.conf 宿主机 DNS: %s", fields[1])
				changed = true
				continue
			}
		}
		kept = append(kept, line)
	}

	// 如果没有任何 nameserver，补充默认公共 DNS
	hasNameserver := false
	for _, line := range kept {
		if strings.HasPrefix(strings.TrimSpace(line), "nameserver") {
			hasNameserver = true
			break
		}
	}
	if !hasNameserver {
		for _, dns := range DefaultPublicDNS {
			kept = append(kept, "nameserver "+dns)
		}
		changed = true
	}

	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0644)
}

// isPrivateIP 判断是否是私有/本地 IP（不应出现在容器 DNS 中）
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified()
}

func defaultResolvConfContent() string {
	lines := []string{"# Generated by groot"}
	for _, dns := range DefaultPublicDNS {
		lines = append(lines, "nameserver "+dns)
	}
	return strings.Join(lines, "\n") + "\n"
}

// ensureHostname 保留 rootfs 自己的 hostname，仅在缺失时补充默认
func ensureHostname(path string, hostname string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeIfDir(path, hostname+"\n")
		}
		return err
	}
	if strings.TrimSpace(string(data)) == "" {
		return os.WriteFile(path, []byte(hostname+"\n"), 0644)
	}
	// rootfs 已有自己的 hostname，保留
	return nil
}

// cleanShellHistory 清理常见 shell 的历史记录文件
func cleanShellHistory(rootfsPath string) {
	homes := []string{
		filepath.Join(rootfsPath, "root"),
		filepath.Join(rootfsPath, "home"),
	}
	histFiles := []string{".bash_history", ".ash_history", ".zsh_history", ".sh_history", ".history"}

	for _, base := range homes {
		if base == filepath.Join(rootfsPath, "home") {
			entries, err := os.ReadDir(base)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				userHome := filepath.Join(base, e.Name())
				for _, hf := range histFiles {
					os.WriteFile(filepath.Join(userHome, hf), []byte{}, 0600)
				}
			}
			continue
		}
		for _, hf := range histFiles {
			os.WriteFile(filepath.Join(base, hf), []byte{}, 0600)
		}
	}
}

// cleanSSHKnownHosts 清理 SSH known_hosts
func cleanSSHKnownHosts(rootfsPath string) {
	sshDirs := []string{
		filepath.Join(rootfsPath, "root", ".ssh"),
	}
	entries, _ := os.ReadDir(filepath.Join(rootfsPath, "home"))
	for _, e := range entries {
		if e.IsDir() {
			sshDirs = append(sshDirs, filepath.Join(rootfsPath, "home", e.Name(), ".ssh"))
		}
	}
	for _, dir := range sshDirs {
		known := filepath.Join(dir, "known_hosts")
		if _, err := os.Stat(known); err == nil {
			os.WriteFile(known, []byte{}, 0600)
		}
	}
}

// truncateLogs 截断 /var/log 下的日志文件（不删除，避免破坏目录结构）
func truncateLogs(rootfsPath string) {
	logDir := filepath.Join(rootfsPath, "var", "log")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(logDir, entry.Name())
		info, err := entry.Info()
		if err != nil || info.IsDir() {
			continue
		}
		_ = os.WriteFile(path, []byte{}, 0600)
	}
}

// cleanTmpDir 清理 /tmp 下的临时文件
func cleanTmpDir(rootfsPath string) {
	tmpDir := filepath.Join(rootfsPath, "tmp")
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(tmpDir, entry.Name())
		_ = os.RemoveAll(path)
	}
}
