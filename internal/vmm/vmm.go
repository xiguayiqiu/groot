package vmm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"litevm/internal/i18n"
	"litevm/internal/logger"
)

const (
	defaultKernelPath    = "kernel/amd64/vmlinux.bin"
	defaultRootfsPath    = "rootfs/rootfs.ext4"
	defaultSocketPath    = "/tmp/litevm-firecracker.socket"
	defaultKernelCmdline = "console=ttyS0,115200n8 reboot=k panic=1 nomodule loglevel=5"
)

// guestShutdownMarkers 是 Linux 内核 / init 系统在 guest 关机停机时打印到串口的
// 固定标记文本。Firecracker 在 guest 执行 poweroff / halt 后不会自行退出（进程
// 会一直卡在 vCPU halt 状态），litevm 需要根据这些标记主动停止 VMM，才能回到
// 宿主的 shell。这些标记由内核在最后阶段打印（"reboot: System halted" 等），
// 与 guest 使用 systemd / openrc / runit 无关。
var guestShutdownMarkers = []string{
	"system halted",               // Linux 内核 halt（无 ACPI 的 poweroff 也走这里）
	"power down",                  // Linux 内核 ACPI poweroff（"reboot: Power down"）
	"powering off",                // systemd（Debian/Ubuntu 关机时打印）
	"power off now",               // sysvinit
	"requesting system poweroff",  // sysvinit
	"requesting system halt",      // sysvinit
}

// guestRebootMarkers 是 guest 执行 reboot 时内核 / init 系统打印的固定标记。
// 与关机不同，guest reboot 会让内核 reset CPU，导致 Firecracker 进程退出
// （Firecracker 的已知行为："Firecracker exits on CPU reset"），因此 litevm
// 需要识别这些标记，在进程退出后重新拉起同一个 microVM，实现"正常重启"。
var guestRebootMarkers = []string{
	"restarting system",           // Linux 内核（"reboot: Restarting system"）
	"machine restart",             // Linux 内核（"reboot: machine restart"）
	"requesting system restart",   // sysvinit
	"requesting system reboot",    // sysvinit
	"for reboot now",              // sysvinit（"The system is going down for reboot NOW"）
}

// DiskFormat represents the on-disk format of a virtual disk image.
type DiskFormat int

const (
	DiskFormatRaw    DiskFormat = iota // raw / dd / img
	DiskFormatQCOW2                    // QEMU Copy-On-Write v2
	DiskFormatVMDK                     // VMware Virtual Machine Disk
	DiskFormatVHD                      // Microsoft Virtual Hard Disk
	DiskFormatISO                      // ISO 9660 optical disc image
	DiskFormatUnknown
)

func (f DiskFormat) String() string {
	switch f {
	case DiskFormatRaw:
		return "raw"
	case DiskFormatQCOW2:
		return "qcow2"
	case DiskFormatVMDK:
		return "vmdk"
	case DiskFormatVHD:
		return "vhd"
	case DiskFormatISO:
		return "iso"
	default:
		return "unknown"
	}
}

// DetectDiskFormat detects the disk format from the file extension and magic bytes.
func DetectDiskFormat(path string) DiskFormat {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".iso":
		return DiskFormatISO
	case ".qcow2":
		return DiskFormatQCOW2
	case ".vmdk":
		return DiskFormatVMDK
	case ".vhd", ".vhdx":
		return DiskFormatVHD
	case ".raw", ".img", ".bin", ".ext4", ".ext3", ".ext2", ".rootfs":
		return DiskFormatRaw
	}

	// Fallback: check magic bytes
	f, err := os.Open(path)
	if err != nil {
		return DiskFormatUnknown
	}
	defer f.Close()

	header := make([]byte, 16)
	if _, err := f.Read(header); err != nil {
		return DiskFormatUnknown
	}

	// QCOW2 magic: "QFI\xfb"
	if len(header) >= 4 && header[0] == 'Q' && header[1] == 'F' && header[2] == 'I' && header[3] == 0xfb {
		return DiskFormatQCOW2
	}
	// VMDK magic: "KDMV" (VMDK sparse) or "VMDK" (VMDK descriptor)
	if len(header) >= 4 && ((header[0] == 'K' && header[1] == 'D' && header[2] == 'M' && header[3] == 'V') ||
		(header[0] == 'V' && header[1] == 'M' && header[2] == 'D' && header[3] == 'K')) {
		return DiskFormatVMDK
	}
	// VHD magic: "conectix"
	if len(header) >= 8 && header[0] == 'c' && header[1] == 'o' && header[2] == 'n' && header[3] == 'e' &&
		header[4] == 'c' && header[5] == 't' && header[6] == 'i' && header[7] == 'x' {
		return DiskFormatVHD
	}
	// ISO 9660: "CD001" at offset 0x8001
	isoHeader := make([]byte, 6)
	if _, err := f.ReadAt(isoHeader, 0x8001); err == nil {
		if isoHeader[0] == 'C' && isoHeader[1] == 'D' && isoHeader[2] == '0' && isoHeader[3] == '0' && isoHeader[4] == '1' {
			return DiskFormatISO
		}
	}

	// If it has a common raw-like extension, treat as raw
	switch ext {
	case ".img", ".raw", ".bin", ".ext4", ".ext3", ".ext2", ".rootfs":
		return DiskFormatRaw
	}

	return DiskFormatUnknown
}

// IsReadOnlyFormat returns true if the format is naturally read-only (ISO).
func IsReadOnlyFormat(f DiskFormat) bool {
	return f == DiskFormatISO
}

// ConvertToRaw converts a disk image from the given format to raw format.
// Returns the path to the raw file. If the input is already raw, returns the original path.
// The caller should NOT remove the returned path if it is the same as inputPath.
func ConvertToRaw(inputPath string, format DiskFormat) (string, func(), error) {
	if format == DiskFormatRaw || format == DiskFormatISO {
		return inputPath, func() {}, nil
	}

	qemuImg, err := findQemuImg()
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", i18n.T("vmm.disk.error.qemu_img_not_found"), err)
	}

	outputPath := inputPath + ".raw"
	// Remove stale converted file
	os.Remove(outputPath)

	logger.Info("Converting %s (%s) to raw format...", filepath.Base(inputPath), format)
	cmd := exec.Command(qemuImg, "convert", "-f", format.String(), "-O", "raw", inputPath, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(outputPath)
		return "", nil, fmt.Errorf("qemu-img convert failed: %w", err)
	}

	cleanup := func() { os.Remove(outputPath) }
	return outputPath, cleanup, nil
}

// findQemuImg locates the qemu-img binary.
func findQemuImg() (string, error) {
	if path, err := exec.LookPath("qemu-img"); err == nil {
		return path, nil
	}
	for _, p := range []string{"/usr/bin/qemu-img", "/usr/local/bin/qemu-img", "/opt/qemu/bin/qemu-img"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("qemu-img not found. Install qemu-utils (Debian/Ubuntu) or qemu-img (RHEL/Fedora)")
}

// CreateRawDisk creates a new raw ext4 disk of the specified size.
// sizeStr accepts formats like "512M", "1G", "10G".
func CreateRawDisk(outputPath string, sizeStr string) error {
	qemuImg, err := findQemuImg()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("vmm.disk.error.qemu_img_not_found"), err)
	}

	cmd := exec.Command(qemuImg, "create", "-f", "raw", outputPath, sizeStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create raw disk: %w", err)
	}

	// Format as ext4
	mkfs, err := exec.LookPath("mkfs.ext4")
	if err != nil {
		return fmt.Errorf("mkfs.ext4 not found: %w", err)
	}

	logger.Info("Formatting %s as ext4...", filepath.Base(outputPath))
	cmd = exec.Command(mkfs, "-F", "-q", outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("mkfs.ext4 failed: %w", err)
	}

	return nil
}

// DriveConfig represents an additional drive to attach to the VM.
type DriveConfig struct {
	ID         string // unique drive ID
	Path       string // host path (may be qcow2/vmdk/vhd/raw/iso)
	ReadOnly   bool   // mount read-only
	IsRoot     bool   // is this the root device
	CacheType  string // "Unsafe" or "Writeback"
}

// BalloonConfig represents a virtio-balloon device configuration.
type BalloonConfig struct {
	AmountMib           int64  // target balloon size in MiB
	DeflateOnOom        bool   // deflate when guest has memory pressure
	StatsPollingSeconds int64  // interval for statistics refresh, 0 = disabled
}

// VsockConfig represents a virtio-vsock device configuration.
type VsockConfig struct {
	GuestCID int64  // guest CID (must be >= 3)
	UDSPath  string // host-side Unix domain socket path
}

// serialTailMax 是串口滚动窗口大小：只扫描最近一小段输出，避免误匹配到运行期
// 偶然出现的相似文本。
const serialTailMax = 4096

// serialConsole 上报的事件类型
const (
	evPowerOff = 1 // guest 已关机/停机：停止 VMM 并退出 litevm
	evReboot   = 2 // guest 正在重启：需要重新拉起 microVM
)

// waitForVM 的返回值
const (
	vmStop    = 0 // 本次 microVM 会话结束（关机/异常退出），退出 Run
	vmRestart = 1 // 检测到 guest 重启：重新拉起 microVM
)

// 检测到重启标记后，等待 Firecracker 进程因 CPU reset 退出的宽限期；
// 若超时说明 Firecracker 已原地完成重启（进程未退出），只需重新武装监视。
const rebootExitGrace = 5 * time.Second

// guest 关机后停止 VMM（SIGTERM）的宽限期，超时后改用 SIGKILL。
const stopVMMGrace = 10 * time.Second

// serialConsole 是一个 tee 型的 io.Writer：把 Firecracker 的串口（guest 控制台）
// 输出原样转发到宿主终端，同时扫描最近一段输出，检测 guest 是关机停机还是重启。
// 一旦命中对应标记，就通过 event 通道把事件类型通知主流程。
type serialConsole struct {
	dst        io.Writer
	event      chan int
	poweredOff bool
	rebooted   bool
	triggered  bool
	tail       []byte
}

func (s *serialConsole) Write(p []byte) (int, error) {
	n := len(p)

	if s.dst != nil {
		if _, err := s.dst.Write(p); err != nil {
			return 0, err
		}
	}

	if !s.triggered {
		s.scan(p)
	}

	return n, nil
}

// scan 把新写入的数据追加到滚动窗口并检查关机/重启标记。
func (s *serialConsole) scan(p []byte) {
	s.tail = append(s.tail, p...)
	if len(s.tail) > serialTailMax {
		s.tail = s.tail[len(s.tail)-serialTailMax:]
	}

	low := strings.ToLower(string(s.tail))
	for _, marker := range guestShutdownMarkers {
		if strings.Contains(low, marker) {
			s.poweredOff = true
			s.triggered = true
			s.signal(evPowerOff)
			return
		}
	}
	for _, marker := range guestRebootMarkers {
		if strings.Contains(low, marker) {
			s.rebooted = true
			s.triggered = true
			s.signal(evReboot)
			return
		}
	}
}

// signal 非阻塞地向 event 通道发送一个事件（通道容量为 1，最多发送一次）。
func (s *serialConsole) signal(ev int) {
	select {
	case s.event <- ev:
	default:
	}
}

// Reset 清空已触发状态，供"原地重启（进程未退出）"后重新武装监视使用。
func (s *serialConsole) Reset() {
	s.tail = nil
	s.triggered = false
	s.poweredOff = false
	s.rebooted = false
}

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

	ExtraDrives []DriveConfig // additional block devices (data disks, ISO, etc.)
	Balloon     *BalloonConfig // optional balloon device
	Vsock       *VsockConfig   // optional vsock device
}

func DefaultConfig() Config {
	arch := runtime.GOARCH
	kernelPath := defaultKernelPath
	if arch == "arm64" {
		kernelPath = "kernel/arm64/vmlinux.bin"
	}

	socketPath := defaultSocketPath
	// 允许用环境变量覆盖 API socket 路径（便于多实例/避免权限冲突）。
	if env := os.Getenv("LITEVM_FIRECRACKER_SOCKET"); env != "" {
		socketPath = env
	}

	return Config{
		KernelPath: kernelPath,
		RootfsPath: defaultRootfsPath,
		SocketPath: socketPath,
		MemSizeMB:  1024,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 信号处理器整个生命周期只安装一次，通过 atomic 引用始终指向"当前的"
	// machine，这样 guest 重启后 Ctrl+C 等信号依然作用于最新启动的 VM。
	// sig 记录用户（宿主侧）是否主动发起了关闭请求：此时 guest 即使按
	// ctrl-alt-del 走了重启路径（SendCtrlAltDel 会触发 guest 重启），
	// litevm 也应该退出，而不是重新拉起 microVM。
	var active atomic.Value
	var sig atomic.Bool
	installSignalHandlers(ctx, &active, &sig)

	// 外层循环：guest 执行 reboot 会让内核 reset CPU，导致 Firecracker 进程
	// 退出（对应 Wait 返回），此时用相同的配置重新拉起 microVM，实现"正常重启"。
	for {
		if err := cleanupSocket(cfg.SocketPath); err != nil {
			return err
		}

		fcCfg := buildConfig(cfg)

		// 把串口输出接一个 tee：既转发到宿主终端，又监视 guest 关机/重启标记。
		eventCh := make(chan int, 1)
		console := &serialConsole{dst: os.Stdout, event: eventCh}

		cmd := firecracker.VMCommandBuilder{}.
			WithBin(firecrackerBin).
			WithSocketPath(cfg.SocketPath).
			WithStdin(os.Stdin).
			WithStdout(console).
			WithStderr(os.Stderr).
			Build(ctx)

		m, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithProcessRunner(cmd))
		if err != nil {
			return fmt.Errorf("failed to create machine: %w", err)
		}
		defer func() {
			if err := m.StopVMM(); err != nil {
				logger.Error("Error stopping VMM: %v", err)
			}
		}()
		active.Store(m)

		logger.Info("Starting microVM...")
		if err := m.Start(ctx); err != nil {
			return fmt.Errorf("failed to start machine: %w", err)
		}

		// Create balloon device after machine starts (if configured)
		if cfg.Balloon != nil {
			if err := m.CreateBalloon(ctx, cfg.Balloon.AmountMib, cfg.Balloon.DeflateOnOom, cfg.Balloon.StatsPollingSeconds); err != nil {
				logger.Warn("Failed to create balloon device: %v", err)
			} else {
				logger.Info("Balloon device created: %d MB", cfg.Balloon.AmountMib)
			}
		}

		logger.Info("microVM started. Press Ctrl+A then x to exit.")

		waitDone := make(chan error, 1)
		go func() {
			waitDone <- m.Wait(ctx)
		}()

		result, waitErr := waitForVM(ctx, m, waitDone, eventCh, console, &sig)
		if result == vmRestart {
			logger.Info("Guest rebooted, restarting microVM...")
			continue
		}
		if waitErr != nil {
			return waitErr
		}
		break
	}

	logger.Info("microVM exited")
	return nil
}

// waitForVM 等待当前 microVM 结束，并根据串口监视到的事件处理 guest 关机/重启。
// 返回值：
//   vmRestart：guest 执行了 reboot，需要重新拉起 microVM；
//   否则返回 vmStop 和（可能为 nil 的）错误。
func waitForVM(ctx context.Context, m *firecracker.Machine, waitDone chan error, eventCh chan int, console *serialConsole, sig *atomic.Bool) (int, error) {
	for {
		select {
		case err := <-waitDone:
			// Firecracker 进程已退出：若退出前出现重启标记，说明这是 guest
			// reboot（CPU reset），需要重新拉起；但如果是宿主主动关闭
			// （Ctrl+C 等触发 ctrl-alt-del），则直接退出 litevm。
			if console.rebooted {
				if err != nil {
					logger.Debug("VM exited after reboot: %v", err)
				}
				if sig.Load() {
					return vmStop, nil
				}
				return vmRestart, nil
			}
			if console.poweredOff {
				// 理论上关机后进程不会自行退出；万一出现了，按正常结束处理。
				return vmStop, nil
			}
			return vmStop, err
		case ev := <-eventCh:
			switch ev {
			case evReboot:
				// guest 正在重启：内核随即 reset CPU，Firecracker 进程退出。
				// 若在宽限期后仍未退出，说明 VMM 已原地完成重启，重新武装监视。
				logger.Info("Guest reboot request detected, waiting for microVM to reset...")
				select {
				case err := <-waitDone:
					if err != nil {
						logger.Debug("VM exited after reboot: %v", err)
					}
					if sig.Load() {
						return vmStop, nil
					}
					return vmRestart, nil
				case <-time.After(rebootExitGrace):
					logger.Info("microVM rebooted in place, continuing to monitor")
					console.Reset()
				}
			case evPowerOff:
				logger.Info("Guest powered off, stopping microVM...")
				if err := m.StopVMM(); err != nil {
					logger.Error("Error stopping VMM: %v", err)
				}
				select {
				case err := <-waitDone:
					if err != nil {
						logger.Debug("VM process exited after shutdown: %v", err)
					}
					return vmStop, nil
				case <-time.After(stopVMMGrace):
					logger.Warn("VMM did not stop in time, force killing...")
					if pid, pidErr := m.PID(); pidErr == nil {
						if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
							logger.Error("Error force killing VMM: %v", killErr)
						}
					}
					if err := <-waitDone; err != nil {
						logger.Debug("VM process killed after shutdown: %v", err)
					}
					return vmStop, nil
				}
			}
		}
	}
}

func validatePaths(cfg Config) error {
	if _, err := os.Stat(cfg.KernelPath); os.IsNotExist(err) {
		return fmt.Errorf("kernel not found: %s", cfg.KernelPath)
	}
	if _, err := os.Stat(cfg.RootfsPath); os.IsNotExist(err) {
		return fmt.Errorf("rootfs not found: %s", cfg.RootfsPath)
	}
	for _, d := range cfg.ExtraDrives {
		if _, err := os.Stat(d.Path); os.IsNotExist(err) {
			return fmt.Errorf("drive not found: %s", d.Path)
		}
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
		kernelArgs += " ip=dhcp"
	}

	// Build drives list: root drive first, then extra drives
	drives := []models.Drive{
		{
			DriveID:      &driveID,
			PathOnHost:   firecracker.String(filepath.Clean(cfg.RootfsPath)),
			IsRootDevice: &isRootDevice,
			IsReadOnly:   &isReadOnly,
		},
	}

	for _, extra := range cfg.ExtraDrives {
		id := extra.ID
		ro := extra.ReadOnly
		d := models.Drive{
			DriveID:      &id,
			PathOnHost:   firecracker.String(filepath.Clean(extra.Path)),
			IsRootDevice: firecracker.Bool(extra.IsRoot),
			IsReadOnly:   &ro,
		}
		if extra.CacheType != "" {
			d.CacheType = &extra.CacheType
		}
		drives = append(drives, d)
	}

	fcCfg := firecracker.Config{
		SocketPath:      cfg.SocketPath,
		KernelImagePath: filepath.Clean(cfg.KernelPath),
		KernelArgs:      kernelArgs,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(cfg.Vcpus)),
			MemSizeMib: firecracker.Int64(int64(cfg.MemSizeMB)),
			Smt:        firecracker.Bool(false),
		},
		Drives:      drives,
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

	// Balloon device is configured after machine creation via CreateBalloon()
	if cfg.Vsock != nil {
		cid := uint32(cfg.Vsock.GuestCID)
		uds := cfg.Vsock.UDSPath
		fcCfg.VsockDevices = []firecracker.VsockDevice{
			{
				ID:   "vsock0",
				Path: uds,
				CID:  cid,
			},
		}
	}

	return fcCfg
}

func installSignalHandlers(ctx context.Context, active *atomic.Value, sig *atomic.Bool) {
	go func() {
		signal.Reset(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				logger.Info("Received signal, shutting down...")
				sig.Store(true)
				if m := active.Load().(*firecracker.Machine); m != nil {
					if err := m.Shutdown(ctx); err != nil {
						logger.Error("Shutdown error: %v", err)
					}
				}
			case s == syscall.SIGQUIT:
				logger.Info("Received SIGQUIT, force stopping...")
				sig.Store(true)
				if m := active.Load().(*firecracker.Machine); m != nil {
					if err := m.StopVMM(); err != nil {
						logger.Error("StopVMM error: %v", err)
					}
				}
			}
		}
	}()
}
