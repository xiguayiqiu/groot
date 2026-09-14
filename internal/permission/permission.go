package permission

import (
	"os"
	"path/filepath"
	"strings"

	"litevm/internal/termux"
)

// IsRoot 检查是否是真实 root 权限
func IsRoot() bool {
	return os.Geteuid() == 0
}

// HasUnprivilegedUserns 检查是否支持非特权用户命名空间
func HasUnprivilegedUserns() bool {
	// 检查内核参数
	data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err == nil {
		return strings.TrimSpace(string(data)) == "1"
	}

	// 检查另一种可能的路径
	data, err = os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err == nil {
		val := strings.TrimSpace(string(data))
		return val != "0" && val != ""
	}

	return false
}

// FindSu 查找 su 可执行文件的位置（适配 Magisk 30+）
func FindSu() string {
	// 常见的 su 路径列表
	suPaths := []string{
		"/system/xbin/su",
		"/system/bin/su",
		"/sbin/su",
		"/sbin/su/su",
		"/data/adb/modules/su/bin/su",
		"/data/adb/magisk/su",
		"/data/adb/ksu/bin/su",
		"/data/local/su",
		"/data/local/bin/su",
		"/data/local/xbin/su",
		"/system/sd/xbin/su",
		"/system/xbin/magisk",
		"/system/bin/magisk",
	}

	// 也可以尝试 PATH 中的 su
	if pathSu, err := termux.SafeLookPath("su"); err == nil {
		return pathSu
	}

	// 遍历常见路径
	for _, path := range suPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// DetectDistro 检测发行版
func DetectDistro() string {
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return "arch"
	}

	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return "redhat"
	}

	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "debian"
	}

	if _, err := os.Stat("/etc/os-release"); err == nil {
		content, err := os.ReadFile("/etc/os-release")
		if err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "ID=") {
					id := strings.TrimPrefix(line, "ID=")
					id = strings.Trim(id, "\"")
					switch id {
					case "arch":
						return "arch"
					case "centos", "rhel", "fedora", "rocky", "alma":
						return "redhat"
					case "debian", "ubuntu", "linuxmint":
						return "debian"
					}
					return id
				}
			}
		}
	}

	return "unknown"
}

// GetExecutablePath 获取当前可执行文件的绝对路径
func GetExecutablePath() string {
	// 优先使用 /proc/self/exe，最可靠
	if exePath, err := os.Readlink("/proc/self/exe"); err == nil {
		return exePath
	}

	// 尝试 os.Args[0]
	if filepath.IsAbs(os.Args[0]) {
		if _, err := os.Stat(os.Args[0]); err == nil {
			return os.Args[0]
		}
	}

	// 尝试在 PATH 中查找
	if path, err := termux.SafeLookPath(os.Args[0]); err == nil {
		if absPath, err := filepath.Abs(path); err == nil {
			return absPath
		}
		return path
	}

	// 最后，返回 os.Args[0] 作为备用
	return os.Args[0]
}
