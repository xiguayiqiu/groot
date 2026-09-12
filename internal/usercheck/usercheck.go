package usercheck

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"groot/internal/i18n"
	"groot/internal/logger"
)

type UserInfo struct {
	Username string
	Uid      int
	Gid      int
	Home     string
	Shell    string
}

func CheckUser(rootfsPath string, username string) (*UserInfo, error) {
	if username == "" {
		username = "root"
	}

	passwdPath := filepath.Join(rootfsPath, "etc", "passwd")
	_, err := os.Stat(passwdPath)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.Tf("usercheck.no_passwd", err))
	}

	file, err := os.Open(passwdPath)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.Tf("usercheck.read_passwd_fail", err))
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}

		if parts[0] == username {
			uid, _ := strconv.Atoi(parts[2])
			gid, _ := strconv.Atoi(parts[3])
			home := parts[5]
			shell := parts[6]

			user := &UserInfo{
				Username: username,
				Uid:      uid,
				Gid:      gid,
				Home:     home,
				Shell:    shell,
			}

			checkAndWarnRootfsHealth(rootfsPath, user)

			return user, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s", i18n.Tf("usercheck.parse_passwd_fail", err))
	}

	return nil, fmt.Errorf("%s", i18n.Tf("usercheck.user_not_exist", username))
}

func CheckAndFixUser(rootfsPath string, username string) (*UserInfo, error) {
	user, err := CheckUser(rootfsPath, username)
	if err != nil {
		return nil, err
	}

	// 尝试自动修复常见问题
	FixCommonIssues(rootfsPath, user)

	return user, nil
}

// fixKeyDirs 修复关键目录权限（即使非 root 也尝试）
func fixKeyDirs(rootfsPath string) {
	// 修复需要写入的目录权限
	dirs := []string{
		"/var/log",
		"/var/cache",
		"/var/lib",
		"/var/lib/dpkg",
		"/var/lib/dpkg/info",
		"/var/lib/dpkg/alternatives",
		"/var/lib/dpkg/updates",
		"/var/lib/dpkg/triggers",
		"/var/lib/dpkg/tmp",
		"/var/lib/dpkg/tmp.ci",
		"/tmp",
		"/tmp/dpkg",
		"/root",
		"/run",
		"/run/lock",
		"/run/lock/lock",
	}

	for _, dir := range dirs {
		targetDir := filepath.Join(rootfsPath, dir[1:])
		if _, err := getFileStat(targetDir); err == nil {
			// 总是设置写权限，不管所有者是谁
			os.Chmod(targetDir, 0755)
		}
	}
}

func checkAndWarnRootfsHealth(rootfsPath string, userInfo *UserInfo) {
	checkSudoFiles(rootfsPath)
	checkHomeDir(rootfsPath, userInfo)
	checkShadowFile(rootfsPath)
	checkPAMFiles(rootfsPath)
	checkNSSFiles(rootfsPath)
}

func checkSudoFiles(rootfsPath string) {
	// 检查 /bin/sudo
	sudoBin := filepath.Join(rootfsPath, "bin", "sudo")
	if stat, err := getFileStat(sudoBin); err == nil {
		if stat.Mode&04000 == 0 {
			logger.Warn(i18n.T("usercheck.warn_sudo_nosuid"))
			logger.Warn(i18n.T("usercheck.warn_sudo_fix"))
		}
		if stat.Uid != 0 {
			logger.Warn("警告: /bin/sudo 所有者不是 root")
			logger.Warn("  请在 root 权限下修复: chown root:root /bin/sudo")
		}
	}

	// 检查 /usr/bin/sudo (用于 Alpine 等发行版)
	sudoBinUsr := filepath.Join(rootfsPath, "usr", "bin", "sudo")
	if stat, err := getFileStat(sudoBinUsr); err == nil {
		if stat.Mode&04000 == 0 {
			logger.Warn(i18n.T("usercheck.warn_sudo_nosuid"))
			logger.Warn(i18n.T("usercheck.warn_sudo_fix"))
		}
		if stat.Uid != 0 {
			logger.Warn("警告: /usr/bin/sudo 所有者不是 root")
			logger.Warn("  请在 root 权限下修复: chown root:root /usr/bin/sudo")
		}
	}

	sudoConf := filepath.Join(rootfsPath, "etc", "sudo.conf")
	if stat, err := getFileStat(sudoConf); err == nil {
		if stat.Uid != 0 {
			logger.Warn("警告: /etc/sudo.conf 所有者不是 root")
			logger.Warn("  请在 root 权限下修复: chown root:root /etc/sudo.conf")
		}
	}

	sudoers := filepath.Join(rootfsPath, "etc", "sudoers")
	if stat, err := getFileStat(sudoers); err == nil {
		if stat.Uid != 0 {
			logger.Warn("警告: /etc/sudoers 所有者不是 root")
			logger.Warn("  请在 root 权限下修复: chown root:root /etc/sudoers")
		}
		if stat.Mode&0022 != 0 {
			logger.Warn("警告: /etc/sudoers 权限过宽")
			logger.Warn("  请在 root 权限下修复: chmod 0440 /etc/sudoers")
		}
	}
}

func checkHomeDir(rootfsPath string, userInfo *UserInfo) {
	homeDir := filepath.Join(rootfsPath, userInfo.Home[1:])
	if _, err := os.Stat(homeDir); err != nil {
		logger.Warn("警告: 用户 %s 的主目录 %s 不存在", userInfo.Username, userInfo.Home)
		logger.Warn("  建议: mkdir -p %s", userInfo.Home)
	} else {
		if stat, err := getFileStat(homeDir); err == nil && stat.Uid != uint32(userInfo.Uid) {
			logger.Warn("警告: 用户 %s 的主目录 %s 所有权不正确", userInfo.Username, userInfo.Home)
			logger.Warn("  建议: chown -R %d:%d %s", userInfo.Uid, userInfo.Gid, userInfo.Home)
		}
	}
}

func checkShadowFile(rootfsPath string) {
	shadowPath := filepath.Join(rootfsPath, "etc", "shadow")
	if _, err := os.Stat(shadowPath); err != nil {
		logger.Warn("警告: /etc/shadow 文件不存在，su 可能受限")
		return
	}

	stat, err := getFileStat(shadowPath)
	if err != nil {
		return
	}

	if stat.Uid != 0 {
		logger.Warn("警告: /etc/shadow 所有者不是 root")
		logger.Warn("  建议: chown root:root /etc/shadow")
	}

	if stat.Mode&0077 != 0 {
		logger.Warn(i18n.T("usercheck.warn_shadow_wide"))
		logger.Warn(i18n.T("usercheck.warn_shadow_fix"))
	}
}

func checkPAMFiles(rootfsPath string) {
	pamDir := filepath.Join(rootfsPath, "etc", "pam.d")
	if _, err := os.Stat(pamDir); err != nil {
		logger.Warn("警告: /etc/pam.d 目录不存在，认证功能可能受限")
		return
	}
}

func checkNSSFiles(rootfsPath string) {
	nsswitchPath := filepath.Join(rootfsPath, "etc", "nsswitch.conf")
	if _, err := os.Stat(nsswitchPath); err != nil {
		logger.Warn(i18n.T("usercheck.warn_nsswitch"))
		logger.Warn(i18n.T("usercheck.warn_nsswitch_fix"))
	}
}

func FixCommonIssues(rootfsPath string, userInfo *UserInfo) {
	logger.Info(i18n.T("usercheck.start_fix"))
	logger.Info("目标目录: %s", rootfsPath)

	// 获取绝对路径
	absPath, err := filepath.Abs(rootfsPath)
	if err == nil {
		rootfsPath = absPath
	}

	// 修复关键目录权限（即使非 root 也尝试）
	fixKeyDirs(rootfsPath)

	// 仅在有真正 root 权限下做更深入的修复
	isRoot := os.Geteuid() == 0
	if !isRoot {
		logger.Warn("警告：非 root 权限运行，跳过深度权限修复！")
		logger.Warn("建议：使用 sudo ./groot 以获得完整功能")
		return
	}
	logger.Info("当前是真实 root 权限，开始深度修复...")

	// 先打印所有目标文件的原始状态！！！
	logger.Info("=== 修复前检查 ===")
	printFileStatus(rootfsPath + "/bin/sudo")
	printFileStatus(rootfsPath + "/usr/bin/sudo")
	printFileStatus(rootfsPath + "/bin/su")
	printFileStatus(rootfsPath + "/usr/bin/su")

	// 直接修复，每个都单独检查并日志！
	fixSingleFile(rootfsPath+"/bin/sudo", 04755, 0, 0)
	fixSingleFile(rootfsPath+"/usr/bin/sudo", 04755, 0, 0)
	fixSingleFile(rootfsPath+"/bin/su", 04755, 0, 0)
	fixSingleFile(rootfsPath+"/usr/bin/su", 04755, 0, 0)

	// 修复 sudoers 配置
	fixSingleFile(rootfsPath+"/etc/sudo.conf", 0644, 0, 0)
	fixSingleFile(rootfsPath+"/etc/sudoers", 0440, 0, 0)

	// 修复 /var/lib/sudo
	sudoLib := rootfsPath + "/var/lib/sudo"
	if _, err := os.Stat(sudoLib); err == nil {
		logger.Info("正在修复 %s", sudoLib)
		if err := os.Chown(sudoLib, 0, 0); err == nil {
			logger.Info("✓ %s 所有者已改为 root", sudoLib)
		}
		if err := os.Chmod(sudoLib, 0700); err == nil {
			logger.Info("✓ %s 权限已设置 0700", sudoLib)
		}
		// 递归修复子目录
		filepath.Walk(sudoLib, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			os.Chown(path, 0, 0)
			if info.IsDir() {
				os.Chmod(path, 0700)
			} else {
				os.Chmod(path, 0600)
			}
			return nil
		})
	}

	// 修复 shadow
	fixSingleFile(rootfsPath+"/etc/shadow", 0400, 0, 0)

	// 解锁 root 账户
	unlockRootAccount(rootfsPath)

	// 如果不是 root 用户，尝试添加到 sudoers
	if userInfo.Uid != 0 {
		sudoersFile := rootfsPath + "/etc/sudoers"
		sudoersContent, err := os.ReadFile(sudoersFile)
		if err == nil {
			sudoersStr := string(sudoersContent)
			userEntry := userInfo.Username + " ALL=(ALL) ALL"
			if !strings.Contains(sudoersStr, userEntry) {
				newContent := append(sudoersContent, []byte("\n"+userEntry+"\n")...)
				if err := os.WriteFile(sudoersFile, newContent, 0440); err == nil {
					logger.Info("✓ 用户 %s 已添加到 sudoers", userInfo.Username)
				}
			}
		}
	}

	// 再打印一次修复后的最终状态！！！
	logger.Info("=== 修复后检查 ===")
	printFileStatus(rootfsPath + "/bin/sudo")
	printFileStatus(rootfsPath + "/usr/bin/sudo")
	printFileStatus(rootfsPath + "/bin/su")
	printFileStatus(rootfsPath + "/usr/bin/su")

	logger.Info(i18n.T("usercheck.end_fix"))
}

func printFileStatus(path string) {
	if _, err := os.Stat(path); err != nil {
		logger.Debug("文件不存在: %s", path)
		return
	}
	stat, err := getFileStat(path)
	if err != nil {
		return
	}

	info, _ := os.Lstat(path)
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		realPath, _ := os.Readlink(path)
		logger.Warn("%s 是符号链接，指向: %s", path, realPath)
	}

	logger.Info("%s: UID=%d, GID=%d, MODE=%#o", path, stat.Uid, stat.Gid, stat.Mode)
}

func fixSingleFile(path string, mode os.FileMode, uid, gid int) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	logger.Info("正在修复: %s", path)

	// 先检查当前的状态
	currentStat, err := getFileStat(path)
	if err == nil {
		logger.Debug("  当前状态: UID=%d, GID=%d, MODE=%#o", currentStat.Uid, currentStat.Gid, currentStat.Mode)
	}

	// 执行修复
	chownOk := false
	if err := os.Chown(path, uid, gid); err == nil {
		logger.Info("✓ %s 所有者已改为 %d:%d", path, uid, gid)
		chownOk = true
	} else {
		logger.Warn("✗ 所有者修复失败: %v", err)
	}

	if err := os.Chmod(path, mode); err == nil {
		logger.Info("✓ %s 权限已设置 %#o", path, mode)
	} else {
		logger.Warn("✗ 权限设置失败: %v", err)
	}

	// 验证修复结果
	if chownOk {
		newStat, err := getFileStat(path)
		if err == nil {
			logger.Debug("  修复后状态: UID=%d, GID=%d, MODE=%#o", newStat.Uid, newStat.Gid, newStat.Mode)
			if newStat.Uid != uint32(uid) || newStat.Gid != uint32(gid) {
				logger.Warn("⚠ 修复没有生效！！！！！！！！！")
			} else {
				logger.Debug("✓ 修复已验证生效")
			}
		}
	}
}

func unlockRootAccount(rootfsPath string) {
	shadowPath := filepath.Join(rootfsPath, "etc", "shadow")
	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	modified := false

	for i, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) > 1 && parts[0] == "root" {
			// 直接设置密码为空字符串（最简单，直接解锁）
			parts[1] = ""
			lines[i] = strings.Join(parts, ":")
			modified = true
			break
		}
	}

	if modified {
		newContent := strings.Join(lines, "\n")
		if err := os.WriteFile(shadowPath, []byte(newContent), 0400); err == nil {
			logger.Info("已修复: root 账户已解锁，su - root 现在可以使用（无需密码）")
		}
	}
}

type fileStat struct {
	Uid  uint32
	Gid  uint32
	Mode os.FileMode
}

func getFileStat(path string) (*fileStat, error) {
	info, err := os.Stat(path) // 使用 Stat 而不是 Lstat，跟踪符号链接
	if err != nil {
		return nil, err
	}

	sys := info.Sys()
	if stat, ok := sys.(*syscall.Stat_t); ok {
		return &fileStat{
			Uid:  stat.Uid,
			Gid:  stat.Gid,
			Mode: info.Mode(),
		}, nil
	}

	return &fileStat{
		Uid:  0,
		Gid:  0,
		Mode: info.Mode(),
	}, nil
}
