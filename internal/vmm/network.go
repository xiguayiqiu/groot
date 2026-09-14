package vmm

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"litevm/internal/i18n"
	"litevm/internal/logger"
)

// DistroFamily represents the Linux distribution family
type DistroFamily int

const (
	DistroUnknown DistroFamily = iota
	DistroArch                   // Arch, Manjaro, EndeavourOS
	DistroDebian                 // Debian, Ubuntu, Mint, Kali
	DistroRedHat                 // RHEL, CentOS, Fedora, Rocky, Alma
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
		TapName: "litevm-tap0",
		HostIP:  "172.16.0.1",
		GuestIP: "172.16.0.2",
		MaskLen: 24,
	}
}

// detectDistroFamily detects the Linux distribution family
func detectDistroFamily() DistroFamily {
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return DistroArch
	}
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return DistroDebian
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return DistroRedHat
	}
	// Try /etc/os-release for more clues
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(data)
		if strings.Contains(content, "ID=arch") || strings.Contains(content, "ID=manjaro") {
			return DistroArch
		}
		if strings.Contains(content, "ID=debian") || strings.Contains(content, "ID=ubuntu") ||
			strings.Contains(content, "ID=linuxmint") || strings.Contains(content, "ID=kali") {
			return DistroDebian
		}
		if strings.Contains(content, "ID=fedora") || strings.Contains(content, "ID=centos") ||
			strings.Contains(content, "ID=rhel") || strings.Contains(content, "ID=rocky") ||
			strings.Contains(content, "ID=almalinux") || strings.Contains(content, "ID=ol") {
			return DistroRedHat
		}
	}
	return DistroUnknown
}

// hasSystemd checks if systemd is the init system
func hasSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// hasNetworkManager checks if NetworkManager is running
func hasNetworkManager() bool {
	cmd := exec.Command("systemctl", "is-active", "NetworkManager")
	output, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(output)) == "active"
}

// hasSystemdNetworkd checks if systemd-networkd is running
func hasSystemdNetworkd() bool {
	cmd := exec.Command("systemctl", "is-active", "systemd-networkd")
	output, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(output)) == "active"
}

// hasNftables checks if nftables is available and in use
func hasNftables() bool {
	// Check if nft command exists
	if _, err := exec.LookPath("nft"); err != nil {
		return false
	}
	// Check if nftables service is active or iptables is a symlink to nft
	if data, err := os.ReadFile("/etc/alternatives/iptables"); err == nil {
		if strings.Contains(string(data), "nft") {
			return true
		}
	}
	// Check if iptables --version mentions nf_tables
	cmd := exec.Command("iptables", "--version")
	if output, err := cmd.CombinedOutput(); err == nil {
		return strings.Contains(string(output), "nf_tables")
	}
	return false
}

// hasFirewalld checks if firewalld is running
func hasFirewalld() bool {
	cmd := exec.Command("systemctl", "is-active", "firewalld")
	output, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(output)) == "active"
}

func SetupNetwork() error {
	tapName := "litevm-tap0"
	hostIP := "172.16.0.1"
	uid := os.Getuid()

	if uid != 0 {
		return fmt.Errorf("%s", i18n.T("vmm.network.error.no_root_setup"))
	}

	// Get real user (the one who invoked sudo)
	realUID := os.Getuid()
	if sudoUser := os.Getenv("SUDO_UID"); sudoUser != "" {
		if id, err := strconv.Atoi(sudoUser); err == nil {
			realUID = id
		}
	}

	// Check if TAP already exists and is configured
	if tapDeviceExists(tapName) {
		fmt.Printf("%s\n", i18n.Tf("vmm.network.tap_exists", tapName))
	} else {
		// Create TAP owned by real user
		fmt.Printf("%s\n", i18n.Tf("vmm.network.creating_tap", tapName, realUID))
		if err := runCmdSilent("ip", "tuntap", "add", "dev", tapName, "mode", "tap", "user", strconv.Itoa(realUID)); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.create_tap"), err)
		}
		if err := runCmdSilent("ip", "addr", "add", hostIP+"/24", "dev", tapName); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.configure_tap_ip"), err)
		}
		if err := runCmdSilent("ip", "link", "set", tapName, "up"); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.bring_up_tap"), err)
		}
		fmt.Println(i18n.T("vmm.network.tap_created_ok"))
	}

	// Enable IP forwarding
	fmt.Println(i18n.T("vmm.network.enabling_ip_forward"))
	runCmdSilent("sysctl", "-w", "net.ipv4.ip_forward=1")

	// Setup NAT
	hostIface := FindHostInterface()
	if hostIface == "" {
		logger.Warn("No host interface found for NAT")
	} else {
		fmt.Printf("%s\n", i18n.Tf("vmm.network.setting_nat", hostIface))
		setupNAT(tapName, hostIface)
	}

	// Grant /dev/kvm access
	uidStr := strconv.Itoa(realUID)
	fmt.Printf("%s\n", i18n.Tf("vmm.network.granting_kvm", uidStr))
	if err := runCmdSilent("setfacl", "-m", "u:"+uidStr+":rw", "/dev/kvm"); err != nil {
		logger.Warn("Failed to set /dev/kvm ACL (setfacl may not be installed): %v", err)
		fmt.Println(i18n.T("vmm.network.tip_kvm_group"))
	}

	// Ensure /dev/net/tun is accessible
	fmt.Println(i18n.T("vmm.network.ensuring_tun"))
	runCmdSilent("chmod", "0666", "/dev/net/tun")

	// Persist network config for reboot survival
	// The systemd service will manage dnsmasq + NAT, so skip starting dnsmasq here
	fmt.Println(i18n.T("vmm.network.persisting"))
	family := detectDistroFamily()
	if err := persistNetworkConfig(family, tapName, hostIP, hostIface, realUID); err != nil {
		logger.Warn("Failed to persist network config: %v", err)
		fmt.Println(i18n.T("vmm.network.tip_no_persist"))
	}

	fmt.Println()
	fmt.Println(i18n.T("vmm.network.setup_complete"))
	fmt.Println()
	fmt.Println(i18n.T("vmm.network.persistent_hint"))
	fmt.Println()
	fmt.Println(i18n.T("vmm.network.run_without_root"))
	fmt.Printf("%s\n", i18n.Tf("vmm.network.run_hint", tapName))
	fmt.Println()
	fmt.Println(i18n.T("vmm.network.cleanup_hint"))
	fmt.Println(i18n.T("vmm.network.cleanup_hint_cmd"))
	return nil
}

func tapDeviceExists(tapName string) bool {
	cmd := exec.Command("ip", "link", "show", tapName)
	return cmd.Run() == nil
}

func TapDeviceAccessible(tapName string) bool {
	cmd := exec.Command("ip", "addr", "show", tapName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "172.16.0")
}

func tapExistsAndConfigured(tapName, prefix string) bool {
	return TapDeviceAccessible(tapName)
}

func runCmdSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Debug("Command failed: %s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(output)))
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func runCmdOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// getDnsmasqPath finds the dnsmasq binary path
func getDnsmasqPath() string {
	if path, err := exec.LookPath("dnsmasq"); err == nil {
		return path
	}
	// Common locations
	for _, p := range []string{"/usr/sbin/dnsmasq", "/usr/bin/dnsmasq", "/sbin/dnsmasq"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "dnsmasq"
}

// getIptablesPath finds the iptables binary path
func getIptablesPath() string {
	if path, err := exec.LookPath("iptables"); err == nil {
		return path
	}
	for _, p := range []string{"/usr/sbin/iptables", "/sbin/iptables"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "iptables"
}

func persistNetworkConfig(family DistroFamily, tapName, hostIP, hostIface string, realUID int) error {
	dnsmasqBin := getDnsmasqPath()
	iptablesBin := getIptablesPath()

	switch family {
	case DistroArch:
		return persistArch(family, tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
	case DistroDebian:
		return persistDebian(family, tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
	case DistroRedHat:
		return persistRedHat(family, tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
	default:
		// Try systemd-networkd as fallback
		if hasSystemd() {
			return persistSystemdNetworkd(tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
		}
		return fmt.Errorf("unsupported distribution for persistence")
	}
}

// persistArch - Arch Linux: systemd service handles everything
func persistArch(family DistroFamily, tapName, hostIP, hostIface string, realUID int, dnsmasqBin, iptablesBin string) error {
	// Create systemd service (manages TAP + IP + DHCP + NAT)
	if err := createDnsmasqService(dnsmasqBin, iptablesBin, tapName, hostIP, hostIface, realUID); err != nil {
		return err
	}

	// Persist IP forwarding
	persistIPForwarding()

	fmt.Println(i18n.T("vmm.network.persist_created_tap"))
	fmt.Println(i18n.T("vmm.network.persist_created_service"))
	fmt.Println(i18n.T("vmm.network.persist_enabled"))
	return nil
}

// persistDebian - Debian/Ubuntu: systemd-networkd (preferred) or NetworkManager
func persistDebian(family DistroFamily, tapName, hostIP, hostIface string, realUID int, dnsmasqBin, iptablesBin string) error {
	// Prefer systemd-networkd if available (modern Debian/Ubuntu)
	if hasSystemd() && (hasSystemdNetworkd() || !hasNetworkManager()) {
		return persistSystemdNetworkd(tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
	}

	// Fallback: NetworkManager dispatcher script
	return persistNetworkManager(tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
}

// persistRedHat - RHEL/CentOS/Fedora: NetworkManager (primary), firewalld support
func persistRedHat(family DistroFamily, tapName, hostIP, hostIface string, realUID int, dnsmasqBin, iptablesBin string) error {
	// If firewalld is active, use it; otherwise use direct iptables
	if hasFirewalld() {
		return persistRedHatFirewalld(tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
	}
	return persistNetworkManager(tapName, hostIP, hostIface, realUID, dnsmasqBin, iptablesBin)
}

// persistSystemdNetworkd - Generic systemd-based persistence
func persistSystemdNetworkd(tapName, hostIP, hostIface string, realUID int, dnsmasqBin, iptablesBin string) error {
	// Create systemd service (manages TAP + IP + DHCP + NAT)
	if err := createDnsmasqService(dnsmasqBin, iptablesBin, tapName, hostIP, hostIface, realUID); err != nil {
		return err
	}

	// Persist IP forwarding
	persistIPForwarding()

	fmt.Println(i18n.T("vmm.network.persist_created_tap"))
	fmt.Println(i18n.T("vmm.network.persist_created_service"))
	fmt.Println(i18n.T("vmm.network.persist_enabled"))
	return nil
}

// persistNetworkManager - NetworkManager dispatcher script
func persistNetworkManager(tapName, hostIP, hostIface string, realUID int, dnsmasqBin, iptablesBin string) error {
	// Create NetworkManager dispatcher script
	dispatcherDir := "/etc/NetworkManager/dispatcher.d"
	scriptFile := filepath.Join(dispatcherDir, "99-litevm-tap")

	scriptContent := fmt.Sprintf(`#!/bin/bash
# LiteVM TAP device auto-setup
# Triggered by NetworkManager on interface events

TAP_NAME="%s"
HOST_IP="%s"

case "$2" in
    up)
        # Create TAP if not exists
        if ! ip link show "$TAP_NAME" &>/dev/null; then
            ip tuntap add dev "$TAP_NAME" mode tap user %d
            ip addr add "$HOST_IP/24" dev "$TAP_NAME"
            ip link set "$TAP_NAME" up
        fi
        ;;
    down)
        # Leave TAP up - it may be in use
        ;;
esac
`, tapName, hostIP, realUID)
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0755); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.write_network"), err)
	}

	// Create systemd service (manages TAP + IP + DHCP + NAT)
	if err := createDnsmasqService(dnsmasqBin, iptablesBin, tapName, hostIP, hostIface, realUID); err != nil {
		return err
	}

	// Persist IP forwarding
	persistIPForwarding()

	fmt.Println(i18n.T("vmm.network.persist_created_tap"))
	fmt.Println(i18n.T("vmm.network.persist_created_service"))
	fmt.Println(i18n.T("vmm.network.persist_enabled"))
	return nil
}

// persistRedHatFirewalld - RHEL with firewalld
func persistRedHatFirewalld(tapName, hostIP, hostIface string, realUID int, dnsmasqBin, iptablesBin string) error {
	// Create TAP zone in firewalld
	zoneFile := "/etc/firewalld/zones/litevm.xml"
	zoneContent := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<zone>
  <short>LiteVM</short>
  <description>LiteVM TAP network zone</description>
  <interface name="%s"/>
  <masquerade/>
  <forward/>
</zone>`, tapName)
	if err := os.WriteFile(zoneFile, []byte(zoneContent), 0644); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.write_network"), err)
	}

	// Reload firewalld
	runCmdSilent("firewall-cmd", "--reload")

	// Add masquerade to public zone
	runCmdSilent("firewall-cmd", "--zone=public", "--add-masquerade")
	runCmdSilent("firewall-cmd", "--permanent", "--zone=public", "--add-masquerade")

	// Create systemd service (manages TAP + IP + DHCP + NAT)
	if err := createDnsmasqService(dnsmasqBin, iptablesBin, tapName, hostIP, hostIface, realUID); err != nil {
		return err
	}

	// Persist IP forwarding
	persistIPForwarding()

	fmt.Println(i18n.T("vmm.network.persist_created_tap"))
	fmt.Println(i18n.T("vmm.network.persist_created_service"))
	fmt.Println(i18n.T("vmm.network.persist_enabled"))
	return nil
}

// createDnsmasqService creates a systemd service that manages the full TAP lifecycle:
// create device → assign IP → enable forwarding → NAT → start dnsmasq
func createDnsmasqService(dnsmasqBin, iptablesBin, tapName, hostIP, hostIface string, realUID int) error {
	if hostIface == "" {
		hostIface = "eth0"
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=LiteVM network for %s (TAP + DHCP + NAT)
After=network.target

[Service]
Type=simple

# 1. Create TAP device (skip if exists)
ExecStartPre=/usr/bin/env sh -c 'ip link show %s >/dev/null 2>&1 || ip tuntap add dev %s mode tap user %d'
# 2. Assign IP and bring up (skip if already assigned)
ExecStartPre=/usr/bin/env sh -c 'ip addr show dev %s | grep -q %s || ip addr add %s/24 dev %s'
ExecStartPre=/usr/bin/env ip link set %s up
# 3. Enable IP forwarding
ExecStartPre=/usr/bin/env sh -c 'echo 1 > /proc/sys/net/ipv4/ip_forward'

# 4. Start dnsmasq (DHCP)
ExecStart=%s --no-daemon --no-resolv --no-poll --server=8.8.8.8 --server=1.1.1.1 -i %s -F %s,12h --bind-interfaces -I lo -q

# 5. Setup NAT after dnsmasq is up
ExecStartPost=/usr/bin/env sh -c 'iptables -t nat -A POSTROUTING -o %s -j MASQUERADE 2>/dev/null || true; iptables -A FORWARD -i %s -o %s -j ACCEPT 2>/dev/null || true; iptables -A FORWARD -i %s -o %s -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true'

# 6. Cleanup on explicit stop only (not on restart)
ExecStopPost=/usr/bin/env sh -c 'iptables -t nat -D POSTROUTING -o %s -j MASQUERADE 2>/dev/null; iptables -D FORWARD -i %s -o %s -j ACCEPT 2>/dev/null; iptables -D FORWARD -i %s -o %s -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null'

Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`, tapName,
		tapName, tapName, realUID,
		tapName, hostIP, hostIP, tapName,
		tapName,
		dnsmasqBin, tapName, "172.16.0.10,172.16.0.100,255.255.255.0",
		hostIface, tapName, hostIface, hostIface, tapName,
		hostIface, tapName, hostIface, hostIface, tapName)

	serviceFile := "/etc/systemd/system/litevm-network.service"
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.write_service"), err)
	}

	runCmdSilent("systemctl", "daemon-reload")
	runCmdSilent("systemctl", "enable", "litevm-network")
	runCmdSilent("systemctl", "start", "litevm-network")
	return nil
}

// persistIPForwarding persists IP forwarding across reboots
func persistIPForwarding() {
	sysctlFile := "/etc/sysctl.d/99-litevm.conf"
	sysctlContent := "net.ipv4.ip_forward=1\n"
	if err := os.WriteFile(sysctlFile, []byte(sysctlContent), 0644); err != nil {
		logger.Warn("Failed to persist IP forwarding: %v", err)
	}
}

func removeNetworkPersistence() error {
	var errs []error

	// 1. Stop and disable litevm-network service
	runCmdSilent("systemctl", "stop", "litevm-network")
	runCmdSilent("systemctl", "disable", "litevm-network")
	serviceFile := "/etc/systemd/system/litevm-network.service"
	if err := os.Remove(serviceFile); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove %s: %w", serviceFile, err))
	}

	// 2. Remove systemd-networkd config files
	ndFiles := []string{
		"/etc/systemd/network/20-litevm-tap.netdev",
		"/etc/systemd/network/21-litevm-tap.network",
	}
	for _, f := range ndFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", f, err))
		}
	}

	// 3. Remove NetworkManager dispatcher script
	nmScript := "/etc/NetworkManager/dispatcher.d/99-litevm-tap"
	if err := os.Remove(nmScript); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove %s: %w", nmScript, err))
	}

	// 4. Remove firewalld zone if exists
	firewalldZone := "/etc/firewalld/zones/litevm.xml"
	if err := os.Remove(firewalldZone); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove %s: %w", firewalldZone, err))
	}
	if hasFirewalld() {
		runCmdSilent("firewall-cmd", "--reload")
		runCmdSilent("firewall-cmd", "--permanent", "--zone=public", "--remove-masquerade")
	}

	// 5. Remove sysctl persistence
	sysctlFile := "/etc/sysctl.d/99-litevm.conf"
	if err := os.Remove(sysctlFile); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove %s: %w", sysctlFile, err))
	}

	// 6. Reload systemd and restart network services
	runCmdSilent("systemctl", "daemon-reload")
	if hasSystemdNetworkd() {
		runCmdSilent("systemctl", "restart", "systemd-networkd")
	}
	if hasNetworkManager() {
		runCmdSilent("systemctl", "restart", "NetworkManager")
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s: %v", i18n.T("vmm.network.error.cleanup"), errs)
	}
	return nil
}

func CleanupNetwork() error {
	tapName := "litevm-tap0"

	if os.Getuid() != 0 {
		return fmt.Errorf("%s", i18n.T("vmm.network.error.no_root_cleanup"))
	}

	// 1. Stop and disable systemd services
	fmt.Println(i18n.T("vmm.network.stopping_services"))
	runCmdSilent("systemctl", "stop", "litevm-network")
	runCmdSilent("systemctl", "disable", "litevm-network")

	// 2. Kill any running dnsmasq processes for this TAP
	fmt.Println(i18n.T("vmm.network.stopping_dhcp"))
	killDnsmasqForTap(tapName)

	// 3. Remove NAT/firewall rules
	fmt.Println(i18n.T("vmm.network.removing_nat"))
	removeAllNatRules(tapName)

	// 4. Remove TAP device
	if tapDeviceExists(tapName) {
		fmt.Printf("%s\n", i18n.Tf("vmm.network.removing_tap", tapName))
		if err := runCmdSilent("ip", "link", "set", tapName, "down"); err != nil {
			logger.Debug("Failed to bring down TAP: %v", err)
		}
		if err := runCmdSilent("ip", "tuntap", "del", "dev", tapName, "mode", "tap"); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("vmm.network.error.remove_tap"), err)
		}
	} else {
		fmt.Printf("%s\n", i18n.Tf("vmm.network.tap_not_exist", tapName))
	}

	// 5. Remove persistent config
	fmt.Println(i18n.T("vmm.network.removing_persist"))
	if err := removeNetworkPersistence(); err != nil {
		logger.Warn("Failed to remove some persistent config: %v", err)
	}

	// 6. Restore /dev/kvm permissions (remove ACL)
	fmt.Println(i18n.T("vmm.network.restoring_kvm"))
	realUID := os.Getuid()
	if sudoUser := os.Getenv("SUDO_UID"); sudoUser != "" {
		if id, err := strconv.Atoi(sudoUser); err == nil {
			realUID = id
		}
	}
	uid := strconv.Itoa(realUID)
	runCmdSilent("setfacl", "-x", "u:"+uid, "/dev/kvm")

	fmt.Println()
	fmt.Println(i18n.T("vmm.network.cleanup_complete"))
	fmt.Println(i18n.T("vmm.network.removed_service"))
	fmt.Println(i18n.T("vmm.network.removed_sysctl"))
	return nil
}

// killDnsmasqForTap kills all dnsmasq processes bound to the TAP interface
func killDnsmasqForTap(tapName string) {
	// Kill by PID file
	if pid, err := os.ReadFile("/tmp/litevm-dnsmasq.pid"); err == nil {
		runCmdSilent("kill", strings.TrimSpace(string(pid)))
		os.Remove("/tmp/litevm-dnsmasq.pid")
	}
	os.Remove("/tmp/litevm-dnsmasq.leases")

	// Also kill by process name filtering for this TAP
	if output, err := runCmdOutput("pgrep", "-f", "dnsmasq.*"+tapName); err == nil && output != "" {
		for _, pid := range strings.Split(output, "\n") {
			pid = strings.TrimSpace(pid)
			if pid != "" {
				runCmdSilent("kill", pid)
			}
		}
	}
}

// removeAllNatRules removes all NAT and firewall rules related to litevm
func removeAllNatRules(tapName string) {
	iptablesBin := getIptablesPath()

	// Remove iptables rules (try both with and without specific interface)
	rules := []string{
		fmt.Sprintf("-t nat -D POSTROUTING -o %s -j MASQUERADE", "litevm-tap0"),
		fmt.Sprintf("-D FORWARD -i %s -o %s -j ACCEPT", "litevm-tap0", "*"),
		fmt.Sprintf("-D FORWARD -i %s -o %s -m state --state RELATED,ESTABLISHED -j ACCEPT", "*", "litevm-tap0"),
	}

	for _, rule := range rules {
		args := strings.Fields(rule)
		cmd := exec.Command(iptablesBin, args...)
		cmd.Run() // Ignore errors - rule may not exist
	}

	// Also try with nft if available
	if hasNftables() {
		// Don't flush entire ruleset, just try to remove specific rules
		// This is safer - just ignore errors
		runCmdSilent("nft", "delete", "rule", "ip", "nat", "POSTROUTING", "handle", "last")
	}
}

func SetupTapDevice(cfg NetworkConfig) error {
	// Check if TAP already exists and is configured
	if TapDeviceAccessible(cfg.TapName) {
		logger.Debug("TAP device %s already exists, reusing", cfg.TapName)
	} else {
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
	}

	// Always ensure DHCP server is running (even if TAP was pre-created)
	if !dnsmasqIsRunning() {
		if err := startDHCP(cfg.TapName, cfg.HostIP, cfg.GuestIP); err != nil {
			logger.Warn("DHCP server failed (may need root): %v", err)
		}
	} else {
		logger.Debug("dnsmasq already running")
	}

	return nil
}

func CleanupTapDevice(tapName string) error {
	// If the systemd service is managing this TAP, don't touch anything
	cmd := exec.Command("systemctl", "is-active", "litevm-network")
	if output, err := cmd.CombinedOutput(); err == nil && strings.TrimSpace(string(output)) == "active" {
		logger.Debug("litevm-network service is managing %s, skipping cleanup", tapName)
		return nil
	}

	// Kill dnsmasq if running
	killDnsmasqForTap(tapName)

	// Remove NAT rules
	removeNAT(tapName)

	cmd = exec.Command("ip", "link", "del", tapName)
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "cannot find device") {
			// Try with sudo
			cmd = exec.Command("sudo", "-n", "ip", "link", "del", tapName)
			if output, err := cmd.CombinedOutput(); err != nil {
				if !strings.Contains(string(output), "cannot find device") {
					return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.delete_tap", tapName), err)
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
				return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.create_tap_need_root", string(output)), err)
			}
		} else {
			return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.create_tap_generic", string(output)), err)
		}
	}

	// Bring up the tap device
	cmd = exec.Command("ip", "link", "set", tapName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			cmd = exec.Command("sudo", "-n", "ip", "link", "set", tapName, "up")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.bring_up_tap_generic", string(output)), err)
			}
		} else {
			return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.bring_up_tap_generic", string(output)), err)
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
				return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.configure_tap_ip_generic", string(output)), err)
			}
		} else {
			return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.configure_tap_ip_generic", string(output)), err)
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
			return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.ip_forward", string(output)), err)
		}
	}
	return nil
}

func runIptables(args []string) error {
	iptablesBin := getIptablesPath()
	cmd := exec.Command(iptablesBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			sudoArgs := append([]string{"-n", iptablesBin}, args...)
			cmd = exec.Command("sudo", sudoArgs...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.iptables", strings.TrimSpace(string(output))), err)
			}
		} else {
			return fmt.Errorf("%s: %w", i18n.Tf("vmm.network.error.iptables", strings.TrimSpace(string(output))), err)
		}
	}
	return nil
}

func startDHCP(tapName, hostIP, guestIP string) error {
	dhcpRange := "172.16.0.10,172.16.0.100,255.255.255.0,12h"
	dnsmasqBin := getDnsmasqPath()

	args := []string{
		"-i", tapName,
		"-F", dhcpRange,
		"--bind-interfaces",
		"-I", "lo",
		"--pid-file=/tmp/litevm-dnsmasq.pid",
		"--dhcp-leasefile=/tmp/litevm-dnsmasq.leases",
		"-q",
	}

	// Kill existing dnsmasq first to avoid duplicates
	killDnsmasqForTap(tapName)

	// Try dnsmasq without sudo first
	cmd := exec.Command(dnsmasqBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Operation not permitted") {
			sudoArgs := append([]string{"-n", dnsmasqBin}, args...)
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

func dnsmasqIsRunning() bool {
	// Check via our PID file first
	if pid, err := os.ReadFile("/tmp/litevm-dnsmasq.pid"); err == nil {
		pidStr := strings.TrimSpace(string(pid))
		cmd := exec.Command("kill", "-0", pidStr)
		if cmd.Run() == nil {
			return true
		}
	}
	// Check if litevm-network service is managing dnsmasq
	cmd := exec.Command("systemctl", "is-active", "litevm-network")
	if output, err := cmd.CombinedOutput(); err == nil && strings.TrimSpace(string(output)) == "active" {
		return true
	}
	// Check if any dnsmasq is listening on port 67
	cmd = exec.Command("ss", "-ulnp")
	if output, err := cmd.CombinedOutput(); err == nil {
		return strings.Contains(string(output), "dnsmasq")
	}
	return false
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
	runIptables(strings.Fields("-t nat -D POSTROUTING -o litevm-tap0 -j MASQUERADE"))
	runIptables(strings.Fields("-D FORWARD -i litevm-tap0 -o eth0 -j ACCEPT"))
	runIptables(strings.Fields("-D FORWARD -i eth0 -o litevm-tap0 -m state --state RELATED,ESTABLISHED -j ACCEPT"))
}

func FindHostInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	exclude := map[string]bool{
		"lo":          true,
		"litevm-tap0": true,
	}

	for _, iface := range ifaces {
		if exclude[iface.Name] {
			continue
		}
		if strings.HasPrefix(iface.Name, "litevm-tap") {
			continue
		}
		// Skip virtual interfaces
		if strings.HasPrefix(iface.Name, "virbr") || strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "br-") || strings.HasPrefix(iface.Name, "veth") {
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
