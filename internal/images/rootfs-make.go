package images

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"litevm/internal/i18n"
	"litevm/internal/permission"
	"litevm/internal/termux"
)

type BuildType string

const (
	BuildTypeMinimal  BuildType = "minimal"
	BuildTypeStandard BuildType = "standard"
	BuildTypeFull     BuildType = "full"
)

type BuildConfig struct {
	Distro  string
	Version string
	Arch    string
	DestDir string
	Mirror  string
	Type    BuildType
}

type MirrorSource string

const (
	MirrorTsinghua MirrorSource = "tsinghua"
	MirrorUSTC     MirrorSource = "ustc"
	MirrorOfficial MirrorSource = "official"
)

var mirrorMap = map[string]map[MirrorSource]string{
	"debian": {
		MirrorTsinghua: "https://mirrors.tuna.tsinghua.edu.cn/debian",
		MirrorUSTC:     "https://mirrors.ustc.edu.cn/debian",
		MirrorOfficial: "http://deb.debian.org/debian",
	},
	"ubuntu": {
		MirrorTsinghua: "https://mirrors.tuna.tsinghua.edu.cn/ubuntu",
		MirrorUSTC:     "https://mirrors.ustc.edu.cn/ubuntu",
		MirrorOfficial: "http://archive.ubuntu.com/ubuntu",
	},
	"arch": {
		MirrorTsinghua: "https://mirrors.tuna.tsinghua.edu.cn/archlinux",
		MirrorUSTC:     "https://mirrors.ustc.edu.cn/archlinux",
		MirrorOfficial: "https://geo.mirror.pkgbuild.com",
	},
}

func resolveMirror(distro, mirror string) string {
	if mirror == "" {
		mirror = string(MirrorOfficial)
	}

	source := MirrorSource(strings.ToLower(mirror))
	distroLower := strings.ToLower(distro)

	if distroMap, ok := mirrorMap[distroLower]; ok {
		if url, ok := distroMap[source]; ok {
			return url
		}
	}

	return mirror
}

func MakeRootfs(config BuildConfig) error {
	if !permission.IsRoot() {
		return fmt.Errorf("error: building rootfs requires root permission, please use sudo")
	}

	fmt.Printf("Starting build %s %s %s rootfs...\n", config.Distro, config.Version, config.Arch)

	currentDistro := detectDistro()
	fmt.Printf("Current system: %s\n", currentDistro)

	distroLower := strings.ToLower(config.Distro)

	supportedDistros := getSupportedDistros(currentDistro)
	if !isDistroSupported(distroLower, supportedDistros) {
		return fmt.Errorf("unsupported: cannot build %s on %s, supported distros: %s",
			config.Distro, currentDistro, strings.Join(supportedDistros, ", "))
	}

	if err := os.MkdirAll(config.DestDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	switch distroLower {
	case "debian", "ubuntu":
		return buildDebootstrap(config)
	case "arch":
		return buildArch(config)
	default:
		return fmt.Errorf("unsupported distro: %s", config.Distro)
	}
}

func getSupportedDistros(currentDistro string) []string {
	switch currentDistro {
	case "void":
		return []string{"debian", "ubuntu"}
	case "debian", "arch":
		return []string{"debian", "ubuntu", "arch"}
	default:
		return []string{"debian", "ubuntu", "arch"}
	}
}

func isDistroSupported(distro string, supportedList []string) bool {
	for _, d := range supportedList {
		if d == distro {
			return true
		}
	}
	return false
}

func buildDebootstrap(config BuildConfig) error {
	if err := checkTool("debootstrap"); err != nil {
		return err
	}

	distroConfigs := map[string]struct {
		versions   map[string]string
		mirrorType string
	}{
		"debian": {
			versions: map[string]string{
				"10":       "buster",
				"11":       "bullseye",
				"12":       "bookworm",
				"testing":  "testing",
				"unstable": "sid",
			},
			mirrorType: "debian",
		},
		"ubuntu": {
			versions: map[string]string{
				"20.04": "focal",
				"22.04": "jammy",
				"24.04": "noble",
			},
			mirrorType: "ubuntu",
		},
	}

	distroLower := strings.ToLower(config.Distro)
	distroConfig, ok := distroConfigs[distroLower]
	if !ok {
		return fmt.Errorf("unsupported distro: %s, supported debootstrap distros: debian, ubuntu", config.Distro)
	}

	var codename string
	if version, ok := distroConfig.versions[config.Version]; ok {
		codename = version
	} else {
		codename = config.Version
		fmt.Printf("Trying custom version: %s\n", codename)
	}

	mirror := resolveMirror(distroConfig.mirrorType, config.Mirror)

	targetDir := filepath.Join(config.DestDir, fmt.Sprintf("%s-%s-%s-%s", distroLower, config.Version, config.Arch, config.Type))

	fmt.Printf("Building %s %s (%s) [%s] to %s using debootstrap\n", config.Distro, codename, config.Arch, config.Type, targetDir)
	fmt.Printf("Mirror: %s\n", mirror)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	var variant string
	switch config.Type {
	case BuildTypeMinimal:
		variant = "minbase"
	case BuildTypeStandard:
		variant = "standard"
	case BuildTypeFull:
		variant = "buildd"
	default:
		variant = "standard"
	}

	args := []string{
		"--arch", config.Arch,
		"--variant", variant,
		codename,
		targetDir,
		mirror,
	}

	cmd, err := termux.Command("debootstrap", args...)
	if err != nil {
		return fmt.Errorf("%s", i18n.Tf("make.debootstrap_not_found", err))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("debootstrap failed: %w", err)
	}

	var extraPackages []string
	switch config.Type {
	case BuildTypeFull:
		extraPackages = []string{
			"build-essential",
			"git",
			"curl",
			"wget",
			"vim",
			"sudo",
			"openssh-client",
			"ca-certificates",
		}
	case BuildTypeStandard:
		extraPackages = []string{
			"curl",
			"wget",
			"vim",
			"ca-certificates",
		}
	}

	if len(extraPackages) > 0 {
		fmt.Printf("Installing extra packages: %v\n", extraPackages)
		installArgs := append([]string{
			"chroot", targetDir,
			"apt-get", "update", "-y",
			"&&",
			"apt-get", "install", "-y", "--no-install-recommends",
		}, extraPackages...)
		installCmd, err := termux.Command("bash", "-c", strings.Join(installArgs, " "))
		if err != nil {
			return fmt.Errorf("%s", i18n.Tf("make.bash_not_found", err))
		}
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		installCmd.Stdin = os.Stdin
		if err := installCmd.Run(); err != nil {
			fmt.Printf("Warning: failed to install extra packages: %v\n", err)
		}
	}

	fmt.Printf("%s rootfs build complete: %s\n", config.Distro, targetDir)
	return nil
}

func buildArch(config BuildConfig) error {
	if err := checkTool("pacstrap"); err != nil {
		return err
	}

	if config.Arch != "amd64" && config.Arch != "x86_64" {
		return fmt.Errorf("Arch Linux only supports amd64 architecture")
	}

	mirror := resolveMirror("arch", config.Mirror)
	targetDir := filepath.Join(config.DestDir, fmt.Sprintf("arch-%s-%s", config.Arch, config.Type))

	fmt.Printf("Building Arch Linux [%s] to %s using pacstrap\n", config.Type, targetDir)
	fmt.Printf("Mirror: %s\n", mirror)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	tempConfigDir := filepath.Join(os.TempDir(), fmt.Sprintf("litevm-pacman-%d", os.Getpid()))
	if err := os.MkdirAll(tempConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempConfigDir)

	pacmanConfPath := filepath.Join(tempConfigDir, "pacman.conf")
	pacmanConfContent := fmt.Sprintf(`[options]
HoldPkg     = pacman glibc
Architecture = auto
SigLevel    = Required DatabaseOptional
LocalFileSigLevel = Optional

[core]
Include = %s/mirrorlist

[extra]
Include = %s/mirrorlist

[community]
Include = %s/mirrorlist
`, tempConfigDir, tempConfigDir, tempConfigDir)

	if err := os.WriteFile(pacmanConfPath, []byte(pacmanConfContent), 0644); err != nil {
		return fmt.Errorf("failed to create temp pacman.conf: %w", err)
	}

	mirrorlistPath := filepath.Join(tempConfigDir, "mirrorlist")
	mirrorlistContent := fmt.Sprintf("Server = %s/$repo/os/$arch\n", mirror)
	if err := os.WriteFile(mirrorlistPath, []byte(mirrorlistContent), 0644); err != nil {
		return fmt.Errorf("failed to create temp mirrorlist: %w", err)
	}

	var packages []string
	switch config.Type {
	case BuildTypeMinimal:
		packages = []string{"base"}
	case BuildTypeStandard:
		packages = []string{"base", "base-devel", "curl", "wget", "vim", "ca-certificates"}
	case BuildTypeFull:
		packages = []string{"base", "base-devel", "git", "curl", "wget", "vim", "sudo", "openssh", "ca-certificates"}
	default:
		packages = []string{"base", "base-devel"}
	}

	args := append([]string{
		"-C", pacmanConfPath,
		targetDir,
	}, packages...)

	cmd, err := termux.Command("pacstrap", args...)
	if err != nil {
		return fmt.Errorf("%s", i18n.Tf("make.pacstrap_not_found", err))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pacstrap failed: %w", err)
	}

	fmt.Printf("Arch Linux rootfs build complete: %s\n", targetDir)
	return nil
}

func checkTool(tool string) error {
	_, err := termux.SafeLookPath(tool)
	if err == nil {
		return nil
	}

	return installTool(tool)
}

func detectDistro() string {
	fileExists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	if fileExists("/etc/void-release") {
		return "void"
	}
	if fileExists("/etc/arch-release") {
		return "arch"
	}
	if fileExists("/etc/debian_version") {
		return "debian"
	}
	if fileExists("/etc/os-release") {
		data, err := os.ReadFile("/etc/os-release")
		if err == nil {
			strData := string(data)
			if strings.Contains(strData, "ID=void") ||
				strings.Contains(strData, "ID=\"void\"") ||
				strings.Contains(strData, "Void Linux") {
				return "void"
			}
			if strings.Contains(strData, "ID=arch") ||
				strings.Contains(strData, "ID=\"arch\"") {
				return "arch"
			}
			if strings.Contains(strData, "ID=debian") ||
				strings.Contains(strData, "ID=\"debian\"") ||
				strings.Contains(strData, "ID=ubuntu") ||
				strings.Contains(strData, "ID=\"ubuntu\"") {
				return "debian"
			}
		}
	}
	if fileExists("/usr/bin/xbps-install") {
		return "void"
	}
	return "unknown"
}

func installTool(tool string) error {
	fmt.Printf("Detected missing %s, attempting to install...\n", tool)

	distro := detectDistro()
	fmt.Printf("Detected current system: %s\n", distro)

	var installFuncs []func(string) error

	switch distro {
	case "debian":
		installFuncs = []func(string) error{installWithApt, installWithPacman}
	case "arch":
		installFuncs = []func(string) error{installWithPacman, installWithApt}
	default:
		installFuncs = []func(string) error{installWithApt, installWithPacman}
	}

	for _, installFunc := range installFuncs {
		if err := installFunc(tool); err == nil {
			return nil
		}
		fmt.Println("Trying next package manager...")
	}

	return fmt.Errorf("cannot automatically install %s, please install manually and try again", tool)
}

func installWithApt(tool string) error {
	var pkgName string
	switch tool {
	case "debootstrap":
		pkgName = "debootstrap"
	case "pacstrap":
		pkgName = "arch-install-scripts"
	case "xbps-install":
		return fmt.Errorf("xbps-install is not available via apt")
	default:
		return fmt.Errorf("unsupported installation via apt for %s", tool)
	}

	fmt.Printf("Installing %s using apt...\n", pkgName)
	cmd, err := termux.Command("apt", "install", "-y", pkgName)
	if err != nil {
		return fmt.Errorf("%s", i18n.Tf("make.apt_not_found", err))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}

	if _, err := termux.SafeLookPath(tool); err != nil {
		return err
	}

	fmt.Printf("%s installed successfully!\n", tool)
	return nil
}

func installWithPacman(tool string) error {
	var pkgName string
	switch tool {
	case "debootstrap":
		pkgName = "debootstrap"
	case "pacstrap":
		pkgName = "arch-install-scripts"
	case "xbps-install":
		return fmt.Errorf("xbps-install is not available via pacman")
	default:
		return fmt.Errorf("unsupported installation via pacman for %s", tool)
	}

	fmt.Printf("Installing %s using pacman...\n", pkgName)
	cmd, err := termux.Command("pacman", "-S", "--noconfirm", pkgName)
	if err != nil {
		return fmt.Errorf("%s", i18n.Tf("make.pacman_not_found", err))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}

	if _, err := termux.SafeLookPath(tool); err != nil {
		return err
	}

	fmt.Printf("%s installed successfully!\n", tool)
	return nil
}

func InteractiveMakeRootfs(destDir string) error {
	if !permission.IsRoot() {
		return fmt.Errorf("error: building rootfs requires root permission, please use sudo")
	}

	if _, err := termux.SafeLookPath("whiptail"); err == nil {
		return interactiveMakeRootfsWhiptail(destDir)
	}
	// 如果 whiptail 没有找到，先尝试安装
	fmt.Println(i18n.T("make.whiptail_not_found"))
	distro := detectDistro()
	fmt.Printf("%s\n", i18n.Tf("make.detect_distro", distro))
	switch distro {
	case "alpine":
		if _, err := termux.SafeLookPath("apk"); err == nil {
			cmd, err := termux.Command("apk", "add", "--no-cache", "newt")
			if err != nil {
				break
			}
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				if _, err := termux.SafeLookPath("whiptail"); err == nil {
					return interactiveMakeRootfsWhiptail(destDir)
				}
			}
		}
	case "void":
		if _, err := termux.SafeLookPath("xbps-install"); err == nil {
			fmt.Println(i18n.T("make.xbps_installing"))
			cmd, err := termux.Command("xbps-install", "-Sy", "newt")
			if err != nil {
				break
			}
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if err := cmd.Run(); err != nil {
				fmt.Printf("%s\n", i18n.Tf("make.newt_install_fail", err))
			} else {
				fmt.Println(i18n.T("make.newt_installed_check"))
				if _, err := termux.SafeLookPath("whiptail"); err == nil {
					fmt.Println(i18n.T("make.whiptail_found"))
					return interactiveMakeRootfsWhiptail(destDir)
				} else {
					fmt.Printf("%s\n", i18n.Tf("make.whiptail_still_missing", err))
				}
			}
		} else {
			fmt.Printf("%s\n", i18n.Tf("make.xbps_not_found", err))
		}
	}
	// 如果安装失败或者其他发行版，回退到文本模式
	fmt.Println(i18n.T("make.text_mode"))
	return interactiveMakeRootfsSimple(destDir)
}

func interactiveMakeRootfsWhiptail(destDir string) error {
	currentDistro := detectDistro()
	supportedDistros := getSupportedDistros(currentDistro)

	distroOptions := []string{}
	for _, d := range supportedDistros {
		switch d {
		case "debian":
			distroOptions = append(distroOptions, "debian", "Debian")
		case "ubuntu":
			distroOptions = append(distroOptions, "ubuntu", "Ubuntu")
		case "arch":
			distroOptions = append(distroOptions, "arch", "Arch Linux")
		}
	}

	distro, err := runWhiptailMenu("Select Distro", "Please select distro to build:", distroOptions...)
	if err != nil || distro == "" {
		return fmt.Errorf("cancelled or invalid selection")
	}

	var version string
	switch distro {
	case "debian":
		versionOptions := []string{
			"12", "Debian 12 (bookworm)",
			"11", "Debian 11 (bullseye)",
			"10", "Debian 10 (buster)",
			"testing", "Debian Testing",
			"unstable", "Debian Unstable (sid)",
		}
		version, err = runWhiptailMenu("Select Debian Version", "Please select version:", versionOptions...)
	case "ubuntu":
		versionOptions := []string{
			"24.04", "Ubuntu 24.04 (noble)",
			"22.04", "Ubuntu 22.04 (jammy)",
			"20.04", "Ubuntu 20.04 (focal)",
		}
		version, err = runWhiptailMenu("Select Ubuntu Version", "Please select version:", versionOptions...)
	}

	if err != nil || version == "" {
		version = ""
	}

	archOptions := []string{
		"amd64", "amd64 (x86_64)",
		"aarch64", "aarch64 (ARM64)",
	}
	arch, err := runWhiptailMenu("Select Architecture", "Please select system architecture:", archOptions...)
	if err != nil || arch == "" {
		return fmt.Errorf("cancelled or invalid selection")
	}

	typeOptions := []string{
		"standard", "Standard (recommended)",
		"minimal", "Minimal",
		"full", "Full",
	}
	buildTypeStr, err := runWhiptailMenu("Select Build Type", "Please select build type:", typeOptions...)
	if err != nil || buildTypeStr == "" {
		return fmt.Errorf("cancelled or invalid selection")
	}
	var buildType BuildType
	switch buildTypeStr {
	case "minimal":
		buildType = BuildTypeMinimal
	case "full":
		buildType = BuildTypeFull
	default:
		buildType = BuildTypeStandard
	}

	var mirror string
	mirrorOptions := []string{
		"tsinghua", "Tsinghua University Mirror",
		"ustc", "USTC Mirror",
		"official", "Official Mirror",
	}
	if distro == "void" {
		mirrorOptions = []string{
			"tsinghua", "Tsinghua University Mirror",
			"ustc", "USTC Mirror",
		}
	}
	mirror, err = runWhiptailMenu("Select Mirror", "Please select download mirror:", mirrorOptions...)
	if err != nil || mirror == "" {
		return fmt.Errorf("cancelled or invalid selection")
	}

	promptText := fmt.Sprintf(`Please enter build directory path:

Current default path: %s
Relative paths are relative to current working directory
Path can be customized
Press Enter to use default path`, destDir)
	newDestDir, err := runWhiptailInputWithSize("Select Build Directory", promptText, destDir, "14", "70")
	if err != nil {
		return fmt.Errorf("cancelled or invalid selection")
	}
	if newDestDir != "" {
		destDir = newDestDir
	}

	confirmMsg := fmt.Sprintf(`Please confirm configuration:
Distro: %s
Version: %s
Architecture: %s
Type: %s
Mirror: %s
Build Directory: %s

Start building?`, distro, version, arch, buildType, mirror, destDir)

	whiptailPath, _ := termux.SafeLookPath("whiptail")
	cmd := exec.Command(whiptailPath, "--title", "Confirm Configuration", "--yesno", confirmMsg, "18", "60")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build cancelled")
	}

	config := BuildConfig{
		Distro:  distro,
		Version: version,
		Arch:    arch,
		DestDir: destDir,
		Mirror:  mirror,
		Type:    buildType,
	}

	return MakeRootfs(config)
}

func runWhiptailInput(title, prompt, defaultValue string) (string, error) {
	return runWhiptailInputWithSize(title, prompt, defaultValue, "12", "65")
}

func runWhiptailInputWithSize(title, prompt, defaultValue, height, width string) (string, error) {
	whiptailPath, err := termux.SafeLookPath("whiptail")
	if err != nil {
		return "", err
	}

	cmd := exec.Command(whiptailPath, "--title", title, "--inputbox", prompt, height, width, defaultValue)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	output, err := io.ReadAll(stderr)
	if err != nil {
		return "", err
	}

	if err := cmd.Wait(); err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(output)), nil
}

func interactiveMakeRootfsSimple(destDir string) error {
	fmt.Println("=== RootFS Build Tool ===")
	fmt.Println()

	currentDistro := detectDistro()
	supportedDistros := getSupportedDistros(currentDistro)

	fmt.Println("Please select distro:")
	distroMap := make(map[string]int)
	optionNum := 1
	for _, d := range supportedDistros {
		switch d {
		case "debian":
			fmt.Printf("%d. Debian\n", optionNum)
			distroMap["debian"] = optionNum
			optionNum++
		case "ubuntu":
			fmt.Printf("%d. Ubuntu\n", optionNum)
			distroMap["ubuntu"] = optionNum
			optionNum++
		case "arch":
			fmt.Printf("%d. Arch Linux\n", optionNum)
			distroMap["arch"] = optionNum
			optionNum++
		}
	}
	fmt.Println("0. Exit")

	var distroChoice string
	maxOption := optionNum - 1
	fmt.Printf("\nPlease select (0-%d): ", maxOption)
	fmt.Scanln(&distroChoice)

	var distro string
	for d, num := range distroMap {
		if distroChoice == fmt.Sprintf("%d", num) {
			distro = d
			break
		}
	}
	if distroChoice == "0" {
		return nil
	}
	if distro == "" {
		return fmt.Errorf("invalid selection")
	}

	var version string
	switch distro {
	case "debian":
		fmt.Println("\n=== Select Debian Version ===")
		fmt.Println("1. Debian 12 (bookworm)")
		fmt.Println("2. Debian 11 (bullseye)")
		fmt.Println("3. Debian 10 (buster)")
		fmt.Println("4. Debian Testing")
		fmt.Println("5. Debian Unstable (sid)")
		fmt.Print("Please select version (1-5): ")
		var v string
		fmt.Scanln(&v)
		if v == "1" {
			version = "12"
		} else if v == "2" {
			version = "11"
		} else if v == "3" {
			version = "10"
		} else if v == "4" {
			version = "testing"
		} else if v == "5" {
			version = "unstable"
		}
	case "ubuntu":
		fmt.Println("\n=== Select Ubuntu Version ===")
		fmt.Println("1. Ubuntu 24.04 (noble)")
		fmt.Println("2. Ubuntu 22.04 (jammy)")
		fmt.Println("3. Ubuntu 20.04 (focal)")
		fmt.Print("Please select version (1-3): ")
		var v string
		fmt.Scanln(&v)
		if v == "1" {
			version = "24.04"
		} else if v == "2" {
			version = "22.04"
		} else if v == "3" {
			version = "20.04"
		}
	}

	fmt.Println("\n=== Select Architecture ===")
	fmt.Println("1. amd64 (x86_64)")
	fmt.Println("2. aarch64 (ARM64)")
	fmt.Print("Please select architecture (1-2): ")

	var archChoice string
	fmt.Scanln(&archChoice)
	arch := "amd64"
	if archChoice == "2" {
		arch = "aarch64"
	}

	fmt.Println("\n=== Select Build Type ===")
	fmt.Println("1. Standard (recommended)")
	fmt.Println("2. Minimal")
	fmt.Println("3. Full")
	fmt.Print("Please select build type (1-3): ")

	var typeChoice string
	fmt.Scanln(&typeChoice)
	var buildType BuildType
	switch typeChoice {
	case "1":
		buildType = BuildTypeStandard
	case "2":
		buildType = BuildTypeMinimal
	case "3":
		buildType = BuildTypeFull
	default:
		buildType = BuildTypeStandard
	}

	var mirror string
	fmt.Println("\n=== Select Mirror ===")
	if distro == "void" {
		fmt.Println("1. Tsinghua University Mirror (tsinghua)")
		fmt.Println("2. USTC Mirror (ustc)")
		fmt.Print("Please select mirror (1-2): ")
		var mirrorChoice string
		fmt.Scanln(&mirrorChoice)
		mirror = "tsinghua"
		if mirrorChoice == "2" {
			mirror = "ustc"
		}
	} else {
		fmt.Println("1. Tsinghua University Mirror (tsinghua)")
		fmt.Println("2. USTC Mirror (ustc)")
		fmt.Println("3. Official Mirror (official)")
		fmt.Print("Please select mirror (1-3): ")
		var mirrorChoice string
		fmt.Scanln(&mirrorChoice)
		mirror = "official"
		if mirrorChoice == "1" {
			mirror = "tsinghua"
		} else if mirrorChoice == "2" {
			mirror = "ustc"
		}
	}

	fmt.Println("\n=== Select Build Directory ===")
	fmt.Printf("Current default path: %s\n", destDir)
	fmt.Println("Relative paths are relative to current working directory")
	fmt.Println("Path can be customized")
	fmt.Print("Please enter new directory path (press Enter to keep default): ")

	var newDestDir string
	fmt.Scanln(&newDestDir)
	if newDestDir != "" {
		destDir = newDestDir
	}

	fmt.Println("\n=== Please Confirm Configuration ===")
	fmt.Printf("Distro: %s\n", distro)
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Architecture: %s\n", arch)
	fmt.Printf("Type: %s\n", buildType)
	fmt.Printf("Mirror: %s\n", mirror)
	fmt.Printf("Build Directory: %s\n", destDir)
	fmt.Print("\nStart building? (y/n): ")

	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("Build cancelled")
		return nil
	}

	config := BuildConfig{
		Distro:  distro,
		Version: version,
		Arch:    arch,
		DestDir: destDir,
		Mirror:  mirror,
		Type:    buildType,
	}

	return MakeRootfs(config)
}
