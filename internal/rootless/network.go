package rootless

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"litevm/internal/logger"
)

const (
	Slirp4netnsBinary = "slirp4netns"
	Bypass4netnsBinary = "bypass4netns"
)

type Slirp4netnsConfig struct {
	ChildPID     int
	MTU          int
	DisableLoopback bool
	EnableSandbox   bool
	EnableSeccomp   bool
	CIDR            string
	APISocket       string
}

type Slirp4netnsInfo struct {
	IP        string
	Gateway   string
	DNS       []string
	DevName   string
	MTU       int
}

func IsSlirp4netnsAvailable() bool {
	_, err := exec.LookPath(Slirp4netnsBinary)
	return err == nil
}

func IsBypass4netnsAvailable() bool {
	_, err := exec.LookPath(Bypass4netnsBinary)
	return err == nil
}

func DetectSlirp4netnsFeatures() map[string]bool {
	features := map[string]bool{
		"ipv6":            false,
		"cidr":            false,
		"disable-loopback": false,
		"api-socket":      false,
		"sandbox":         false,
		"seccomp":         false,
	}

	binary, err := exec.LookPath(Slirp4netnsBinary)
	if err != nil {
		return features
	}

	cmd := exec.Command(binary, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return features
	}

	help := string(output)
	if strings.Contains(help, "--enable-ipv6") {
		features["ipv6"] = true
	}
	if strings.Contains(help, "--cidr") {
		features["cidr"] = true
	}
	if strings.Contains(help, "--disable-host-loopback") {
		features["disable-loopback"] = true
	}
	if strings.Contains(help, "--api-socket") {
		features["api-socket"] = true
	}
	if strings.Contains(help, "--enable-sandbox") {
		features["sandbox"] = true
	}
	if strings.Contains(help, "--enable-seccomp") {
		features["seccomp"] = true
	}

	return features
}

func SetupSlirp4netns(childPID int, cidr string) (*Slirp4netnsInfo, error) {
	binary, err := exec.LookPath(Slirp4netnsBinary)
	if err != nil {
		return nil, fmt.Errorf("slirp4netns not found: %w", err)
	}

	mtu := 1500
	readyR, readyW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}
	defer readyR.Close()
	defer readyW.Close()

	args := []string{
		"--mtu", strconv.Itoa(mtu),
		"-r", "3",
	}

	if cidr != "" {
		args = append(args, "--cidr", cidr)
	}

	args = append(args, strconv.Itoa(childPID), "tap0")

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = append(cmd.ExtraFiles, readyW)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start slirp4netns: %w", err)
	}

	readyW.Close()

	buf := make([]byte, 16)
	readyR.Read(buf)

	info := &Slirp4netnsInfo{
		IP:      "10.0.2.100",
		Gateway: "10.0.2.2",
		DNS:     []string{"10.0.2.3", "8.8.8.8", "8.8.4.4"},
		DevName: "tap0",
		MTU:     mtu,
	}

	if cidr != "" {
		parts := strings.Split(cidr, "/")
		if len(parts) == 2 {
			info.IP = parts[0]
		}
	}

	logger.Info("slirp4netns started: childPID=%d dev=%s ip=%s gw=%s",
		childPID, info.DevName, info.IP, info.Gateway)

	return info, nil
}

func ConfigureNetworkInChild(info *Slirp4netnsInfo) error {
	if info == nil {
		return fmt.Errorf("slirp4netns info is nil")
	}

	commands := []string{
		fmt.Sprintf("ip link set lo up"),
		fmt.Sprintf("ip addr add %s/24 dev %s", info.IP, info.DevName),
		fmt.Sprintf("ip link set %s up", info.DevName),
		fmt.Sprintf("ip route add default via %s", info.Gateway),
	}

	for _, cmdStr := range commands {
		parts := strings.Fields(cmdStr)
		if len(parts) < 2 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			logger.Debug("network cmd failed: %s: %s", cmdStr, string(output))
		}
	}

	return nil
}

func SetupChildDNS(rootfsPath string, dnsServers []string) error {
	resolvPath := filepath.Join(rootfsPath, "etc", "resolv.conf")

	dir := filepath.Dir(resolvPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create etc dir: %w", err)
	}

	var content strings.Builder
	for _, dns := range dnsServers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", dns))
	}

	if err := os.WriteFile(resolvPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write resolv.conf: %w", err)
	}

	logger.Debug("wrote DNS config: %s", resolvPath)
	return nil
}

func SetupBypass4netns(childPID int) error {
	binary, err := exec.LookPath(Bypass4netnsBinary)
	if err != nil {
		return fmt.Errorf("bypass4netns not found: %w", err)
	}

	cmd := exec.Command(binary, "--add", strconv.Itoa(childPID))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bypass4netns failed: %s: %w", string(output), err)
	}

	logger.Info("bypass4netns enabled for PID %d", childPID)
	return nil
}

func CleanupSlirp4netns() {
	logger.Debug("slirp4netns cleanup: handled by child process exit")
}

func SetupUserModeNetwork(childPID int, rootfsPath string, cidr string) (*Slirp4netnsInfo, error) {
	if !IsSlirp4netnsAvailable() {
		return nil, fmt.Errorf("slirp4netns is not installed")
	}

	info, err := SetupSlirp4netns(childPID, cidr)
	if err != nil {
		return nil, err
	}

	if err := SetupChildDNS(rootfsPath, info.DNS); err != nil {
		logger.Warn("failed to setup DNS: %v", err)
	}

	if IsBypass4netnsAvailable() {
		if err := SetupBypass4netns(childPID); err != nil {
			logger.Debug("bypass4netns not available: %v", err)
		}
	}

	return info, nil
}
