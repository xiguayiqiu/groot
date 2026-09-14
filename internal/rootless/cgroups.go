package rootless

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/fs2"
	"golang.org/x/sys/unix"
	"litevm/internal/logger"
)

type CgroupConfig struct {
	Name       string
	Path       string
	Memory     int64
	MemorySwap int64
	CPUQuota   int64
	CPUShares  uint64
	CPUPeriod  uint64
	PidsLimit  int64
	Rootless   bool
}

type CgroupManager struct {
	config   *CgroupConfig
	manager  cgroups.Manager
	cgPath   string
}

func IsCgroupsV2Available() bool {
	var stat unix.Statfs_t
	if err := unix.Statfs("/sys/fs/cgroup", &stat); err != nil {
		return false
	}
	return stat.Type == unix.CGROUP2_SUPER_MAGIC
}

func NewCgroupManager(cfg *CgroupConfig) (*CgroupManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cgroup config is nil")
	}

	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("litevm-%d", os.Getpid())
	}

	cgPath := filepath.Join("/sys/fs/cgroup", cfg.Name)

	cm := &CgroupManager{
		config: cfg,
		cgPath: cgPath,
	}

	return cm, nil
}

func (cm *CgroupManager) Setup() error {
	if !IsCgroupsV2Available() {
		logger.Debug("cgroups v2 not available, skipping resource limits")
		return nil
	}

	if err := os.MkdirAll(cm.cgPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup path: %w", err)
	}

	cgConfig := &cgroups.Cgroup{
		Name:   cm.config.Name,
		Path:   cm.cgPath,
		Rootless: cm.config.Rootless,
	}

	resources := &cgroups.Resources{}

	if cm.config.Memory > 0 {
		resources.Memory = cm.config.Memory
		logger.Info("cgroup memory limit: %d bytes", cm.config.Memory)
	}

	if cm.config.MemorySwap > 0 {
		resources.MemorySwap = cm.config.MemorySwap
	}

	if cm.config.CPUQuota > 0 {
		resources.CpuQuota = cm.config.CPUQuota
		resources.CpuPeriod = 100000
		logger.Info("cgroup CPU quota: %d/100000", cm.config.CPUQuota)
	}

	if cm.config.CPUShares > 0 {
		resources.CpuShares = cm.config.CPUShares
	}

	if cm.config.PidsLimit > 0 {
		resources.PidsLimit = &cm.config.PidsLimit
		logger.Info("cgroup pids limit: %d", cm.config.PidsLimit)
	}

	cgConfig.Resources = resources

	mgr, err := fs2.NewManager(cgConfig, cm.cgPath)
	if err != nil {
		return fmt.Errorf("failed to create cgroup manager: %w", err)
	}

	cm.manager = mgr

	logger.Info("cgroup v2 setup: path=%s", cm.cgPath)
	return nil
}

func (cm *CgroupManager) Apply(pid int) error {
	if cm.manager == nil {
		return nil
	}

	if err := cm.manager.Apply(pid); err != nil {
		return fmt.Errorf("failed to apply cgroup to pid %d: %w", pid, err)
	}

	logger.Debug("cgroup applied to pid %d: %s", pid, cm.cgPath)
	return nil
}

func (cm *CgroupManager) GetStats() (*cgroups.Stats, error) {
	if cm.manager == nil {
		return nil, fmt.Errorf("cgroup manager not initialized")
	}

	stats, err := cm.manager.GetStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get cgroup stats: %w", err)
	}

	return stats, nil
}

func (cm *CgroupManager) SetResources(resources *cgroups.Resources) error {
	if cm.manager == nil {
		return fmt.Errorf("cgroup manager not initialized")
	}

	if err := cm.manager.Set(resources); err != nil {
		return fmt.Errorf("failed to set cgroup resources: %w", err)
	}

	return nil
}

func (cm *CgroupManager) Destroy() error {
	if cm.manager == nil {
		return nil
	}

	if err := cm.manager.Destroy(); err != nil {
		logger.Debug("failed to destroy cgroup: %v", err)
	}

	if cm.cgPath != "" {
		if err := os.Remove(cm.cgPath); err != nil && !os.IsNotExist(err) {
			logger.Debug("failed to remove cgroup dir: %v", err)
		}
	}

	logger.Debug("cgroup destroyed: %s", cm.cgPath)
	return nil
}

func (cm *CgroupManager) GetPath() string {
	return cm.cgPath
}

func (cm *CgroupManager) Freeze(state cgroups.FreezerState) error {
	if cm.manager == nil {
		return nil
	}
	return cm.manager.Freeze(state)
}

func (cm *CgroupManager) Exists() bool {
	if cm.manager == nil {
		return false
	}
	return cm.manager.Exists()
}

func ApplyResourceLimits(cfg *CgroupConfig, pid int) (*CgroupManager, error) {
	if cfg == nil || (cfg.Memory == 0 && cfg.CPUQuota == 0 && cfg.PidsLimit == 0) {
		return nil, nil
	}

	mgr, err := NewCgroupManager(cfg)
	if err != nil {
		return nil, err
	}

	if err := mgr.Setup(); err != nil {
		return nil, err
	}

	if err := mgr.Apply(pid); err != nil {
		mgr.Destroy()
		return nil, err
	}

	return mgr, nil
}

func ParseMemoryLimit(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	multiplier := int64(1)
	if strings.HasSuffix(s, "g") || strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "k") || strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = s[:len(s)-1]
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory limit: %s", s)
	}

	return val * multiplier, nil
}
