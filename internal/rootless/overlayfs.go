package rootless

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"litevm/internal/logger"
)

const (
	FuseOverlayfsBinary = "fuse-overlayfs"
	OverlayUpperDir     = "upper"
	OverlayWorkDir      = "work"
	OverlayMergedDir    = "merged"
)

type OverlayConfig struct {
	LowerDirs []string
	UpperDir  string
	WorkDir   string
	MergedDir string
	StorageDir string
}

func IsFuseOverlayfsSupported() bool {
	path, err := exec.LookPath(FuseOverlayfsBinary)
	if err != nil {
		return false
	}

	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	logger.Debug("fuse-overlayfs version: %s", string(output))
	return true
}

func SetupOverlayfs(rootfsPath string, readonly bool) (*OverlayConfig, error) {
	if !IsFuseOverlayfsSupported() {
		return nil, fmt.Errorf("fuse-overlayfs is not installed or not supported")
	}

	storageDir := filepath.Join(rootfsPath, ".storage")
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage dir: %w", err)
	}

	upperDir := filepath.Join(storageDir, OverlayUpperDir)
	workDir := filepath.Join(storageDir, OverlayWorkDir)
	mergedDir := filepath.Join(storageDir, OverlayMergedDir)

	for _, dir := range []string{upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create overlay dir %s: %w", dir, err)
		}
	}

	config := &OverlayConfig{
		LowerDirs:  []string{rootfsPath},
		UpperDir:   upperDir,
		WorkDir:    workDir,
		MergedDir:  mergedDir,
		StorageDir: storageDir,
	}

	logger.Info("fuse-overlayfs storage: %s", storageDir)
	logger.Info("  upper: %s", upperDir)
	logger.Info("  work:  %s", workDir)
	logger.Info("  merged: %s", mergedDir)

	return config, nil
}

func MountOverlayfs(config *OverlayConfig, target string, readonly bool) error {
	if config == nil {
		return fmt.Errorf("overlay config is nil")
	}

	args := []string{}

	if readonly {
		for _, lower := range config.LowerDirs {
			args = append(args, "-o", "lowerdir="+lower)
		}
		args = append(args, config.MergedDir)
	} else {
		lowerdir := ""
		for i, l := range config.LowerDirs {
			if i > 0 {
				lowerdir += ":"
			}
			lowerdir += l
		}
		args = append(args, "-o",
			fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdir, config.UpperDir, config.WorkDir))
		args = append(args, config.MergedDir)
	}

	fuseBinary, err := exec.LookPath(FuseOverlayfsBinary)
	if err != nil {
		return fmt.Errorf("fuse-overlayfs binary not found: %w", err)
	}

	cmd := exec.Command(fuseBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fuse-overlayfs mount failed: %s: %w", string(output), err)
	}

	logger.Info("fuse-overlayfs mounted: %s -> %s", config.MergedDir, target)
	return nil
}

func CleanupOverlayfs(config *OverlayConfig) error {
	if config == nil {
		return nil
	}

	fuseBinary, err := exec.LookPath(FuseOverlayfsBinary)
	if err != nil {
		return nil
	}

	cmd := exec.Command(fuseBinary, "-u", config.MergedDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Debug("fuse-overlayfs unmount failed: %s: %v", string(output), err)
	}

	if config.StorageDir != "" {
		if err := os.RemoveAll(config.StorageDir); err != nil {
			logger.Debug("failed to remove overlay storage: %v", err)
		}
	}

	return nil
}

func OverlayfsMountInfo(config *OverlayConfig) string {
	if config == nil {
		return "overlay (not configured)"
	}
	lowerdir := ""
	for i, l := range config.LowerDirs {
		if i > 0 {
			lowerdir += ":"
		}
		lowerdir += l
	}
	return fmt.Sprintf("overlay lowerdir=%s,upperdir=%s,workdir=%s",
		lowerdir, config.UpperDir, config.WorkDir)
}
