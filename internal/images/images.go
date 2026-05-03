package images

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed images_update.jsonc
var embeddedConfig embed.FS

type ImagesConfig struct {
	Void   DistroConfig `json:"void"`
	Ubuntu DistroConfig `json:"ubuntu"`
	Alpine DistroConfig `json:"alpine"`
}

type DistroConfig struct {
	Base BaseConfig `json:"base"`
}

type BaseConfig map[string]interface{}

func LoadImagesConfig(configPath string) (*ImagesConfig, error) {
	var data []byte
	var err error

	// 首先尝试从指定的路径读取文件
	if configPath != "" {
		data, err = os.ReadFile(configPath)
		if err == nil {
			return parseConfig(data)
		}
		// 如果指定路径有问题，报错而不是回退
		return nil, err
	}

	// 如果没有指定路径或者文件不存在，使用嵌入的默认配置
	data, err = embeddedConfig.ReadFile("images_update.jsonc")
	if err != nil {
		return nil, fmt.Errorf("无法读取嵌入的配置文件: %w", err)
	}

	return parseConfig(data)
}

func parseConfig(data []byte) (*ImagesConfig, error) {
	data = removeComments(data)

	var cfg ImagesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func removeComments(data []byte) []byte {
	var result []byte
	inString := false
	inComment := false
	for i := 0; i < len(data); i++ {
		switch {
		case data[i] == '"' && !inComment:
			inString = !inString
			result = append(result, data[i])
		case data[i] == '/' && i+1 < len(data) && data[i+1] == '/' && !inString:
			inComment = true
			i++
		case data[i] == '\n' && inComment:
			inComment = false
			result = append(result, data[i])
		case !inComment:
			result = append(result, data[i])
		}
	}
	return result
}

func ListAvailableImages(cfg *ImagesConfig) error {
	fmt.Println("可用的镜像列表：")
	fmt.Println()

	fmt.Println("Void Linux:")
	voidBase := cfg.Void.Base
	for arch, libc := range voidBase {
		fmt.Printf("  架构: %s", arch)
		if libcMap, ok := libc.(map[string]interface{}); ok {
			fmt.Print(" (")
			first := true
			for libcName := range libcMap {
				if !first {
					fmt.Print(", ")
				}
				fmt.Print(libcName)
				first = false
			}
			fmt.Print(")")
		}
		fmt.Println()
	}
	fmt.Println()

	fmt.Println("Ubuntu:")
	ubuntuBase := cfg.Ubuntu.Base
	for version, arch := range ubuntuBase {
		fmt.Printf("  版本: %s", version)
		if archMap, ok := arch.(map[string]interface{}); ok {
			fmt.Print(" (")
			first := true
			for archName := range archMap {
				if !first {
					fmt.Print(", ")
				}
				fmt.Print(archName)
				first = false
			}
			fmt.Print(")")
		}
		fmt.Println()
	}
	fmt.Println()

	fmt.Println("Alpine:")
	alpineBase := cfg.Alpine.Base
	for arch := range alpineBase {
		fmt.Printf("  架构: %s\n", arch)
	}

	return nil
}

func ensureWgetInstalled() error {
	_, err := exec.LookPath("wget")
	if err == nil {
		return nil
	}

	fmt.Println("检测到系统未安装wget，正在安装...")

	var installCmd *exec.Cmd

	if _, err := exec.LookPath("apt"); err == nil {
		installCmd = exec.Command("apt", "install", "-y", "wget")
	} else if _, err := exec.LookPath("dnf"); err == nil {
		installCmd = exec.Command("dnf", "install", "-y", "wget")
	} else if _, err := exec.LookPath("yum"); err == nil {
		installCmd = exec.Command("yum", "install", "-y", "wget")
	} else if _, err := exec.LookPath("pacman"); err == nil {
		installCmd = exec.Command("pacman", "-S", "--noconfirm", "wget")
	} else if _, err := exec.LookPath("apk"); err == nil {
		installCmd = exec.Command("apk", "add", "--no-cache", "wget")
	} else {
		return fmt.Errorf("无法检测到系统包管理器，请手动安装wget")
	}

	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	installCmd.Stdin = os.Stdin
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("安装wget失败: %v", err)
	}

	fmt.Println("wget安装成功！")
	return nil
}

func DownloadImage(url, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	fileName := filepath.Base(url)
	destPath := filepath.Join(destDir, fileName)

	if err := ensureWgetInstalled(); err != nil {
		fmt.Println("无法使用wget，将使用默认下载方式...")

		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("下载失败: %s", resp.Status)
		}

		fmt.Printf("正在下载: %s -> %s\n", url, destPath)

		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return err
		}

		fmt.Printf("下载完成: %s\n", destPath)
		return nil
	}

	fmt.Printf("正在下载: %s -> %s\n", url, destPath)

	wgetCmd := exec.Command("wget", "-O", destPath, url)
	wgetCmd.Stdout = os.Stdout
	wgetCmd.Stderr = os.Stderr
	wgetCmd.Stdin = os.Stdin

	if err := wgetCmd.Run(); err != nil {
		return fmt.Errorf("wget下载失败: %v", err)
	}

	fmt.Printf("下载完成: %s\n", destPath)
	return nil
}

func DownloadAllImages(cfg *ImagesConfig, destDir string) error {
	fmt.Println("开始下载所有镜像...")

	fmt.Println("\n下载 Void Linux 镜像:")
	voidBase := cfg.Void.Base
	for _, libc := range voidBase {
		if libcMap, ok := libc.(map[string]interface{}); ok {
			for _, url := range libcMap {
				if urlStr, ok := url.(string); ok {
					if err := DownloadImage(urlStr, filepath.Join(destDir, "void")); err != nil {
						return err
					}
				}
			}
		}
	}

	fmt.Println("\n下载 Ubuntu 镜像:")
	ubuntuBase := cfg.Ubuntu.Base
	for _, arch := range ubuntuBase {
		if archMap, ok := arch.(map[string]interface{}); ok {
			for _, url := range archMap {
				if urlStr, ok := url.(string); ok {
					if err := DownloadImage(urlStr, filepath.Join(destDir, "ubuntu")); err != nil {
						return err
					}
				}
			}
		}
	}

	fmt.Println("\n下载 Alpine 镜像:")
	alpineBase := cfg.Alpine.Base
	for _, url := range alpineBase {
		if urlStr, ok := url.(string); ok {
			if err := DownloadImage(urlStr, filepath.Join(destDir, "alpine")); err != nil {
				return err
			}
		}
	}

	fmt.Println("\n所有镜像下载完成！")
	return nil
}

func DownloadBySelection(cfg *ImagesConfig, destDir, distro, selection string) error {
	return fmt.Errorf("此函数已废弃，请使用交互式下载或直接指定完整的下载路径")
}

func runWhiptailMenu(title, prompt string, items ...string) (string, error) {
	whiptailPath, err := exec.LookPath("whiptail")
	if err != nil {
		return "", err
	}

	// 构建基本命令
	cmd := exec.Command(whiptailPath, "--title", title, "--menu", prompt, "15", "60", "6")
	cmd.Args = append(cmd.Args, items...)

	// 设置标准输入和输出
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// 创建管道读取stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// 读取输出
	output, err := io.ReadAll(stderr)
	if err != nil {
		return "", err
	}

	// 等待命令完成
	if err := cmd.Wait(); err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(output)), nil
}

func InteractiveDownload(cfg *ImagesConfig, destDir string) error {
	_, err := exec.LookPath("whiptail")
	if err != nil {
		return InteractiveDownloadSimple(cfg, destDir)
	}

	for {
		distro, err := runWhiptailMenu("发行版", "请选择:",
			"void", "",
			"ubuntu", "",
			"alpine", "",
			"all", "",
		)
		if err != nil || distro == "" {
			fmt.Println("已退出")
			return nil
		}

		if distro == "all" {
			return DownloadAllImages(cfg, destDir)
		}

		switch distro {
		case "void":
			voidBase := cfg.Void.Base
			for {
				voidArchOptions := []string{}
				for arch := range voidBase {
					voidArchOptions = append(voidArchOptions, arch, "")
				}
				voidArchOptions = append(voidArchOptions, "back", " 返回")
				selectedArch, err := runWhiptailMenu("架构", "请选择:", voidArchOptions...)
				if err != nil || selectedArch == "" || selectedArch == "back" {
					break
				}

				if libcMap, ok := voidBase[selectedArch].(map[string]interface{}); ok {
					for {
						libcOptions := []string{}
						for libc := range libcMap {
							libcOptions = append(libcOptions, libc, "")
						}
						libcOptions = append(libcOptions, "back", "")
						selectedLibc, err := runWhiptailMenu("Libc类型", "请选择:", libcOptions...)
						if err != nil || selectedLibc == "" || selectedLibc == "back" {
							break
						}

						if url, ok := libcMap[selectedLibc].(string); ok {
							return DownloadImage(url, filepath.Join(destDir, "void"))
						}
					}
				}
			}

		case "ubuntu":
			ubuntuBase := cfg.Ubuntu.Base
			for {
				ubuntuVersionOptions := []string{}
				for version := range ubuntuBase {
					ubuntuVersionOptions = append(ubuntuVersionOptions, version, "")
				}
				ubuntuVersionOptions = append(ubuntuVersionOptions, "back", "")
				selectedVersion, err := runWhiptailMenu("版本", "请选择:", ubuntuVersionOptions...)
				if err != nil || selectedVersion == "" || selectedVersion == "back" {
					break
				}

				if archMap, ok := ubuntuBase[selectedVersion].(map[string]interface{}); ok {
					for {
						archOptions := []string{}
						for arch := range archMap {
							archOptions = append(archOptions, arch, "")
						}
						archOptions = append(archOptions, "back", "")
						selectedArch, err := runWhiptailMenu("架构", "请选择:", archOptions...)
						if err != nil || selectedArch == "" || selectedArch == "back" {
							break
						}

						if url, ok := archMap[selectedArch].(string); ok {
							return DownloadImage(url, filepath.Join(destDir, "ubuntu"))
						}
					}
				}
			}

		case "alpine":
			alpineBase := cfg.Alpine.Base
			for {
				alpineOptions := []string{}
				for arch := range alpineBase {
					alpineOptions = append(alpineOptions, arch, "")
				}
				alpineOptions = append(alpineOptions, "back", "")
				selectedArch, err := runWhiptailMenu("架构", "请选择:", alpineOptions...)
				if err != nil || selectedArch == "" || selectedArch == "back" {
					break
				}

				if url, ok := alpineBase[selectedArch].(string); ok {
					return DownloadImage(url, filepath.Join(destDir, "alpine"))
				}
			}
		}
	}
}

func InteractiveDownloadSimple(cfg *ImagesConfig, destDir string) error {
	// 简单的交互式界面
	fmt.Println("=== 镜像选择工具 ===")
	fmt.Println("1. Void Linux")
	fmt.Println("2. Ubuntu")
	fmt.Println("3. Alpine")
	fmt.Println("4. 下载所有镜像")
	fmt.Println("0. 退出")

	var choice string
	fmt.Print("\n请选择: ")
	fmt.Scanln(&choice)

	switch choice {
	case "1":
		// Void Linux
		fmt.Println("\n---  Void Linux  ---")
		voidBase := cfg.Void.Base
		fmt.Println("可用架构:")
		i := 1
		archList := []string{}
		for arch := range voidBase {
			fmt.Printf("%d. %s\n", i, arch)
			archList = append(archList, arch)
			i++
		}
		fmt.Println("0. 返回")

		var archChoice string
		fmt.Print("\n请选择: ")
		fmt.Scanln(&archChoice)

		if archChoice != "0" && archChoice >= "1" && archChoice <= fmt.Sprintf("%d", len(archList)) {
			idx := int(archChoice[0]-'0') - 1
			selectedArch := archList[idx]

			if libcMap, ok := voidBase[selectedArch].(map[string]interface{}); ok {
				fmt.Printf("\n架构 %s 的 libc 类型:\n", selectedArch)
				i = 1
				libcList := []string{}
				for libc := range libcMap {
					fmt.Printf("%d. %s\n", i, libc)
					libcList = append(libcList, libc)
					i++
				}
				fmt.Println("0. 返回")

				var libcChoice string
				fmt.Print("\n请选择: ")
				fmt.Scanln(&libcChoice)

				if libcChoice != "0" && libcChoice >= "1" && libcChoice <= fmt.Sprintf("%d", len(libcList)) {
					idx := int(libcChoice[0]-'0') - 1
					selectedLibc := libcList[idx]

					if url, ok := libcMap[selectedLibc].(string); ok {
						return DownloadImage(url, filepath.Join(destDir, "void"))
					}
				}
			}
		}

	case "2":
		// Ubuntu
		fmt.Println("\n--- Ubuntu ---")
		ubuntuBase := cfg.Ubuntu.Base
		fmt.Println("可用版本:")
		i := 1
		versionList := []string{}
		for version := range ubuntuBase {
			fmt.Printf("%d. %s\n", i, version)
			versionList = append(versionList, version)
			i++
		}
		fmt.Println("0. 返回")

		var versionChoice string
		fmt.Print("\n请选择: ")
		fmt.Scanln(&versionChoice)

		if versionChoice != "0" && versionChoice >= "1" && versionChoice <= fmt.Sprintf("%d", len(versionList)) {
			idx := int(versionChoice[0]-'0') - 1
			selectedVersion := versionList[idx]

			if archMap, ok := ubuntuBase[selectedVersion].(map[string]interface{}); ok {
				fmt.Printf("\n版本 %s 的架构:\n", selectedVersion)
				i = 1
				archList := []string{}
				for arch := range archMap {
					fmt.Printf("%d. %s\n", i, arch)
					archList = append(archList, arch)
					i++
				}
				fmt.Println("0. 返回")

				var archChoice string
				fmt.Print("\n请选择: ")
				fmt.Scanln(&archChoice)

				if archChoice != "0" && archChoice >= "1" && archChoice <= fmt.Sprintf("%d", len(archList)) {
					idx := int(archChoice[0]-'0') - 1
					selectedArch := archList[idx]

					if url, ok := archMap[selectedArch].(string); ok {
						return DownloadImage(url, filepath.Join(destDir, "ubuntu"))
					}
				}
			}
		}

	case "3":
		// Alpine
		fmt.Println("\n--- Alpine ---")
		alpineBase := cfg.Alpine.Base
		fmt.Println("可用架构:")
		i := 1
		archList := []string{}
		for arch := range alpineBase {
			fmt.Printf("%d. %s\n", i, arch)
			archList = append(archList, arch)
			i++
		}
		fmt.Println("0. 返回")

		var archChoice string
		fmt.Print("\n请选择: ")
		fmt.Scanln(&archChoice)

		if archChoice != "0" && archChoice >= "1" && archChoice <= fmt.Sprintf("%d", len(archList)) {
			idx := int(archChoice[0]-'0') - 1
			selectedArch := archList[idx]

			if url, ok := alpineBase[selectedArch].(string); ok {
				return DownloadImage(url, filepath.Join(destDir, "alpine"))
			}
		}

	case "4":
		// 下载所有镜像
		return DownloadAllImages(cfg, destDir)

	case "0":
		fmt.Println("已退出")
	}

	return nil
}
