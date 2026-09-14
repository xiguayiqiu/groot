package vmm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"litevm/internal/logger"
)

const (
	defaultKernelPath    = "kernel/amd64/vmlinux.bin"
	defaultRootfsPath    = "rootfs/rootfs.ext4"
	defaultSocketPath    = "/tmp/litevm-firecracker.socket"
	defaultKernelCmdline = "console=ttyS0,115200n8 reboot=k panic=1 nomodule"
)

type Config struct {
	KernelPath  string
	RootfsPath  string
	SocketPath  string
	MemSizeMB   int
	Vcpus       int
	EnableNet   bool
	TapDevice   string
	HostIP      string
	GuestIP     string
	KernelArgs  string
}

func DefaultConfig() Config {
	arch := runtime.GOARCH
	kernelPath := defaultKernelPath
	if arch == "arm64" {
		kernelPath = "kernel/arm64/vmlinux.bin"
	}

	return Config{
		KernelPath: kernelPath,
		RootfsPath: defaultRootfsPath,
		SocketPath: defaultSocketPath,
		MemSizeMB:  256,
		Vcpus:      2,
		EnableNet:  false,
		KernelArgs: defaultKernelCmdline,
	}
}

func Run(cfg Config) error {
	if err := validatePaths(cfg); err != nil {
		return err
	}

	firecrackerBin, err := findFirecracker()
	if err != nil {
		return err
	}

	if err := cleanupSocket(cfg.SocketPath); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fcCfg := buildConfig(cfg)

	cmd := firecracker.VMCommandBuilder{}.
		WithBin(firecrackerBin).
		WithSocketPath(cfg.SocketPath).
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	m, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		return fmt.Errorf("failed to create machine: %w", err)
	}

	installSignalHandlers(ctx, m)

	logger.Info("Starting microVM...")
	if err := m.Start(ctx); err != nil {
		return fmt.Errorf("failed to start machine: %w", err)
	}
	defer func() {
		if err := m.StopVMM(); err != nil {
			logger.Error("Error stopping VMM: %v", err)
		}
	}()

	logger.Info("microVM started. Press Ctrl+A then x to exit.")
	if err := m.Wait(ctx); err != nil {
		return fmt.Errorf("machine wait error: %w", err)
	}

	logger.Info("microVM exited")
	return nil
}

func validatePaths(cfg Config) error {
	if _, err := os.Stat(cfg.KernelPath); os.IsNotExist(err) {
		return fmt.Errorf("kernel not found: %s", cfg.KernelPath)
	}
	if _, err := os.Stat(cfg.RootfsPath); os.IsNotExist(err) {
		return fmt.Errorf("rootfs not found: %s", cfg.RootfsPath)
	}
	return nil
}

func findFirecracker() (string, error) {
	homeDir, _ := os.UserHomeDir()
	paths := []string{
		"firecracker",
		"/usr/local/bin/firecracker",
		"/opt/firecracker/firecracker",
	}
	if homeDir != "" {
		paths = append(paths, filepath.Join(homeDir, "bin/firecracker"))
	}
	for _, p := range paths {
		if resolved, err := exec.LookPath(p); err == nil {
			return resolved, nil
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("firecracker binary not found. Install from https://github.com/firecracker-microvm/firecracker/releases")
}

func cleanupSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old socket: %w", err)
	}
	return nil
}

func buildConfig(cfg Config) firecracker.Config {
	driveID := "rootfs"
	isRootDevice := true
	isReadOnly := false

	kernelArgs := cfg.KernelArgs
	if cfg.EnableNet {
		// Use DHCP for automatic IP configuration
		kernelArgs += " ip=dhcp"
	}

	fcCfg := firecracker.Config{
		SocketPath:      cfg.SocketPath,
		KernelImagePath: filepath.Clean(cfg.KernelPath),
		KernelArgs:      kernelArgs,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:   firecracker.Int64(int64(cfg.Vcpus)),
			MemSizeMib:  firecracker.Int64(int64(cfg.MemSizeMB)),
			Smt:         firecracker.Bool(false),
		},
		Drives: []models.Drive{
			{
			 DriveID:      &driveID,
			 PathOnHost:   firecracker.String(filepath.Clean(cfg.RootfsPath)),
			 IsRootDevice: &isRootDevice,
			 IsReadOnly:   &isReadOnly,
			},
		},
		LogLevel:    "Info",
		MetricsPath: "/dev/null",
	}

	if cfg.EnableNet && cfg.TapDevice != "" {
		fcCfg.NetworkInterfaces = []firecracker.NetworkInterface{
			{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					MacAddress:  "AA:FC:00:00:00:01",
					HostDevName: cfg.TapDevice,
				},
			},
		}
	}

	return fcCfg
}

func installSignalHandlers(ctx context.Context, m *firecracker.Machine) {
	go func() {
		signal.Reset(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				logger.Info("Received signal, shutting down...")
				if err := m.Shutdown(ctx); err != nil {
					logger.Error("Shutdown error: %v", err)
				}
			case s == syscall.SIGQUIT:
				logger.Info("Received SIGQUIT, force stopping...")
				if err := m.StopVMM(); err != nil {
					logger.Error("StopVMM error: %v", err)
				}
			}
		}
	}()
}
