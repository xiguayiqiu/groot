package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"groot/internal/chroot"
	"groot/internal/check"
	"groot/internal/images"
	"groot/internal/i18n"
	"groot/internal/logger"
	"groot/internal/mount"
	"groot/internal/network"
	"groot/internal/permission"
	"groot/internal/proot"
	"groot/internal/rootless"
	"groot/internal/termux"
	"groot/internal/vmm"

	"github.com/urfave/cli/v2"
)

const (
	version = "0.4.0"
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
	i18n.Init()

	// 处理内部子命令和版本参数
	if len(os.Args) > 1 {
	switch os.Args[1] {
	case "chroot-child":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.T("cli.error.child_needs_rootfs"))
			os.Exit(1)
		}
		customShell := ""
		customUser := ""
		netMode := ""
		if len(os.Args) >= 4 {
			customShell = os.Args[3]
		}
		if len(os.Args) >= 5 {
			customUser = os.Args[4]
		}
		if len(os.Args) >= 6 {
			netMode = os.Args[5]
		}
		if err := chroot.ChildMain(os.Args[2], customShell, customUser, netMode); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.generic", err))
			os.Exit(1)
		}
		return

	case "rootless-child":
		if err := rootless.RunChild(); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.generic", err))
			os.Exit(1)
		}
		return

		case "-v", "--version":
			fmt.Printf("groot version %s\n", version)
			fmt.Println(i18n.T("cli.author.name"))
			fmt.Println(i18n.T("cli.author.license"))
			return
		}
	}

	app := &cli.App{
		Name:        "groot",
		Usage:       i18n.T("cli.usage.desc"),
		Version:     version,
		Copyright:   i18n.T("cli.copyright"),
		HideVersion: true,
		Commands: []*cli.Command{
			{
				Name:  "vmm",
				Usage: i18n.T("cli.usage.vmm"),
				Subcommands: []*cli.Command{
					{
						Name:  "run",
						Usage: i18n.T("cli.vmm.run"),
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "kernel",
								Aliases: []string{"k"},
								Usage:   i18n.T("cli.vmm.kernel"),
							},
							&cli.StringFlag{
								Name:    "rootfs",
								Aliases: []string{"r"},
								Usage:   i18n.T("cli.vmm.rootfs"),
							},
							&cli.StringFlag{
								Name:  "mem",
								Usage: i18n.T("cli.vmm.mem"),
								Value: "256",
							},
							&cli.IntFlag{
								Name:  "cpus",
								Usage: i18n.T("cli.vmm.cpus"),
								Value: 2,
							},
							&cli.BoolFlag{
								Name:    "net",
								Aliases: []string{"network"},
								Usage:   i18n.T("cli.vmm.net"),
							},
							&cli.StringFlag{
								Name:  "tap",
								Usage: i18n.T("cli.vmm.tap"),
								Value: "groot-tap0",
							},
							&cli.StringFlag{
								Name:  "host-ip",
								Usage: i18n.T("cli.vmm.host_ip"),
								Value: "172.16.0.1",
							},
							&cli.StringFlag{
								Name:  "guest-ip",
								Usage: i18n.T("cli.vmm.guest_ip"),
								Value: "172.16.0.2",
							},
							&cli.StringFlag{
								Name:  "kernel-args",
								Usage: i18n.T("cli.vmm.kernel_args"),
							},
						},
						Action: func(cCtx *cli.Context) error {
							cfg := vmm.DefaultConfig()

							if k := cCtx.String("kernel"); k != "" {
								cfg.KernelPath = k
							}
							if r := cCtx.String("rootfs"); r != "" {
								cfg.RootfsPath = r
							}
							if m := cCtx.String("mem"); m != "" {
								var memMB int
								if _, err := fmt.Sscanf(m, "%d", &memMB); err == nil && memMB > 0 {
									cfg.MemSizeMB = memMB
								}
							}
							cfg.Vcpus = cCtx.Int("cpus")
							if k := cCtx.String("kernel-args"); k != "" {
								cfg.KernelArgs = k
							}
							cfg.EnableNet = cCtx.Bool("net")
							cfg.TapDevice = cCtx.String("tap")
							cfg.HostIP = cCtx.String("host-ip")
							cfg.GuestIP = cCtx.String("guest-ip")

							if cfg.EnableNet && !permission.IsRoot() && !vmm.TapDeviceAccessible(cfg.TapDevice) {
								return fmt.Errorf("network mode requires root or pre-created TAP device\nPlease run: sudo %s vmm setup-network", os.Args[0])
							}

							if cfg.EnableNet {
								netCfg := vmm.NetworkConfig{
									TapName:   cfg.TapDevice,
									HostIP:    cfg.HostIP,
									GuestIP:   cfg.GuestIP,
									MaskLen:   24,
									HostIface: vmm.FindHostInterface(),
								}
								if err := vmm.SetupTapDevice(netCfg); err != nil {
									return fmt.Errorf("network setup failed: %w", err)
								}
								defer vmm.CleanupTapDevice(cfg.TapDevice)
							}

							return vmm.Run(cfg)
						},
					},
					{
						Name:  "download-kernel",
						Usage: i18n.T("cli.vmm.download_kernel"),
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "all",
								Usage: i18n.T("cli.vmm.download_all"),
							},
							&cli.StringFlag{
								Name:  "arch",
								Usage: i18n.T("cli.vmm.arch"),
							},
						},
						Action: func(cCtx *cli.Context) error {
							return vmm.DownloadKernel(cCtx.Bool("all"), cCtx.String("arch"))
						},
					},
					{
						Name:  "setup-network",
						Usage: i18n.T("cli.vmm.setup_network"),
						Action: func(cCtx *cli.Context) error {
							return vmm.SetupNetwork()
						},
					},
					{
						Name:  "rm-network",
						Usage: i18n.T("cli.vmm.rm_network"),
						Action: func(cCtx *cli.Context) error {
							return vmm.CleanupNetwork()
						},
					},
				},
			},
			{
				Name:  "pm",
				Usage: i18n.T("cli.usage.pm"),
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: i18n.T("cli.usage.pm_list"),
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "config",
								Usage: i18n.T("cli.usage.pm_config"),
							},
						},
						Action: func(cCtx *cli.Context) error {
							configPath := cCtx.String("config")
							cfg, err := images.LoadImagesConfig(configPath)
							if err != nil {
							return fmt.Errorf("%s", i18n.Tf("cli.config_load_fail", err))
						}

						return images.ListAvailableImages(cfg)
						},
					},
					{
						Name:  "download",
						Usage: i18n.T("cli.usage.pm_download"),
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "config",
								Usage: i18n.T("cli.usage.pm_config"),
							},
							&cli.StringFlag{
								Name:  "dest",
								Usage: i18n.T("cli.usage.pm_dest"),
								Value: "downloads",
							},
							&cli.BoolFlag{
								Name:  "all",
								Usage: i18n.T("cli.usage.pm_all"),
							},
							&cli.StringFlag{
								Name:  "distro",
								Usage: i18n.T("cli.usage.pm_distro"),
							},
							&cli.StringFlag{
								Name:  "select",
								Usage: i18n.T("cli.usage.pm_select"),
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
							return fmt.Errorf("%s", i18n.Tf("cli.config_load_fail", err))
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
						Usage: i18n.T("cli.usage.pm_make"),
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "distro",
								Usage: i18n.T("cli.usage.pm_distro"),
							},
							&cli.StringFlag{
								Name:  "version",
								Usage: i18n.T("cli.usage.pm_make_ver"),
							},
							&cli.StringFlag{
								Name:  "arch",
								Usage: i18n.T("cli.usage.pm_make_arch"),
								Value: "amd64",
							},
							&cli.StringFlag{
								Name:  "type",
								Usage: i18n.T("cli.usage.pm_make_type"),
								Value: "standard",
							},
							&cli.StringFlag{
								Name:  "dest",
								Usage: i18n.T("cli.usage.pm_make_dest"),
								Value: "rootfs",
							},
							&cli.StringFlag{
								Name:  "mirror",
								Usage: i18n.T("cli.usage.pm_make_mirror"),
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
				Usage:   i18n.T("cli.usage.chroot"),
			},
			&cli.StringFlag{
				Name:    "p",
				Aliases: []string{"proot"},
				Usage:   i18n.T("cli.usage.proot"),
			},
			&cli.StringFlag{
				Name:  "z",
				Usage: i18n.T("cli.usage.compat"),
			},
			&cli.StringFlag{
				Name:  "b",
				Usage: i18n.T("cli.usage.shell"),
			},
			&cli.StringFlag{
				Name:    "k",
				Aliases: []string{"cleanup"},
				Usage:   i18n.T("cli.usage.cleanup"),
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: i18n.T("cli.usage.verbose"),
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: i18n.T("cli.usage.debug"),
			},
			&cli.BoolFlag{
				Name:  "l",
				Usage: i18n.T("cli.usage.list"),
			},
			&cli.BoolFlag{
				Name:    "d",
				Aliases: []string{"download"},
				Usage:   i18n.T("cli.usage.download"),
			},
			&cli.BoolFlag{
				Name:  "check",
				Usage: i18n.T("cli.usage.check"),
			},
			&cli.BoolFlag{
				Name:    "net",
				Aliases: []string{"network"},
				Usage:   i18n.T("cli.usage.net"),
			},
			&cli.BoolFlag{
				Name:  "termux-lang",
				Usage: i18n.T("cli.usage.termux_lang"),
			},
		},
		Before: func(cCtx *cli.Context) error {
			// --termux-lang: 显示 Termux 语言选择 TUI
			if cCtx.Bool("termux-lang") {
				i18n.ShowTermuxLangTUI()
				os.Exit(0)
			}

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
				fmt.Println(i18n.T("cli.copyright"))
				return nil
			}

			if cCtx.Bool("check") {
				args := cCtx.Args()
				mode := check.ModeAll

				if args.Len() > 0 {
					switch args.First() {
					case "proot", "p":
						mode = check.ModeProot
					case "chroot", "c":
						mode = check.ModeChroot
					}
				}

				check.RunCheck(mode)
				return nil
			}

			chrootPath := cCtx.String("c")
			prootPath := cCtx.String("p")
			prootDistro := cCtx.String("z")
			cleanupPath := cCtx.String("k")
			customShell := cCtx.String("b")
			listDistros := cCtx.Bool("l")
			download := cCtx.Bool("d")
			netMode := cCtx.Bool("net")
			args := cCtx.Args()

			// 检查位置参数中是否包含 -net/--net/--network
			if !netMode {
				for _, arg := range args.Slice() {
					if arg == "-net" || arg == "--net" || arg == "--network" {
						netMode = true
						break
					}
				}
			}

			// -d/--download 模式：TUI 选择发行版并打开浏览器
			if download {
				return runDownload()
			}

			// -c 和 -p 模式下，第一个位置参数（若 -b 未指定）作为自定义 shell
			// 同时过滤掉 -net/--net/--network 参数
			if customShell == "" && (chrootPath != "" || prootPath != "") && args.Len() > 0 {
				for _, arg := range args.Slice() {
					if arg != "-net" && arg != "--net" && arg != "--network" {
						customShell = arg
						break
					}
				}
			}

			if cCtx.NArg() > 0 {
				if prootDistro == "" && chrootPath == "" && prootPath == "" && cleanupPath == "" && !listDistros {
					return cli.ShowAppHelp(cCtx)
				}
			}

			if listDistros {
				// 检查 whiptail 是否可用
				whiptailPath, err := termux.SafeLookPath("whiptail")
				if err != nil {
					// 尝试安装 whiptail
					fmt.Println(i18n.T("cli.hint.install_whiptail"))
					distro := detectDistro()
					fmt.Printf("%s", i18n.Tf("cli.hint.detect_distro", distro))
					installed := false
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
								installed = true
							}
						}
					case "void":
						if _, err := termux.SafeLookPath("xbps-install"); err == nil {
							fmt.Println(i18n.T("cli.hint.xbps_installing"))
							cmd, err := termux.Command("xbps-install", "-Sy", "newt")
							if err != nil {
								break
							}
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							cmd.Stdin = os.Stdin
							if err := cmd.Run(); err == nil {
								installed = true
						} else {
							fmt.Printf("%s", i18n.Tf("cli.hint.install_newt_fail", err))
							}
						} else {
							fmt.Printf("%s", i18n.Tf("cli.hint.install_newt_xbps_fail", err))
						}
					case "debian":
						if _, err := termux.SafeLookPath("apt"); err == nil {
							fmt.Println(i18n.T("cli.hint.apt_installing"))
							cmd, err := termux.Command("apt", "install", "-y", "newt")
							if err != nil {
								break
							}
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							cmd.Stdin = os.Stdin
							if err := cmd.Run(); err == nil {
								installed = true
							}
						}
					case "arch":
						if _, err := termux.SafeLookPath("pacman"); err == nil {
							fmt.Println(i18n.T("cli.hint.pacman_installing"))
							cmd, err := termux.Command("pacman", "-S", "--noconfirm", "newt")
							if err != nil {
								break
							}
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							cmd.Stdin = os.Stdin
							if err := cmd.Run(); err == nil {
								installed = true
							}
						}
					}

					if installed {
						whiptailPath, err = termux.SafeLookPath("whiptail")
					}

					if err != nil {
						// 回退到文本显示
						fmt.Println(i18n.T("cli.hint.text_mode"))
						fmt.Println()
						fmt.Println(i18n.T("cli.distro.title") + "：")
						fmt.Println()
						fmt.Println(i18n.T("cli.dl_mode_header"))
						fmt.Println("| " + i18n.T("cli.distro.supported") + "   | proot | chroot                |")
						fmt.Println("| :------- | :---- | :-------------------- |")
						fmt.Println("| Alpine   | ✓     | ✓                     |")
						fmt.Println("| Ubuntu   | -     | ✓                     |")
						fmt.Println("| Void     | -     | ✓                     |")
						fmt.Println()
						fmt.Println(i18n.T("cli.make_mode_header"))
						fmt.Println("| " + i18n.T("cli.distro.supported") + "   | proot | chroot                |")
						fmt.Println("| :------- | :---- | :-------------------- |")
						fmt.Println("| Debian   | ✓     | ✓                     |")
						fmt.Println("| Ubuntu   | -     | ✓                     |")
						fmt.Println("| Arch     | -     | ✓ (仅在 Arch 系统有效)|")
						fmt.Println()
						fmt.Println(i18n.T("cli.distro.note") + "：")
						fmt.Println("  ✓ " + i18n.T("cli.distro.full_support"))
						fmt.Println("  - " + i18n.T("cli.distro.chroot_only"))
						fmt.Println()
						return nil
					}
				}

			// 使用 whiptail 显示
			msg := i18n.T("cli.distro.title") + `
+------------------------------+
` + i18n.T("cli.dl_mode_table") + `
+------------------------------+
| ` + i18n.T("cli.distro.supported") + `   | proot | chroot    |
+--------- + ----- + ----------+
| Alpine   | ✓     | ✓         |
| Ubuntu   | -     | ✓         |
| Void     | -     | ✓         |
+------------------------------+
` + i18n.T("cli.make_mode_table") + `
+------------------------------+
| ` + i18n.T("cli.distro.supported") + `   | proot | chroot    |
+--------- + ----- + ----------+
| Debian   | ✓     | ✓         |
| Ubuntu   | -     | ✓         |
| Arch     | -     | ✓         |
+------------------------------+

` + i18n.T("cli.distro.note") + `：
  ✓ ` + i18n.T("cli.distro.full_support") + `
  - ` + i18n.T("cli.distro.chroot_only") + `
  Arch 构建仅在 Arch Linux 系统有效`

				cmd := exec.Command(whiptailPath, "--title", i18n.T("cli.distro.whiptail_title"), "--msgbox", msg, "35", "60")
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				_ = cmd.Run()
				return nil
			}

			if cleanupPath != "" {
				if chrootPath != "" || prootPath != "" || prootDistro != "" {
					return fmt.Errorf("%s", i18n.T("cli.error.k_conflict"))
				}
				logger.SetLevel(logger.LevelInfo)
				return mount.CleanupMounts(cleanupPath)
			}

			if chrootPath == "" && prootPath == "" && prootDistro == "" {
				cli.ShowAppHelp(cCtx)
				return fmt.Errorf("%s", i18n.T("cli.error.no_params"))
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
				return fmt.Errorf("%s", i18n.T("cli.error.mutual_exclusive"))
			}

			if chrootPath != "" {
				return runChroot(chrootPath, customShell, netMode)
			}

			if prootDistro != "" {
				if args.Len() < 1 {
					return fmt.Errorf("%s", i18n.T("cli.error.z_needs_rootfs"))
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
						return fmt.Errorf("%s", i18n.Tf("cli.error.unsupported_distro", prootDistro))
					}
				})
			}

			return runProot(prootPath, customShell)
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.generic", err))
		os.Exit(1)
	}
}

func runChroot(rootfsPath string, customShell string, netMode bool) error {
	if !permission.IsRoot() {
		// 在 Termux 环境下给出更友好的提示
		if termux.IsTermux() {
			return fmt.Errorf("%s", i18n.T("cli.error.no_root_termux"))
		}
		return fmt.Errorf("%s", i18n.T("cli.error.no_root_chroot"))
	}

	// 检查 -net 参数是否与 -c 一起使用
	if netMode {
		supported, msg := network.CheckNetworkSupport()
		if !supported {
			return fmt.Errorf("%s", i18n.Tf("cli.error.net_unsupported", msg))
		}
	}

	// 自动安装 chroot（Termux 中）
	if err := termux.EnsureChrootInstalled(); err != nil {
		return err
	}

	// Termux 环境：进入 chroot 前清理 LD_PRELOAD
	if termux.IsTermux() {
		termux.CleanupEnv()
	}
	return chroot.Run(rootfsPath, customShell, netMode)
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
	return fmt.Errorf("%s", i18n.T("cli.browser_not_found"))
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
	fmt.Println(i18n.T("cli.download.title"))
	for i, name := range names {
		fmt.Printf("  %d. %s\n", i+1, name)
	}
	fmt.Println("  0. " + i18n.T("cli.download.cancel"))
	fmt.Println("---------------------------")
	fmt.Println(i18n.T("cli.download.hint"))
	fmt.Print(i18n.T("cli.download.prompt"))

	var choice int
	_, err := fmt.Scanf("%d", &choice)
	if err != nil || choice < 1 || choice > len(names) {
		fmt.Println(i18n.T("cli.download.cancelled"))
		return nil
	}

	selected := names[choice-1]
	url := downloadURLs[selected]

	fmt.Printf("%s", i18n.Tf("cli.download.opening", selected, url))
	if err := openURL(url); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.Tf("cli.download.open_fail", url))
		return nil
	}
	fmt.Println(i18n.T("cli.download.opened"))
	return nil
}
