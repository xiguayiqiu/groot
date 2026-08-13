package distro

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"groot/internal/logger"
	"groot/internal/termux"
)

type Distro struct {
	ID     string
	IDLike string
	Name   string
}

type Installer interface {
	Name() string
	CheckInstalled() (hasChroot bool, hasProot bool)
	Install() error
}

func DetectDistro() (*Distro, error) {
	// 首先尝试读取 /etc/os-release
	if osRelease, err := os.Open("/etc/os-release"); err == nil {
		defer osRelease.Close()
		scanner := bufio.NewScanner(osRelease)
		var id, idLike, name string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "ID=") {
				id = strings.Trim(line[3:], "\"'")
			} else if strings.HasPrefix(line, "ID_LIKE=") {
				idLike = strings.Trim(line[8:], "\"'")
			} else if strings.HasPrefix(line, "NAME=") {
				name = strings.Trim(line[5:], "\"'")
			}
		}
		if id != "" {
			return &Distro{
				ID:     id,
				IDLike: idLike,
				Name:   name,
			}, nil
		}
	}

	// 尝试 lsb_release (某些系统)
	if lsbPath, err := termux.SafeLookPath("lsb_release"); err == nil {
		if out, err := exec.Command(lsbPath, "-is").Output(); err == nil {
			id := strings.ToLower(strings.TrimSpace(string(out)))
			if id != "" {
				name, _ := exec.Command(lsbPath, "-ds").Output()
				return &Distro{
					ID:   id,
					Name: strings.TrimSpace(string(name)),
				}, nil
			}
		}
	}

	// 检查特定的文件
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return &Distro{ID: "arch", Name: "Arch Linux"}, nil
	}
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return &Distro{ID: "debian", Name: "Debian"}, nil
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return &Distro{ID: "rhel", Name: "Red Hat Enterprise Linux"}, nil
	}
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		return &Distro{ID: "alpine", Name: "Alpine Linux"}, nil
	}
	if _, err := os.Stat("/etc/SuSE-release"); err == nil {
		return &Distro{ID: "opensuse", Name: "openSUSE"}, nil
	}

	return nil, fmt.Errorf("无法检测发行版")
}

func GetInstaller(distro *Distro) (Installer, error) {
	switch {
	case distro.ID == "arch" || strings.Contains(distro.IDLike, "arch"):
		return NewArchInstaller(), nil
	case distro.ID == "debian" || distro.ID == "ubuntu" || strings.Contains(distro.IDLike, "debian"):
		return NewDebianInstaller(), nil
	case distro.ID == "rhel" || distro.ID == "centos" || distro.ID == "fedora" || strings.Contains(distro.IDLike, "rhel"):
		return NewRedHatInstaller(), nil
	case distro.ID == "alpine":
		return NewAlpineInstaller(), nil
	case distro.ID == "opensuse" || distro.ID == "suse" || strings.Contains(distro.IDLike, "suse"):
		return NewOpenSUSEInstaller(), nil
	case distro.ID == "freebsd":
		return NewFreeBSDInstaller(), nil
	case distro.ID == "netbsd":
		return NewNetBSDInstaller(), nil
	default:
		return nil, fmt.Errorf("不支持的发行版: %s", distro.ID)
	}
}

func InstallDependencies() error {
	logger.Info("正在检测发行版...")
	distro, err := DetectDistro()
	if err != nil {
		return err
	}
	logger.Info("检测到发行版: %s (%s)", distro.Name, distro.ID)

	installer, err := GetInstaller(distro)
	if err != nil {
		return err
	}

	hasChroot, hasProot := installer.CheckInstalled()
	if hasChroot && hasProot {
		logger.Info("所需的依赖已经全部安装：chroot 和 proot")
		return nil
	} else if hasChroot {
		logger.Info("chroot 已安装，proot 缺失")
	} else if hasProot {
		logger.Info("proot 已安装，chroot 缺失")
	}

	logger.Info("使用 %s 进行安装...", installer.Name())
	return installer.Install()
}

// 各个发行版的安装器实现

type ArchInstaller struct{}

func NewArchInstaller() *ArchInstaller { return &ArchInstaller{} }
func (a *ArchInstaller) Name() string   { return "pacman" }
func (a *ArchInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (a *ArchInstaller) Install() error {
	args := []string{"-S", "--noconfirm", "coreutils", "proot"}
	if os.Geteuid() != 0 {
		if sudo, err := termux.SafeLookPath("sudo"); err == nil {
			args = append([]string{sudo, "pacman"}, args...)
		} else {
			return fmt.Errorf("需要 root 权限，请使用 sudo")
		}
	} else {
		args = append([]string{"pacman"}, args...)
	}
	cmd, err := termux.CommandSlice(args)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type DebianInstaller struct{}

func NewDebianInstaller() *DebianInstaller { return &DebianInstaller{} }
func (d *DebianInstaller) Name() string    { return "apt" }
func (d *DebianInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (d *DebianInstaller) Install() error {
	// 先 update
	updateCmd := []string{"apt", "update", "-y"}
	if os.Geteuid() != 0 {
		if sudo, err := termux.SafeLookPath("sudo"); err == nil {
			updateCmd = append([]string{sudo}, updateCmd...)
		} else {
			return fmt.Errorf("需要 root 权限，请使用 sudo")
		}
	} else {
		// 保持原命令
	}
	cmd, err := termux.CommandSlice(updateCmd)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Warn("apt update 失败，继续安装: %v", err)
	}

	// 再 install
	args := []string{"apt", "install", "-y", "coreutils", "proot"}
	if os.Geteuid() != 0 {
		if sudo, err := termux.SafeLookPath("sudo"); err == nil {
			args = append([]string{sudo}, args...)
		} else {
			return fmt.Errorf("需要 root 权限，请使用 sudo")
		}
	}

	cmd, err = termux.CommandSlice(args)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type RedHatInstaller struct{}

func NewRedHatInstaller() *RedHatInstaller { return &RedHatInstaller{} }
func (r *RedHatInstaller) Name() string    { return "dnf/yum + 源码编译 proot" }
func (r *RedHatInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (r *RedHatInstaller) Install() error {
	pkgMan := "dnf"
	if _, err := termux.SafeLookPath("dnf"); err != nil {
		pkgMan = "yum"
	}

	// 检查是否是 Fedora 或 RHEL/CentOS
	if pkgMan == "dnf" {
		// Fedora: 尝试直接安装 proot
		logger.Info("Fedora 检测到，尝试直接安装 proot...")
		args := []string{"install", "-y", "coreutils", "proot"}
		if os.Geteuid() != 0 {
			if sudo, err := termux.SafeLookPath("sudo"); err == nil {
				args = append([]string{sudo, "dnf"}, args...)
			} else {
				return fmt.Errorf("需要 root 权限，请使用 sudo")
			}
		} else {
			args = append([]string{"dnf"}, args...)
		}
		cmd, err := termux.CommandSlice(args)
		if err != nil {
			return err
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err == nil {
			logger.Info("Fedora 上成功安装 proot")
			return nil
		}
		logger.Warn("Fedora 直接安装失败，回退到源码编译: %v", err)
	}

	// RHEL/CentOS: 需要源码编译 proot
	logger.Info("RHEL/CentOS 检测到，需要源码编译 proot")

	// 1. 安装开发依赖
	logger.Info("正在安装开发依赖...")
	devDeps := []string{"groupinstall", "-y", "Development Tools"}
	devDeps = append(devDeps, "install", "-y", "libarchive-devel", "talloc-devel", "uthash-devel", "git")

	if os.Geteuid() != 0 {
		if sudo, err := termux.SafeLookPath("sudo"); err == nil {
			devDeps = append([]string{sudo, pkgMan}, devDeps...)
		} else {
			return fmt.Errorf("需要 root 权限，请使用 sudo")
		}
	} else {
		devDeps = append([]string{pkgMan}, devDeps...)
	}
	cmd, err := termux.CommandSlice(devDeps)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// 2. clone proot
	logger.Info("正在克隆 proot 源码...")
	tempDir := filepath.Join(os.TempDir(), "proot-build")
	os.RemoveAll(tempDir)

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}
	gitPath, err := termux.SafeLookPath("git")
	if err != nil {
		return fmt.Errorf("git 未找到: %v", err)
	}
	cmd = exec.Command(gitPath, "clone", "https://github.com/proot-me/proot.git", tempDir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// 3. make
	logger.Info("正在编译 proot...")
	makePath, err := termux.SafeLookPath("make")
	if err != nil {
		return fmt.Errorf("make 未找到: %v", err)
	}
	cmd = exec.Command(makePath, "-C", tempDir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// 4. install
	logger.Info("正在安装 proot...")
	cmd = exec.Command(makePath, "-C", tempDir, "install")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	logger.Info("proot 成功安装!")
	return nil
}

type AlpineInstaller struct{}

func NewAlpineInstaller() *AlpineInstaller { return &AlpineInstaller{} }
func (a *AlpineInstaller) Name() string    { return "apk" }
func (a *AlpineInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (a *AlpineInstaller) Install() error {
	args := []string{"add", "--no-cache", "coreutils", "proot"}
	if os.Geteuid() != 0 {
		if sudo, err := termux.SafeLookPath("sudo"); err == nil {
			args = append([]string{sudo, "apk"}, args...)
		} else {
			return fmt.Errorf("需要 root 权限，请使用 sudo")
		}
	} else {
		args = append([]string{"apk"}, args...)
	}
	cmd, err := termux.CommandSlice(args)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type OpenSUSEInstaller struct{}

func NewOpenSUSEInstaller() *OpenSUSEInstaller { return &OpenSUSEInstaller{} }
func (s *OpenSUSEInstaller) Name() string     { return "zypper" }
func (s *OpenSUSEInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (s *OpenSUSEInstaller) Install() error {
	args := []string{"install", "-y", "coreutils", "proot"}
	if os.Geteuid() != 0 {
		if sudo, err := termux.SafeLookPath("sudo"); err == nil {
			args = append([]string{sudo, "zypper"}, args...)
		} else {
			return fmt.Errorf("需要 root 权限，请使用 sudo")
		}
	} else {
		args = append([]string{"zypper"}, args...)
	}
	cmd, err := termux.CommandSlice(args)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type FreeBSDInstaller struct{}

func NewFreeBSDInstaller() *FreeBSDInstaller { return &FreeBSDInstaller{} }
func (f *FreeBSDInstaller) Name() string     { return "pkg (chroot 已自带)" }
func (f *FreeBSDInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (f *FreeBSDInstaller) Install() error {
	logger.Info("FreeBSD: chroot 已默认安装")
	logger.Warn("FreeBSD: Linux 版 proot 无法在 FreeBSD 上运行")
	logger.Warn("FreeBSD: 可以使用原生的 chroot 或 jail 替代")
	return nil
}

type NetBSDInstaller struct{}

func NewNetBSDInstaller() *NetBSDInstaller { return &NetBSDInstaller{} }
func (n *NetBSDInstaller) Name() string     { return "pkgin (chroot 已自带)" }
func (n *NetBSDInstaller) CheckInstalled() (bool, bool) {
	_, err1 := termux.SafeLookPath("chroot")
	_, err2 := termux.SafeLookPath("proot")
	return err1 == nil, err2 == nil
}
func (n *NetBSDInstaller) Install() error {
	logger.Info("NetBSD: chroot 已默认安装")
	logger.Warn("NetBSD: Linux 版 proot 无法在 NetBSD 上运行")
	logger.Warn("NetBSD: 可以使用原生的 chroot 替代")
	return nil
}
