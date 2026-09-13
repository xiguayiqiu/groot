package vmm

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"groot/internal/logger"
)

type NetworkConfig struct {
	TapName   string
	HostIP    string
	GuestIP   string
	MaskLen   int
	HostIface string
}

func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		TapName: "groot-tap0",
		HostIP:  "172.16.0.1",
		GuestIP: "172.16.0.2",
		MaskLen: 24,
	}
}

func SetupTapDevice(cfg NetworkConfig) error {
	if err := createTapDevice(cfg.TapName); err != nil {
		return err
	}

	if err := configureTapIP(cfg.TapName, cfg.HostIP, cfg.MaskLen); err != nil {
		CleanupTapDevice(cfg.TapName)
		return err
	}

	if err := enableIPForwarding(); err != nil {
		logger.Warn("IP forwarding failed (may need root): %v", err)
	}

	if cfg.HostIface != "" {
		if err := setupNAT(cfg.TapName, cfg.HostIface); err != nil {
			logger.Warn("NAT setup failed (may need root): %v", err)
		}
	}

	// Start DHCP server on TAP interface
	if err := startDHCP(cfg.TapName, cfg.HostIP, cfg.GuestIP); err != nil {
		logger.Warn("DHCP server failed (may need root): %v", err)
	}

	return nil
}

func CleanupTapDevice(tapName string) error {
	// Kill dnsmasq if running
	if pid, err := os.ReadFile("/tmp/groot-dnsmasq.pid"); err == nil {
		cmd := exec.Command("kill", strings.TrimSpace(string(pid)))
		cmd.Run()
		os.Remove("/tmp/groot-dnsmasq.pid")
	}
	os.Remove("/tmp/groot-dnsmasq.leases")

	// Remove NAT rules
	removeNAT(tapName)

	cmd := exec.Command("ip", "link", "del", tapName)
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "cannot find device") {
			// Try with sudo
			cmd = exec.Command("sudo", "-n", "ip", "link", "del", tapName)
			if output, err := cmd.CombinedOutput(); err != nil {
				if !strings.Contains(string(output), "cannot find device") {
					return fmt.Errorf("failed to delete tap device %s: %w", tapName, err)
				}
			}
		}
	}
	logger.Debug("Cleaned up tap device: %s", tapName)
	return nil
}

func createTapDevice(tapName string) error {
	// Try without sudo first
	cmd := exec.Command("ip", "tuntap", "add", "dev", tapName, "mode", "tap")
	if output, err := cmd.CombinedOutput(); err != nil {
		// If permission denied, try with sudo
		if strings.Contains(string(output), "Operation not permitted") {
			logger.Debug("Need root for TAP device, trying with sudo...")
			cmd = exec.Command("sudo", "-n", "ip", "tuntap", "add", "dev", tapName, "mode", "tap")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to create tap device (need root): %s: %w", string(output), err)
			}
		} else {
			return fmt.Errorf("failed to create tap device: %s: %w", string(output), err)
		}
	}

	// Bring up the tap device
	cmd = exec.Command("ip", "link", "set", tapName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			cmd = exec.Command("sudo", "-n", "ip", "link", "set", tapName, "up")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to bring up tap device: %s: %w", string(output), err)
			}
		} else {
			return fmt.Errorf("failed to bring up tap device: %s: %w", string(output), err)
		}
	}

	logger.Debug("Created tap device: %s", tapName)
	return nil
}

func configureTapIP(tapName, ip string, maskLen int) error {
	cidr := fmt.Sprintf("%s/%d", ip, maskLen)
	cmd := exec.Command("ip", "addr", "add", cidr, "dev", tapName)
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			cmd = exec.Command("sudo", "-n", "ip", "addr", "add", cidr, "dev", tapName)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to configure tap IP: %s: %w", string(output), err)
			}
		} else {
			return fmt.Errorf("failed to configure tap IP: %s: %w", string(output), err)
		}
	}

	logger.Debug("Configured tap device %s with IP %s", tapName, cidr)
	return nil
}

func enableIPForwarding() error {
	// Try directly first (already root)
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if _, err := cmd.CombinedOutput(); err != nil {
		// Try with sudo
		cmd = exec.Command("sudo", "-n", "sysctl", "-w", "net.ipv4.ip_forward=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to enable IP forwarding: %s: %w", string(output), err)
		}
	}
	return nil
}

func runIptables(args []string) error {
	cmd := exec.Command("iptables", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			sudoArgs := append([]string{"-n", "iptables"}, args...)
			cmd = exec.Command("sudo", sudoArgs...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("iptables: %s: %w", strings.TrimSpace(string(output)), err)
			}
		} else {
			return fmt.Errorf("iptables: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}
	return nil
}

func startDHCP(tapName, hostIP, guestIP string) error {
	dhcpRange := "172.16.0.10,172.16.0.100,255.255.255.0,12h"

	args := []string{
		"-i", tapName,
		"-F", dhcpRange,
		"--bind-interfaces",
		"-I", "lo",
		"--pid-file=/tmp/groot-dnsmasq.pid",
		"--dhcp-leasefile=/tmp/groot-dnsmasq.leases",
		"-q",
	}

	// Try dnsmasq without sudo first
	cmd := exec.Command("dnsmasq", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			sudoArgs := append([]string{"-n", "dnsmasq"}, args...)
			cmd = exec.Command("sudo", sudoArgs...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to start DHCP server: %s: %w", string(output), err)
			}
		} else {
			return fmt.Errorf("failed to start DHCP server: %s: %w", string(output), err)
		}
	}

	logger.Debug("DHCP server started on %s", tapName)
	return nil
}

func setupNAT(tapName, hostIface string) error {
	// Flush first to avoid duplicates
	runIptables(strings.Fields("-t nat -F POSTROUTING"))
	runIptables(strings.Fields("-F FORWARD"))

	rules := []string{
		fmt.Sprintf("-t nat -A POSTROUTING -o %s -j MASQUERADE", hostIface),
		fmt.Sprintf("-A FORWARD -i %s -o %s -j ACCEPT", tapName, hostIface),
		fmt.Sprintf("-A FORWARD -i %s -o %s -m state --state RELATED,ESTABLISHED -j ACCEPT", hostIface, tapName),
	}

	for _, rule := range rules {
		args := strings.Fields(rule)
		if err := runIptables(args); err != nil {
			logger.Debug("iptables rule warning: %v", err)
		}
	}

	logger.Debug("NAT configured for %s -> %s", tapName, hostIface)
	return nil
}

func removeNAT(tapName string) {
	runIptables(strings.Fields("-t nat -D POSTROUTING -o groot-tap0 -j MASQUERADE"))
	runIptables(strings.Fields("-D FORWARD -i groot-tap0 -o eth0 -j ACCEPT"))
	runIptables(strings.Fields("-D FORWARD -i eth0 -o groot-tap0 -m state --state RELATED,ESTABLISHED -j ACCEPT"))
}

func FindHostInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	exclude := map[string]bool{
		"lo":         true,
		"groot-tap0": true,
	}

	for _, iface := range ifaces {
		if exclude[iface.Name] {
			continue
		}
		if strings.HasPrefix(iface.Name, "groot-tap") {
			continue
		}
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
					return iface.Name
				}
			}
		}
	}
	return ""
}
