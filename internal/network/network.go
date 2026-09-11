package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"groot/internal/logger"
	"groot/internal/termux"
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
	logger.Info("在子进程网络命名空间中配置网络...")

	if err := setupVethPair(childPid); err != nil {
		return fmt.Errorf("设置 veth 对失败: %w", err)
	}

	if err := configureHostSide(childPid); err != nil {
		return fmt.Errorf("配置主机端网络失败: %w", err)
	}

	if err := configureChildSide(childPid); err != nil {
		return fmt.Errorf("配置容器端网络失败: %w", err)
	}

	if err := setupNAT(); err != nil {
		logger.Warn("设置 NAT 失败: %v", err)
	}

	logger.Info("网络配置完成")
	return nil
}

func setupVethPair(childPid int) error {
	// 在主机端创建 veth 对
	cmds := []string{
		"ip link add veth-host type veth peer name veth-guest",
	}

	for _, cmdStr := range cmds {
		logger.Debug("执行: %s", cmdStr)
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn("命令失败: %s, 输出: %s", cmdStr, string(output))
			return fmt.Errorf("执行 %s 失败: %w", cmdStr, err)
		}
	}

	// 将 veth-guest 移动到子进程的网络命名空间
	cmdStr := fmt.Sprintf("ip link set veth-guest netns %d", childPid)
	logger.Debug("执行: %s", cmdStr)
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("命令失败: %s, 输出: %s", cmdStr, string(output))
		return fmt.Errorf("执行 %s 失败: %w", cmdStr, err)
	}

	return nil
}

func configureHostSide(childPid int) error {
	cmds := []string{
		"ip addr add 10.0.0.1/24 dev veth-host",
		"ip link set veth-host up",
	}

	for _, cmdStr := range cmds {
		logger.Debug("执行: %s", cmdStr)
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn("命令失败: %s, 输出: %s", cmdStr, string(output))
			return fmt.Errorf("执行 %s 失败: %w", cmdStr, err)
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
		logger.Debug("执行: %s", cmdStr)
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn("命令失败: %s, 输出: %s", cmdStr, string(output))
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
		logger.Debug("执行: %s", cmdStr)
		cmd := exec.Command("/bin/sh", "-c", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Warn("命令失败: %s, 输出: %s", cmdStr, string(output))
		}
	}

	return nil
}

func SetupChildDns(rootfsPath string) error {
	resolvPath := filepath.Join(rootfsPath, "etc", "resolv.conf")

	// 确保目录存在
	dir := filepath.Dir(resolvPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	dnsServers := []string{"10.0.0.1", "8.8.8.8", "8.8.4.4", "114.114.114.114"}

	var content strings.Builder
	for _, dns := range dnsServers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", dns))
	}

	if err := os.WriteFile(resolvPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("写入 resolv.conf 失败: %w", err)
	}

	return nil
}

func CleanupNetworkOnHost() {
	logger.Info("清理主机端网络资源...")

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

	logger.Info("主机端网络清理完成")
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
		return false, "未找到 /proc/sys/net/ipv4/ip_forward"
	}

	if _, err := termux.SafeLookPath("ip"); err != nil {
		return false, "未找到 ip 命令"
	}

	if _, err := termux.SafeLookPath("nsenter"); err != nil {
		return false, "未找到 nsenter 命令"
	}

	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err == nil {
		if strings.TrimSpace(string(data)) == "0" {
			logger.Warn("IP 转发未启用，尝试启用...")
			cmd := exec.Command("/bin/sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
			if err := cmd.Run(); err != nil {
				logger.Warn("启用 IP 转发失败: %v", err)
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
