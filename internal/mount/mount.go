package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"litevm/internal/i18n"
	"litevm/internal/logger"
	"litevm/internal/usercheck"

	"github.com/hashicorp/go-multierror"
	"github.com/moby/sys/mountinfo"
)

// isArchLinux 检测 rootfs 是否是 Arch Linux
func isArchLinux(rootfsPath string) bool {
	// 检查 /etc/os-release 文件
	osReleasePath := filepath.Join(rootfsPath, "etc", "os-release")
	data, err := os.ReadFile(osReleasePath)
	if err == nil {
		content := string(data)
		if strings.Contains(content, "ID=arch") ||
			strings.Contains(content, "NAME=Arch") ||
			strings.Contains(content, "NAME=EndeavourOS") ||
			strings.Contains(content, "NAME=Manjaro") {
			return true
		}
	}
	// 检查 /etc/arch-release 文件
	_, err = os.Stat(filepath.Join(rootfsPath, "etc", "arch-release"))
	return err == nil
}

// 定义需要挂载的虚拟文件系统
var mountPoints = []struct {
	source   string
	target   string
	fstype   string
	flags    uintptr
	data     string
	required bool // 标识这是必需挂载点
}{
	// 虚拟文件系统 - 必需挂载点
	{"proc", "proc", "proc", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV, "", true},
	{"sysfs", "sys", "sysfs", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV, "", true},
	{"/dev", "dev", "", syscall.MS_BIND | syscall.MS_REC, "", true},
	{"/dev/pts", "dev/pts", "", syscall.MS_BIND, "", true},
	{"/dev/shm", "dev/shm", "", syscall.MS_BIND, "", true},

	// 运行时临时目录 - 必需挂载点
	{"/run", "run", "", syscall.MS_BIND, "", true},
	{"/tmp", "tmp", "", syscall.MS_BIND, "", true},

	// 可选的挂载点
	{"/var/run", "var/run", "", syscall.MS_BIND, "", false},
	{"/var/tmp", "var/tmp", "", syscall.MS_BIND, "", false},
	{"/var/log", "var/log", "", syscall.MS_BIND, "", false},

	// 网络解析 - 改为由 cleanup 模块在主机端生成，避免泄漏宿主机条目
	// {"/etc/hosts", "etc/hosts", "", syscall.MS_BIND, "", false},
	// {"/etc/resolv.conf", "etc/resolv.conf", "", syscall.MS_BIND, "", false},
	// {"/etc/nsswitch.conf", "etc/nsswitch.conf", "", syscall.MS_BIND, "", false},

	// 系统配置 - 可选
	{"/etc/mtab", "etc/mtab", "", syscall.MS_BIND, "", false},
}

// isMounted 检查路径是否已挂载
func isMounted(path string) bool {
	mounts, err := mountinfo.GetMounts(nil)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, m := range mounts {
		if m.Mountpoint == absPath {
			return true
		}
	}
	return false
}

// MountAll 挂载所有虚拟文件系统
func MountAll(rootfsPath string) ([]string, error) {
	var mounted []string

	// 准备基础目录结构
	if err := prepareDirs(rootfsPath); err != nil {
		logger.Warn(i18n.Tf("mount.prepare_dirs_fail", err))
	}

	archRootfs := isArchLinux(rootfsPath)

	for _, mp := range mountPoints {
		target := filepath.Join(rootfsPath, mp.target)

		// Arch Linux 跳过 /etc/mtab bind mount，用静态文件替代
		if archRootfs && mp.target == "etc/mtab" {
			logger.Debug("Arch Linux: 跳过 /etc/mtab bind mount")
			continue
		}

		// 检查是否已挂载
		if isMounted(target) {
			logger.Debug(i18n.Tf("mount.already_mounted", target))
			mounted = append(mounted, target)
			continue
		}

		// 对于必需挂载点，我们要确保目标目录存在
		if mp.required {
			if err := os.MkdirAll(target, 0755); err != nil {
				logger.Warn(i18n.Tf("mount.create_dir_fail", target, err))
			}
		} else {
			// 对于可选挂载点，检查源文件或目录是否存在
			if mp.fstype == "" { // 绑定挂载
				if _, err := os.Stat(mp.source); os.IsNotExist(err) {
					logger.Debug(i18n.Tf("mount.skip_source", mp.source))
					continue
				}
			}

			// 确保目标目录或文件的父目录存在
			targetParent := filepath.Dir(target)
			if err := os.MkdirAll(targetParent, 0755); err != nil {
				logger.Warn("创建父目录失败 %s: %v", targetParent, err)
				continue
			}

			// 如果是文件，确保目标文件存在或创建它
			sourceInfo, err := os.Stat(mp.source)
			if err == nil {
				if sourceInfo.Mode().IsRegular() {
					if _, err := os.Stat(target); os.IsNotExist(err) {
					if err := os.WriteFile(target, []byte{}, 0644); err != nil {
						logger.Warn(i18n.Tf("mount.create_target_fail", target, err))
							continue
						}
					}
				} else {
					// 目录，确保存在
					if err := os.MkdirAll(target, 0755); err != nil {
						logger.Warn("创建目录失败 %s: %v", target, err)
						continue
					}
				}
			}
		}

		// 尝试挂载
		logger.Debug(i18n.Tf("mount.mounting", mp.source, target))
		if err := syscall.Mount(mp.source, target, mp.fstype, mp.flags, mp.data); err != nil {
			if mp.required {
				logger.Warn(i18n.Tf("mount.required_fallback", target, err))
				// 对于 /dev、/dev/pts、/dev/shm、/tmp、/run，尝试使用 tmpfs
				if mp.fstype == "" && (mp.target == "dev" || mp.target == "dev/pts" || mp.target == "dev/shm" ||
					mp.target == "tmp" || mp.target == "run") {
					// 尝试用 tmpfs 创建一个临时文件系统
					var tmpfsData string
					switch mp.target {
					case "dev/pts":
						tmpfsData = "mode=620,gid=5"
					case "dev/shm", "tmp":
						tmpfsData = "mode=1777"
					default:
						tmpfsData = "mode=755"
					}

					if err := syscall.Mount("tmpfs", target, "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, tmpfsData); err == nil {
						mounted = append(mounted, target)
						logger.Debug(i18n.Tf("mount.tmpfs_fallback_ok", target))

						// 如果是 /dev，我们还需要创建一些基本的设备文件
						if mp.target == "dev" {
							if err := createDevFiles(target); err != nil {
								logger.Warn(i18n.Tf("mount.devfiles_fail", err))
							}
						}
						continue
					}
				}
			}
			logger.Warn(i18n.Tf("mount.mount_fail", target, err))
		} else {
			mounted = append(mounted, target)
			logger.Debug("挂载成功: %s", target)
		}
	}

	// 处理 /dev/ptmx 符号链接
	ptmxPath := filepath.Join(rootfsPath, "dev/ptmx")
	if _, err := os.Lstat(ptmxPath); os.IsNotExist(err) {
		if err := os.Symlink("pts/ptmx", ptmxPath); err != nil {
			logger.Warn("创建 ptmx 符号链接失败: %v", err)
		}
	}

	// Arch Linux 特殊处理
	if isArchLinux(rootfsPath) {
		logger.Info(i18n.T("mount.alarch_detected"))

		// 确保 pacman 缓存目录存在
		pacmanCache := filepath.Join(rootfsPath, "var/cache/pacman/pkg")
		if err := os.MkdirAll(pacmanCache, 0755); err != nil {
			logger.Warn("创建 pacman 缓存目录失败: %v", err)
		} else {
			// 确保权限正确
			if err := os.Chmod(pacmanCache, 0755); err != nil {
				logger.Debug("修改缓存目录权限失败: %v", err)
			}
			logger.Debug("已创建 pacman 缓存目录: %s", pacmanCache)
		}

		// 确保 pacman 数据库目录存在
		pacmanDB := filepath.Join(rootfsPath, "var/lib/pacman")
		if err := os.MkdirAll(filepath.Join(pacmanDB, "sync"), 0755); err != nil {
			logger.Warn("创建 pacman sync 目录失败: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(pacmanDB, "local"), 0755); err != nil {
			logger.Warn("创建 pacman local 目录失败: %v", err)
		}

		// 创建 ALPM 数据库版本标记（如果不存在）
		alpmVersion := filepath.Join(pacmanDB, "local/ALPM_DB_VERSION")
		if _, err := os.Stat(alpmVersion); os.IsNotExist(err) {
			if err := os.WriteFile(alpmVersion, []byte("9\n"), 0644); err != nil {
				logger.Debug("创建 ALPM_DB_VERSION 失败: %v", err)
			}
		}
	}

	return mounted, nil
}

// MountAllProot 为 proot 模式挂载虚拟文件系统
func MountAllProot(rootfsPath string) ([]string, error) {
	var mounted []string

	if err := prepareDirs(rootfsPath); err != nil {
		logger.Warn(i18n.Tf("mount.prepare_dirs_fail", err))
	}

	rootfsDev := filepath.Join(rootfsPath, "dev")
	rootfsDevPts := filepath.Join(rootfsPath, "dev/pts")
	rootfsDevShm := filepath.Join(rootfsPath, "dev/shm")
	rootfsProc := filepath.Join(rootfsPath, "proc")
	rootfsSys := filepath.Join(rootfsPath, "sys")
	rootfsTmp := filepath.Join(rootfsPath, "tmp")
	rootfsRun := filepath.Join(rootfsPath, "run")
	rootfsVarRun := filepath.Join(rootfsPath, "var/run")
	rootfsVarTmp := filepath.Join(rootfsPath, "var/tmp")
	rootfsVarLog := filepath.Join(rootfsPath, "var/log")

	os.MkdirAll(rootfsDev, 0755)
	os.MkdirAll(rootfsDevPts, 0755)
	os.MkdirAll(rootfsDevShm, 01777)
	os.MkdirAll(rootfsProc, 0555)
	os.MkdirAll(rootfsSys, 0555)
	os.MkdirAll(rootfsTmp, 01777)
	os.MkdirAll(rootfsRun, 0755)
	os.MkdirAll(rootfsVarRun, 0755)
	os.MkdirAll(rootfsVarTmp, 01777)
	os.MkdirAll(rootfsVarLog, 0755)

	prootMounts := []struct {
		source string
		target string
		fstype string
		flags  uintptr
		data   string
	}{
		{"proc", rootfsProc, "proc", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV, ""},
		{"sysfs", rootfsSys, "sysfs", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV, ""},
		{"tmpfs", rootfsDev, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=755,gid=0,uid=0"},
		{"devpts", rootfsDevPts, "devpts", syscall.MS_NOSUID | syscall.MS_NOEXEC, "newinstance,ptmxmode=0666,mode=620"},
		{"shm", rootfsDevShm, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC, "mode=1777,gid=0,uid=0"},
		{"tmpfs", rootfsTmp, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=1777,gid=0,uid=0"},
		{"tmpfs", rootfsRun, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=755,gid=0,uid=0"},
		{"tmpfs", rootfsVarRun, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=755,gid=0,uid=0"},
		{"tmpfs", rootfsVarTmp, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=1777,gid=0,uid=0"},
		{"tmpfs", rootfsVarLog, "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=755,gid=0,uid=0"},
	}

	for _, mp := range prootMounts {
		target := mp.target
		if isMounted(target) {
			logger.Debug(i18n.Tf("mount.already_mounted", target))
			mounted = append(mounted, target)
			continue
		}

		logger.Debug(i18n.Tf("mount.mounting", mp.source, target))
		if err := syscall.Mount(mp.source, target, mp.fstype, mp.flags, mp.data); err != nil {
			logger.Warn(i18n.Tf("mount.mount_fail", target, err))
		} else {
			mounted = append(mounted, target)
			logger.Debug("挂载成功: %s", target)
		}
	}

	bindMounts := []struct {
		source string
		target string
	}{
		{"/etc/hosts", filepath.Join(rootfsPath, "etc/hosts")},
		{"/etc/resolv.conf", filepath.Join(rootfsPath, "etc/resolv.conf")},
		{"/etc/passwd", filepath.Join(rootfsPath, "etc/passwd")},
		{"/etc/group", filepath.Join(rootfsPath, "etc/group")},
		{"/etc/nsswitch.conf", filepath.Join(rootfsPath, "etc/nsswitch.conf")},
		{"/etc/host.conf", filepath.Join(rootfsPath, "etc/host.conf")},
	}

	for _, bm := range bindMounts {
		if _, err := os.Stat(bm.source); err != nil {
			continue
		}
		targetDir := filepath.Dir(bm.target)
		os.MkdirAll(targetDir, 0755)

		if isMounted(bm.target) {
			continue
		}

		logger.Debug("绑定挂载: %s -> %s", bm.source, bm.target)
		if err := syscall.Mount(bm.source, bm.target, "", syscall.MS_BIND, ""); err != nil {
			logger.Warn("绑定挂载失败 %s: %v", bm.target, err)
		} else {
			mounted = append(mounted, bm.target)
		}
	}

	if err := createDevFiles(rootfsDev); err != nil {
		logger.Warn(i18n.Tf("mount.devfiles_fail", err))
	}

	ptmxPath := filepath.Join(rootfsDev, "ptmx")
	if _, err := os.Lstat(ptmxPath); os.IsNotExist(err) {
		if err := os.Symlink("pts/ptmx", ptmxPath); err != nil {
			logger.Warn("创建 ptmx 符号链接失败: %v", err)
		}
		os.Chmod(ptmxPath, 0666)
	}

	nullPath := filepath.Join(rootfsDev, "null")
	if _, err := os.Stat(nullPath); os.IsNotExist(err) {
		devNull, _ := os.OpenFile(nullPath, os.O_WRONLY|os.O_CREATE, 0666)
		if devNull != nil {
			devNull.Close()
		}
	}

	return mounted, nil
}

func createDevFiles(devPath string) error {
	devices := map[string]struct {
		mode os.FileMode
	}{
		"null":    {0666},
		"zero":    {0666},
		"random":  {0664},
		"urandom": {0664},
		"console": {0600},
		"tty":     {0666},
		"full":    {0666},
	}

	for dev, props := range devices {
		path := filepath.Join(devPath, dev)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, props.mode)
			if err == nil {
				f.Close()
			}
		}
	}

	return nil
}

// prepareDirs 准备必要的目录结构
func prepareDirs(rootfsPath string) error {
	dirs := []string{
		"proc", "sys", "dev", "dev/pts", "dev/shm",
		"tmp", "run", "root", "home",
		"var/run", "var/tmp", "var/log", "var/lib",
		"var/lib/apt/lists/partial",
	}
	for _, d := range dirs {
		path := filepath.Join(rootfsPath, d)
		if err := os.MkdirAll(path, 0755); err != nil {
			logger.Debug("创建目录 %s: %v", path, err)
		}
	}
	return nil
}

// UnmountAll 卸载所有已挂载的文件系统
func UnmountAll(mounted []string) error {
	var lastErr error

	// 构建已知挂载点集合，用于快速查找
	mountSet := make(map[string]bool, len(mounted))
	for _, p := range mounted {
		mountSet[p] = true
	}

	// 获取当前命名空间中的实际挂载点列表
	actualMounts, err := mountinfo.GetMounts(nil)
	if err != nil {
		logger.Debug("获取挂载列表失败: %v", err)
	}

	// 合并挂载列表：优先使用 mounted 列表中的路径，同时添加在命名空间中发现的 rootfs 相关挂载
	var allPaths []string
	seen := make(map[string]bool)
	for _, p := range mounted {
		if !seen[p] {
			allPaths = append(allPaths, p)
			seen[p] = true
		}
	}
	for _, m := range actualMounts {
		if !seen[m.Mountpoint] && mountSet[m.Mountpoint] {
			allPaths = append(allPaths, m.Mountpoint)
			seen[m.Mountpoint] = true
		}
	}

	// 反向卸载（从最深层开始）
	for i := len(allPaths) - 1; i >= 0; i-- {
		path := allPaths[i]
		if !isMounted(path) {
			logger.Debug(i18n.Tf("mount.unmount_skip", path))
			continue
		}
		logger.Debug(i18n.Tf("mount.unmounting", path))
		// 先尝试正常卸载
		if err := syscall.Unmount(path, 0); err != nil {
			logger.Debug("正常卸载失败，尝试 lazy unmount: %v", err)
			// 尝试 lazy unmount
			if err := syscall.Unmount(path, syscall.MNT_DETACH); err != nil {
				// EINVAL 通常意味着路径无法解析（如 chroot 恢复后路径不在当前 root 下）
				// 这种情况下挂载会在命名空间销毁时自动清理，不算严重错误
				if err == syscall.EINVAL {
					logger.Debug(i18n.Tf("mount.unmount_lazy_skip", path))
				} else {
					logger.Warn(i18n.Tf("mount.unmount_fail", path, err))
					lastErr = fmt.Errorf("%s", i18n.Tf("mount.unmount_fail", path, err))
				}
			}
		}
	}
	return lastErr
}

// ValidateRootfs 验证 rootfs 目录
func ValidateRootfs(rootfsPath string, customShell string) error {
	if _, err := os.Stat(rootfsPath); err != nil {
		return fmt.Errorf("%s", i18n.Tf("mount.rootfs_not_exist", err))
	}
	// 检查基本目录
	requiredDirs := []string{"bin", "etc"}
	for _, d := range requiredDirs {
		if _, err := os.Stat(filepath.Join(rootfsPath, d)); err != nil {
			return fmt.Errorf("%s", i18n.Tf("mount.missing_dir", d))
		}
	}
	// 检查 shell
	shellPath := "bin/sh"
	if customShell != "" {
		shellPath = customShell
		if shellPath[0] == '/' {
			shellPath = shellPath[1:]
		}
	}
	if _, err := os.Lstat(filepath.Join(rootfsPath, shellPath)); err != nil {
		return fmt.Errorf("%s", i18n.Tf("mount.missing_shell", shellPath))
	}
	return nil
}

// CleanupMounts 清理指定 rootfs 下的残留挂载点
func CleanupMounts(rootfsPath string) error {
	var result *multierror.Error
	absRoot, err := filepath.Abs(rootfsPath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %w", err)
	}

	// 尝试清理多次
	for attempt := 1; attempt <= 3; attempt++ {
		mounts, err := mountinfo.GetMounts(nil)
		if err != nil {
			result = multierror.Append(result, fmt.Errorf("获取挂载列表失败: %w", err))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var cleaned []string
		// 反向遍历卸载
		for i := len(mounts) - 1; i >= 0; i-- {
			m := mounts[i]
			if len(m.Mountpoint) > len(absRoot) && m.Mountpoint[:len(absRoot)+1] == absRoot+"/" {
				logger.Info("清理: %s", m.Mountpoint)
				if err := syscall.Unmount(m.Mountpoint, syscall.MNT_DETACH); err != nil {
					logger.Warn("卸载失败: %v", err)
					result = multierror.Append(result, err)
				} else {
					cleaned = append(cleaned, m.Mountpoint)
				}
			}
		}

		if len(cleaned) == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 最后检查
	finalMounts, _ := mountinfo.GetMounts(nil)
	var remaining []string
	for _, m := range finalMounts {
		if len(m.Mountpoint) > len(absRoot) && m.Mountpoint[:len(absRoot)+1] == absRoot+"/" {
			remaining = append(remaining, m.Mountpoint)
		}
	}
	if len(remaining) > 0 {
		logger.Warn("仍有未清理的挂载: %v", remaining)
		result = multierror.Append(result, fmt.Errorf("%s", i18n.Tf("mount.still_remaining", len(remaining))))
	} else {
		logger.Info(i18n.T("mount.cleanup_done"))
	}

	return result.ErrorOrNil()
}

// PrepareUserHome 准备用户主目录
func PrepareUserHome(rootfsPath string, userInfo *usercheck.UserInfo, isProot bool) {
	homePath := filepath.Join(rootfsPath, userInfo.Home[1:])
	if _, err := os.Stat(homePath); os.IsNotExist(err) {
		if err := os.MkdirAll(homePath, 0755); err != nil {
			logger.Debug("创建主目录失败: %v", err)
			return
		}
		logger.Debug("创建主目录: %s", homePath)
	}
}

// GetMountInfo 获取挂载信息（调试用）
func GetMountInfo() ([]*mountinfo.Info, error) {
	return mountinfo.GetMounts(nil)
}
