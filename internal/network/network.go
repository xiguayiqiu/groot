package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"litevm/internal/i18n"
	"litevm/internal/logger"
	"litevm/internal/termux"
)

type NetworkConfig struct {
	Enable    bool
	Interface string
	IPAddr    string
	Subnet    string
	Gateway   string
	DNS       []string
	MTU       int
}

func SetupNetworkInChildNs(childPid int) error {
	logger.Info(i18n.T("network.configuring_ns"))

	if err := setupVethPair(childPid); err != nil {
		return fmt.Errorf("%s", i18n.Tf("network.veth_fail", err))
	}

	if err := configureHostSide(childPid); err != nil {
		return fmt.Errorf("%s", i18n.Tf("network.host_fail", err))
	}

	if err := configureChildSide(childPid); err != nil {
		return fmt.Errorf("%s", i18n.Tf("network.guest_fail", err))
	}

	if err := setupNAT(); err != nil {
		logger.Warn(i18n.Tf("network.nat_fail", err))
	}

	logger.Info(i18n.T("network.config_done"))
	return nil
}

func setupVethPair(childPid int) error {
	// 在主机端创建 veth 对
	cmds := []string{
		"ip link add veth-host type veth peer name veth-guest",
	}

	for _, cmdStr := range cmds {
		logger.Debug(i18n.Tf("network.cmd_exec", cmdStr))
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn(i18n.Tf("network.cmd_fail", cmdStr, string(output)))
			return fmt.Errorf("%s", i18n.Tf("network.cmd_exec_fail", cmdStr, err))
		}
	}

	// 将 veth-guest 移动到子进程的网络命名空间
	cmdStr := fmt.Sprintf("ip link set veth-guest netns %d", childPid)
	logger.Debug(i18n.Tf("network.cmd_exec", cmdStr))
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(i18n.Tf("network.cmd_fail", cmdStr, string(output)))
		return fmt.Errorf("%s", i18n.Tf("network.cmd_exec_fail", cmdStr, err))
	}

	return nil
}

func configureHostSide(childPid int) error {
	cmds := []string{
		"ip addr add 10.0.0.1/24 dev veth-host",
		"ip link set veth-host up",
	}

	for _, cmdStr := range cmds {
		logger.Debug(i18n.Tf("network.cmd_exec", cmdStr))
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn(i18n.Tf("network.cmd_fail", cmdStr, string(output)))
			return fmt.Errorf("%s", i18n.Tf("network.cmd_exec_fail", cmdStr, err))
		}
	}

	return nil
}

func configureChildSide(childPid int) error {
	cmds := []string{
		fmt.Sprintf("nsenter -t %d -n -- ip link set lo up", childPid),
		fmt.Sprintf("nsenter -t %d -n -- ip addr add 10.0.0.2/24 dev veth-guest", childPid),
		fmt.Sprintf("nsenter -t %d -n -- ip link set veth-guest up", childPid),
		fmt.Sprintf("nsenter -t %d -n -- ip route add default via 10.0.0.1", childPid),
	}

	for _, cmdStr := range cmds {
		logger.Debug(i18n.Tf("network.cmd_exec", cmdStr))
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn(i18n.Tf("network.cmd_fail", cmdStr, string(output)))
		}
	}

	return nil
}

func setupNAT() error {
	cmds := []string{
		"echo 1 > /proc/sys/net/ipv4/ip_forward",
		"iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -j MASQUERADE",
		"iptables -A FORWARD -i veth-host -o veth-host -j ACCEPT",
		"iptables -A FORWARD -i veth-host -j ACCEPT",
		"iptables -A FORWARD -o veth-host -j ACCEPT",
	}

	for _, cmdStr := range cmds {
		logger.Debug(i18n.Tf("network.cmd_exec", cmdStr))
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn(i18n.Tf("network.cmd_fail", cmdStr, string(output)))
		}
	}

	return nil
}

func SetupChildDns(rootfsPath string) error {
	resolvPath := filepath.Join(rootfsPath, "etc", "resolv.conf")

	// 确保目录存在
	dir := filepath.Dir(resolvPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("%s", i18n.T("network.create_dir_fail"))
	}

	dnsServers := []string{"10.0.0.1", "8.8.8.8", "8.8.4.4", "114.114.114.114"}

	var content strings.Builder
	for _, dns := range dnsServers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", dns))
	}

	if err := os.WriteFile(resolvPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("%s", i18n.T("network.write_resolv_fail"))
	}

	return nil
}

func CleanupNetworkOnHost() {
	logger.Info(i18n.T("network.cleanup_host"))

	cmds := []string{
		"ip link delete veth-host 2>/dev/null || true",
		"iptables -t nat -D POSTROUTING -s 10.0.0.0/24 -j MASQUERADE 2>/dev/null || true",
		"iptables -D FORWARD -i veth-host -o veth-host -j ACCEPT 2>/dev/null || true",
		"iptables -D FORWARD -i veth-host -j ACCEPT 2>/dev/null || true",
		"iptables -D FORWARD -o veth-host -j ACCEPT 2>/dev/null || true",
	}

	for _, cmdStr := range cmds {
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		cmd.Run()
	}

	logger.Info(i18n.T("network.cleanup_done"))
}

// CleanupContainerResources 清理容器使用的所有网络资源。
// 包括：
//  1. 主机端 veth-host 接口和 iptables 规则
//  2. 尝试清理容器网络命名空间中的 veth-guest（若容器已终止）
//  3. 禁用 IP 转发（如果之前启用过）
func CleanupContainerResources(pid int, rootfs string) {
	logger.Info(i18n.Tf("network.cleanup_container", pid))

	// 1. 清理主机端资源
	CleanupNetworkOnHost()

	// 2. 尝试清理容器网络命名空间中的 veth-guest
	//    如果容器进程已终止，veth-guest 可能仍残留在其网络命名空间中。
	//    尝试通过 nsenter 删除，或者依赖内核在网络命名空间销毁时自动清理。
	if pid > 0 {
		// 检查容器进程是否仍存在
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); os.IsNotExist(err) {
			// 容器已终止，尝试清理其网络命名空间中的残留接口
			// 注意：如果网络命名空间已被销毁，这条命令会失败，但不会影响
			cmdStr := fmt.Sprintf(
				"nsenter -t %d -n -- ip link del veth-guest 2>/dev/null || true", pid,
			)
			exec.Command("/bin/sh", "-c", cmdStr).Run()
		}
	}

	// 3. 禁用 IP 转发（除非系统需要它）
	// 注意：这可能会影响其他容器或系统功能，因此仅在确定没有其他使用时才执行
	// 此处不自动禁用，以免影响其他容器

	logger.Info(i18n.Tf("network.cleanup_container_done", pid))
}

// EnsureNoOrphanedNetwork 检测并清理可能残留的孤立网络资源。
// 用于启动容器前或系统启动时，清理上次异常退 possible 留下的残留。
func EnsureNoOrphanedNetwork() {
	logger.Debug(i18n.T("network.detect_orphan"))

	// 检测 veth-host 是否存在但无对应容器
	// 尝试删除 veth-host（如果存在）
	cmd := exec.Command("/bin/sh", "-c", "ip link delete veth-host 2>/dev/null || true")
	cmd.Run()

	// 检测是否有残留的 iptables 规则
	iptablesCmds := []string{
		"iptables -t nat -D POSTROUTING -s 10.0.0.0/24 -j MASQUERADE 2>/dev/null || true",
		"iptables -D FORWARD -i veth-host -o veth-host -j ACCEPT 2>/dev/null || true",
		"iptables -D FORWARD -i veth-host -j ACCEPT 2>/dev/null || true",
		"iptables -D FORWARD -o veth-host -j ACCEPT 2>/dev/null || true",
	}
	for _, cmdStr := range iptablesCmds {
		exec.Command("/bin/sh", "-c", cmdStr).Run()
	}

	logger.Debug(i18n.T("network.orphan_done"))
}

func GetHostIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "192.168.1.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "192.168.1.1"
}

func CheckNetworkSupport() (bool, string) {
	if _, err := os.Stat("/proc/sys/net/ipv4/ip_forward"); err != nil {
		return false, i18n.T("network.no_ip_forward_file")
	}

	if _, err := termux.SafeLookPath("ip"); err != nil {
		return false, i18n.T("network.no_ip_cmd")
	}

	if _, err := termux.SafeLookPath("nsenter"); err != nil {
		return false, i18n.T("network.no_nsenter_cmd")
	}

	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err == nil {
		if strings.TrimSpace(string(data)) == "0" {
			logger.Warn(i18n.T("network.no_ip_forward"))
			cmd := exec.Command("/bin/sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
			if err := cmd.Run(); err != nil {
				logger.Warn(i18n.Tf("network.ip_forward_fail", err))
			}
		}
	}

	return true, ""
}

func ValidateNetworkConfig(config *NetworkConfig) error {
	if !config.Enable {
		return nil
	}

	if config.Interface == "" {
		config.Interface = DetectAvailableInterface()
	}

	if config.IPAddr == "" {
		config.IPAddr = "10.0.0.2"
	}

	if config.Subnet == "" {
		config.Subnet = "24"
	}

	if config.Gateway == "" {
		config.Gateway = "10.0.0.1"
	}

	if config.MTU == 0 {
		config.MTU = 1500
	}

	return nil
}

func DetectAvailableInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "eth0"
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp != 0 {
			return iface.Name
		}
	}

	return "eth0"
}
