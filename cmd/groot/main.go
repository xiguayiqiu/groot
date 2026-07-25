package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"groot/internal/chroot"
	"groot/internal/images"
	"groot/internal/logger"
	"groot/internal/mount"
	"groot/internal/permission"
	"groot/internal/proot"
	"groot/internal/termux"

	"github.com/urfave/cli/v2"
)

const (
	version = "0.3"
)

func detectDistro() string {
	fileExists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	if fileExists("/etc/void-release") {
		return "void"
	}
	if fileExists("/etc/alpine-release") {
		return "alpine"
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
			if strings.Contains(strData, "ID=alpine") ||
				strings.Contains(strData, "ID=\"alpine\"") {
				return "alpine"
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

func main() {
	// 处理内部子命令和版本参数
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "chroot-child":
			if len(os.Args) < 3 {
				fmt.Fprintf(os.Stderr, "错误: chroot-child 子命令需要 rootfs 路径参数\n")
				os.Exit(1)
			}
			customShell := ""
			customUser := ""
			if len(os.Args) >= 4 {
				customShell = os.Args[3]
			}
			if len(os.Args) >= 5 {
				customUser = os.Args[4]
			}
			if err := chroot.ChildMain(os.Args[2], customShell, customUser); err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			return
		case "-v", "--version":
			fmt.Printf("groot version %s\n", version)
			fmt.Println("作者：弈秋忘忧白帽")
			fmt.Println("开源协议：MIT")
			return
		}
	}

	app := &cli.App{
		Name:        "groot",
		Usage:       "Go 版双模式隔离工具（chroot/proot）",
		Version:     version,
		Copyright:   "MIT License - Copyright © 2026 弈秋忘忧白帽",
		HideVersion: true,
		Commands: []*cli.Command{
			{
				Name:  "pm",
				Usage: "包管理器：管理 rootfs 镜像下载和构建",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "列出可用的 rootfs 镜像",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "config",
								Usage: "指定镜像配置文件路径（默认使用嵌入的配置）",
							},
						},
						Action: func(cCtx *cli.Context) error {
							configPath := cCtx.String("config")
							cfg, err := images.LoadImagesConfig(configPath)
							if err != nil {
								return fmt.Errorf("加载配置失败: %v", err)
							}

							return images.ListAvailableImages(cfg)
						},
					},
					{
						Name:  "download",
						Usage: "下载 rootfs 镜像",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "config",
								Usage: "指定镜像配置文件路径（默认使用嵌入的配置）",
							},
							&cli.StringFlag{
								Name:  "dest",
								Usage: "指定下载目录",
								Value: "downloads",
							},
							&cli.BoolFlag{
								Name:  "all",
								Usage: "下载所有镜像",
							},
							&cli.StringFlag{
								Name:  "distro",
								Usage: "指定发行版（void/ubuntu/alpine）",
							},
							&cli.StringFlag{
								Name:  "select",
								Usage: "选择要下载的镜像（架构/版本/库）",
							},
						},
						Action: func(cCtx *cli.Context) error {
							configPath := cCtx.String("config")

							destDir := cCtx.String("dest")
							if !filepath.IsAbs(destDir) {
								wd, err := os.Getwd()
								if err != nil {
									return err
								}
								destDir = filepath.Join(wd, destDir)
							}

							cfg, err := images.LoadImagesConfig(configPath)
							if err != nil {
								return fmt.Errorf("加载配置失败: %v", err)
							}

							if cCtx.Bool("all") {
								return images.DownloadAllImages(cfg, destDir)
							}

							// 直接使用交互式下载
							return images.InteractiveDownload(cfg, destDir)
						},
					},
					{
						Name:  "make",
						Usage: "构建 rootfs 镜像",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "distro",
								Usage: "指定发行版（debian/ubuntu/arch/alpine/void）",
							},
							&cli.StringFlag{
								Name:  "version",
								Usage: "指定发行版版本（如 12 对于 Debian, 22.04 对于 Ubuntu, v3.20 对于 Alpine, x86_64 对于 Void）",
							},
							&cli.StringFlag{
								Name:  "arch",
								Usage: "指定架构（amd64/aarch64）",
								Value: "amd64",
							},
							&cli.StringFlag{
								Name:  "type",
								Usage: "指定构建类型：minimal（精简版）、standard（标准版）、full（完整版）（可选，默认 standard）",
								Value: "standard",
							},
							&cli.StringFlag{
								Name:  "dest",
								Usage: "指定构建目录",
								Value: "rootfs",
							},
							&cli.StringFlag{
								Name:  "mirror",
								Usage: "指定镜像源：tsinghua（清华）、ustc（中科大）、official（官方）或自定义 URL（可选，默认 official）",
							},
						},
						Action: func(cCtx *cli.Context) error {
							destDir := cCtx.String("dest")
							if !filepath.IsAbs(destDir) {
								wd, err := os.Getwd()
								if err != nil {
									return err
								}
								destDir = filepath.Join(wd, destDir)
							}

							// 如果没有通过命令行参数提供完整信息，进入交互式模式
							distro := cCtx.String("distro")
							if distro == "" {
								return images.InteractiveMakeRootfs(destDir)
							}

							// 解析构建类型
							var buildType images.BuildType
							typeStr := cCtx.String("type")
							switch typeStr {
							case "minimal":
								buildType = images.BuildTypeMinimal
							case "full":
								buildType = images.BuildTypeFull
							default:
								buildType = images.BuildTypeStandard
							}

							config := images.BuildConfig{
								Distro:  distro,
								Version: cCtx.String("version"),
								Arch:    cCtx.String("arch"),
								DestDir: destDir,
								Mirror:  cCtx.String("mirror"),
								Type:    buildType,
							}

							return images.MakeRootfs(config)
						},
					},
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "c",
				Aliases: []string{"chroot"},
				Usage:   "chroot 模式：指定 rootfs 目录（需要 root 权限）",
			},
			&cli.StringFlag{
				Name:    "p",
				Aliases: []string{"proot"},
				Usage:   "proot 模式：指定 rootfs 目录（无需 root 权限）",
			},
			&cli.StringFlag{
				Name:  "z",
				Usage: "专属 proot 兼容模式：指定发行版（alpine/debian），然后指定 rootfs 目录",
			},
			&cli.StringFlag{
				Name:  "b",
				Usage: "指定容器目录的 shell 解释器路径（例如：/bin/ash、/bin/bash）",
			},
			&cli.StringFlag{
				Name:    "k",
				Aliases: []string{"cleanup"},
				Usage:   "清理指定 rootfs 下的残留挂载点",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "启用详细日志",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "启用调试日志",
			},
			&cli.BoolFlag{
				Name:  "l",
				Usage: "列出支持的发行版列表",
			},
			&cli.BoolFlag{
				Name:    "d",
				Aliases: []string{"download"},
				Usage:   "打开浏览器选择并下载 rootfs 镜像",
			},
		},
		Before: func(cCtx *cli.Context) error {
			if cCtx.Bool("debug") {
				logger.SetLevel(logger.LevelDebug)
			} else if cCtx.Bool("verbose") {
				logger.SetLevel(logger.LevelInfo)
			} else {
				// 非 verbose 模式只显示 WARN 及以上级别的日志
				logger.SetLevel(logger.LevelWarn)
			}
			return nil
		},
		Action: func(cCtx *cli.Context) error {
			if cCtx.Bool("v") || cCtx.Bool("version") {
				fmt.Printf("groot version %s\n", version)
				fmt.Println("MIT License - Copyright © 2026 弈秋忘忧白帽")
				return nil
			}

			chrootPath := cCtx.String("c")
			prootPath := cCtx.String("p")
			prootDistro := cCtx.String("z")
			cleanupPath := cCtx.String("k")
			customShell := cCtx.String("b")
			listDistros := cCtx.Bool("l")
			download := cCtx.Bool("d")
			args := cCtx.Args()

			// -d/--download 模式：TUI 选择发行版并打开浏览器
			if download {
				return runDownload()
			}

			// -c 和 -p 模式下，第一个位置参数（若 -b 未指定）作为自定义 shell
			if customShell == "" && (chrootPath != "" || prootPath != "") && args.Len() > 0 {
				customShell = args.First()
			}

			if cCtx.NArg() > 0 {
				if prootDistro == "" && chrootPath == "" && prootPath == "" && cleanupPath == "" && !listDistros {
					return cli.ShowAppHelp(cCtx)
				}
			}

			if listDistros {
				// 检查 whiptail 是否可用
				whiptailPath, err := exec.LookPath("whiptail")
				if err != nil {
					// 尝试安装 whiptail
					fmt.Println("提示: whiptail 没有找到，尝试安装它以获得更好的交互体验...")
					distro := detectDistro()
					fmt.Printf("检测到当前系统是: %s\n", distro)
					installed := false
					switch distro {
					case "alpine":
						if _, err := exec.LookPath("apk"); err == nil {
							cmd := exec.Command("apk", "add", "--no-cache", "newt")
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							if err := cmd.Run(); err == nil {
								installed = true
							}
						}
					case "void":
						if _, err := exec.LookPath("xbps-install"); err == nil {
							fmt.Println("找到 xbps-install，正在安装 newt 包...")
							cmd := exec.Command("xbps-install", "-Sy", "newt")
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							cmd.Stdin = os.Stdin
							if err := cmd.Run(); err == nil {
								installed = true
							} else {
								fmt.Printf("安装 newt 失败: %v\n", err)
							}
						} else {
							fmt.Printf("未找到 xbps-install: %v\n", err)
						}
					case "debian":
						if _, err := exec.LookPath("apt"); err == nil {
							fmt.Println("找到 apt，正在安装 newt 包...")
							cmd := exec.Command("apt", "install", "-y", "newt")
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							cmd.Stdin = os.Stdin
							if err := cmd.Run(); err == nil {
								installed = true
							}
						}
					case "arch":
						if _, err := exec.LookPath("pacman"); err == nil {
							fmt.Println("找到 pacman，正在安装 newt 包...")
							cmd := exec.Command("pacman", "-S", "--noconfirm", "newt")
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							cmd.Stdin = os.Stdin
							if err := cmd.Run(); err == nil {
								installed = true
							}
						}
					}

					if installed {
						whiptailPath, err = exec.LookPath("whiptail")
					}

					if err != nil {
						// 回退到文本显示
						fmt.Println("将使用文本交互模式")
						fmt.Println()
						fmt.Println("支持的发行版列表：")
						fmt.Println()
						fmt.Println("=== download 方式（pm download） ===")
						fmt.Println("| 发行版   | proot | chroot                |")
						fmt.Println("| :------- | :---- | :-------------------- |")
						fmt.Println("| Alpine   | ✓     | ✓                     |")
						fmt.Println("| Ubuntu   | -     | ✓                     |")
						fmt.Println("| Void     | -     | ✓                     |")
						fmt.Println()
						fmt.Println("=== make 方式（pm make） ===")
						fmt.Println("| 发行版   | proot | chroot                |")
						fmt.Println("| :------- | :---- | :-------------------- |")
						fmt.Println("| Debian   | ✓     | ✓                     |")
						fmt.Println("| Ubuntu   | -     | ✓                     |")
						fmt.Println("| Arch     | -     | ✓ (仅在 Arch 系统有效)|")
						fmt.Println()
						fmt.Println("说明：")
						fmt.Println("  ✓ 完美支持")
						fmt.Println("  - 仅 chroot 模式支持")
						fmt.Println()
						return nil
					}
				}

				// 使用 whiptail 显示
				msg := `支持的发行版列表
+------------------------------+
| download 方式（pm download） |
+------------------------------+
| 发行版   | proot | chroot    |
+--------- + ----- + ----------+
| Alpine   | ✓     | ✓         |
| Ubuntu   | -     | ✓         |
| Void     | -     | ✓         |
+------------------------------+
| make 方式（pm make）         |
+------------------------------+
| 发行版   | proot | chroot    |
+--------- + ----- + ----------+
| Debian   | ✓     | ✓         |
| Ubuntu   | -     | ✓         |
| Arch     | -     | ✓         |
+------------------------------+

说明：
  ✓ 完美支持
  - 仅 chroot 模式支持
  Arch 构建仅在 Arch Linux 系统有效`

				cmd := exec.Command(whiptailPath, "--title", "Groot 发行版支持列表", "--msgbox", msg, "35", "60")
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				_ = cmd.Run()
				return nil
			}

			if cleanupPath != "" {
				if chrootPath != "" || prootPath != "" || prootDistro != "" {
					return fmt.Errorf("-k 参数不能和 -c/-p/-z 同时使用")
				}
				logger.SetLevel(logger.LevelInfo)
				return mount.CleanupMounts(cleanupPath)
			}

			if chrootPath == "" && prootPath == "" && prootDistro == "" {
				cli.ShowAppHelp(cCtx)
				return fmt.Errorf("请指定 -c、-p、-z、-k 或 -l 参数，或使用子命令")
			}

			// 检查选项互斥
			count := 0
			if chrootPath != "" {
				count++
			}
			if prootPath != "" {
				count++
			}
			if prootDistro != "" {
				count++
			}
			if count > 1 {
				return fmt.Errorf("-c、-p、-z 只能选一个")
			}

			if chrootPath != "" {
				return runChroot(chrootPath, customShell)
			}

			if prootDistro != "" {
				if args.Len() < 1 {
					return fmt.Errorf("使用 -z 参数需要指定 rootfs 目录，例如：./groot -z alpine rootfs/")
				}
				rootfsPath := args.First()

				// 自动安装 proot（Termux 中）
				if err := termux.EnsureProotInstalled(); err != nil {
					return err
				}

				// 检查 proot 包是否有对应函数
				// Termux 环境需要先清理 LD_PRELOAD
				return termux.RunWithCleanEnv(func() error {
					switch prootDistro {
					case "alpine":
						return proot.RunAlpineProot(rootfsPath, customShell)
					case "debian":
						return proot.Run(rootfsPath, customShell)
					default:
						return fmt.Errorf("不支持的发行版：%s，当前支持 alpine 和 debian", prootDistro)
					}
				})
			}

			return runProot(prootPath, customShell)
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func runChroot(rootfsPath string, customShell string) error {
	if !permission.IsRoot() {
		// 在 Termux 环境下给出更友好的提示
		if termux.IsTermux() {
			return fmt.Errorf("当前设备没有 root 权限，仅支持 proot 模式（请使用 -p 参数）")
		}
		return fmt.Errorf("-c 参数（chroot）必须以 root 身份运行，请使用 sudo 或切换到 root 用户")
	}

	// 自动安装 chroot（Termux 中）
	if err := termux.EnsureChrootInstalled(); err != nil {
		return err
	}

	// Termux 环境：进入 chroot 前清理 LD_PRELOAD
	if termux.IsTermux() {
		termux.CleanupEnv()
	}
	return chroot.Run(rootfsPath, customShell)
}

func runProot(rootfsPath string, customShell string) error {
	// 自动安装 proot（Termux 中）
	if err := termux.EnsureProotInstalled(); err != nil {
		return err
	}

	// Termux 环境：proot 需要 unset LD_PRELOAD 才能正常工作
	return termux.RunWithCleanEnv(func() error {
		return proot.Run(rootfsPath, customShell)
	})
}

// openURL 使用系统默认浏览器打开指定 URL（使用 SafeLookPath 避免 Termux SIGSYS）
func openURL(url string) error {
	openers := []string{"xdg-open", "open", "termux-open-url"}
	for _, cmdName := range openers {
		path, err := termux.SafeLookPath(cmdName)
		if err == nil {
			cmd := exec.Command(path, url)
			cmd.Stderr = os.Stderr
			return cmd.Start()
		}
	}
	return fmt.Errorf("未找到可用的浏览器打开工具（尝试 xdg-open、open、termux-open-url）")
}

// downloadURLs 下载链接配置
var downloadURLs = map[string]string{
	"Void Linux": "https://voidlinux.org/download/",
	"Alpine":     "https://alpinelinux.cn/downloads/",
	"Kali":       "https://kali.download/",
	"Arch ARM":   "https://archlinuxarm.org/platforms/armv8/generic",
}

// runDownload 文本菜单选择发行版并打开浏览器下载页面
func runDownload() error {
	names := make([]string, 0, len(downloadURLs))
	for name := range downloadURLs {
		names = append(names, name)
	}
	sort.Strings(names)

	// 文本菜单，兼容 Termux
	fmt.Println()
	fmt.Println("---- 选择要下载的发行版 ----")
	for i, name := range names {
		fmt.Printf("  %d. %s\n", i+1, name)
	}
	fmt.Println("  0. 取消")
	fmt.Println("---------------------------")
	fmt.Println("\033[36m如果没找到你想要的rootfs\033[0m\033[33m请使用\033[0m\033[32mpacstrap\033[0m、\033[35mdebootstrap\033[0m、\033[31mapkstrap\033[0m、\033[32mdnfstrap\033[0m\033[33m自行构建吧\033[0m\033[36m[QwQ]\033[0m")
	fmt.Print("请输入数字: ")

	var choice int
	_, err := fmt.Scanf("%d", &choice)
	if err != nil || choice < 1 || choice > len(names) {
		fmt.Println("已取消")
		return nil
	}

	selected := names[choice-1]
	url := downloadURLs[selected]

	fmt.Printf("正在打开 %s 下载页面: %s\n", selected, url)
	if err := openURL(url); err != nil {
		fmt.Fprintf(os.Stderr, "无法自动打开浏览器，请手动访问：%s\n", url)
		return nil
	}
	fmt.Println("浏览器已打开，若未弹出请检查浏览器设置")
	return nil
}
