package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"groot/internal/chroot"
	"groot/internal/images"
	"groot/internal/logger"
	"groot/internal/mount"
	"groot/internal/permission"
	"groot/internal/proot"

	"github.com/urfave/cli/v2"
)

const (
	version = "0.1"
)

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
		Copyright:   "MIT License - Copyright © 2025 弈秋忘忧白帽",
		HideVersion: true,
		Commands: []*cli.Command{
			{
				Name:  "pm",
				Usage: "包管理器：管理 rootfs 镜像下载",
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
				Usage: "专属 proot 兼容模式：指定发行版（alpine/debian/ubuntu），然后指定 rootfs 目录",
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
		},
		Before: func(cCtx *cli.Context) error {
			if cCtx.Bool("debug") {
				logger.SetLevel(logger.LevelDebug)
			} else if cCtx.Bool("verbose") {
				logger.SetLevel(logger.LevelInfo)
			} else {
				logger.SetLevel(logger.LevelInfo)
			}
			return nil
		},
		Action: func(cCtx *cli.Context) error {
			if cCtx.Bool("v") || cCtx.Bool("version") {
				fmt.Printf("groot version %s\n", version)
				fmt.Println("MIT License - Copyright © 2025 弈秋忘忧白帽")
				return nil
			}

			if cCtx.NArg() > 0 {
				return cli.ShowAppHelp(cCtx)
			}

			chrootPath := cCtx.String("c")
			prootPath := cCtx.String("p")
			prootDistro := cCtx.String("z")
			cleanupPath := cCtx.String("k")
			customShell := cCtx.String("b")
			listDistros := cCtx.Bool("l")
			args := cCtx.Args()

			if listDistros {
				// 检查 whiptail 是否可用
				whiptailPath, err := exec.LookPath("whiptail")
				if err != nil {
					// 回退到文本显示
					fmt.Println("支持的发行版列表：")
					fmt.Println()
					fmt.Println("| 发行版   | proot | chroot                |")
					fmt.Println("| :------- | :---- | :-------------------- |")
					fmt.Println("| Alpine   | ✓     | ✓                     |")
					fmt.Println("| Debian   | ✓     | ✓                     |")
					fmt.Println("| Void     | -     | ✓                     |")
					fmt.Println("| Ubuntu   | -     | ✓                     |")
					fmt.Println("| Kali     | -     | ✓                     |")
					fmt.Println("| RedHat   | -     | ✓                     |")
					fmt.Println("| Fedora   | -     | ✓                     |")
					fmt.Println("| CentOS   | -     | ✓                     |")
					fmt.Println("| Arch     | -     | ✓                     |")
					fmt.Println()
					fmt.Println("说明：")
					fmt.Println("  ✓ 完美支持")
					fmt.Println("  - 仅 chroot 模式支持")
					fmt.Println()
					return nil
				}

				// 使用 whiptail 显示
				msg := `支持的发行版列表
|------------------------------------------|
| 发行版   | proot | chroot                |
| -------- | ----- | ----------------------|
| Alpine   | ✓     | ✓                     |
| Debian   | ✓     | ✓                     |
| Void     | -     | ✓                     |
| Ubuntu   | -     | ✓                     |
| Kali     | -     | ✓                     |
| RedHat   | -     | ✓                     |
| Fedora   | -     | ✓                     |
| CentOS   | -     | ✓                     |
| Arch     | -     | ✓                     |
--------------------------------------------

说明：
  ✓ 完美支持
  - 仅 chroot 模式支持`

				cmd := exec.Command(whiptailPath, "--title", "Groot 发行版支持列表", "--msgbox", msg, "28", "55")
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

				// 检查 proot 包是否有对应函数
				switch prootDistro {
				case "alpine":
					return proot.RunAlpineProot(rootfsPath, customShell)
				case "debian":
					return proot.Run(rootfsPath, customShell)
				case "ubuntu":
					return proot.RunUbuntuProot(rootfsPath, customShell)
				default:
					return fmt.Errorf("不支持的发行版：%s，当前支持 alpine、debian 和 ubuntu", prootDistro)
				}
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
		logger.Info("使用 User Namespace，无需真实 root 权限")
	}

	return chroot.Run(rootfsPath, customShell)
}

func runProot(rootfsPath string, customShell string) error {
	return proot.Run(rootfsPath, customShell)
}
