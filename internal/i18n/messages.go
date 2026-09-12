package i18n

// messages 定义所有翻译字符串
// key 格式: "模块.功能"，例如 "cli.usage.chroot"
var messages = map[string]map[Lang]string{
	// ==========================================
	// CLI 层 (cmd/groot/main.go)
	// ==========================================

	// 错误消息
	"cli.error.child_needs_rootfs": {
		LangZH: "错误: chroot-child 子命令需要 rootfs 路径参数",
		LangEN: "Error: chroot-child subcommand requires rootfs path argument",
	},
	"cli.error.generic": {
		LangZH: "错误: %v",
		LangEN: "Error: %v",
	},
	"cli.error.mutual_exclusive": {
		LangZH: "-c、-p、-z 只能选一个",
		LangEN: "-c, -p, -z are mutually exclusive",
	},
	"cli.error.z_needs_rootfs": {
		LangZH: "使用 -z 参数需要指定 rootfs 目录，例如：./groot -z alpine rootfs/",
		LangEN: "-z requires a rootfs directory, e.g.: ./groot -z alpine rootfs/",
	},
	"cli.error.unsupported_distro": {
		LangZH: "不支持的发行版：%s，当前支持 alpine 和 debian",
		LangEN: "Unsupported distro: %s, only alpine and debian are supported",
	},
	"cli.error.no_params": {
		LangZH: "请指定 -c、-p、-z、-k 或 -l 参数，或使用子命令",
		LangEN: "Please specify -c, -p, -z, -k or -l parameter, or use subcommands",
	},
	"cli.error.k_conflict": {
		LangZH: "-k 参数不能和 -c/-p/-z 同时使用",
		LangEN: "-k cannot be used together with -c/-p/-z",
	},
	"cli.error.no_root_chroot": {
		LangZH: "-c 参数（chroot）必须以 root 身份运行，请使用 sudo 或切换到 root 用户",
		LangEN: "-c (chroot) requires root privileges, please use sudo or switch to root",
	},
	"cli.error.no_root_termux": {
		LangZH: "当前设备没有 root 权限，仅支持 proot 模式（请使用 -p 参数）",
		LangEN: "No root privileges, only proot mode is supported (use -p parameter)",
	},
	"cli.error.net_unsupported": {
		LangZH: "网络命名空间不支持: %s",
		LangEN: "Network namespace not supported: %s",
	},

	// 作者信息
	"cli.author.name": {
		LangZH: "作者：弈秋忘忧白帽",
		LangEN: "Author: yiqiu",
	},
	"cli.author.license": {
		LangZH: "开源协议：MIT",
		LangEN: "License: MIT",
	},
	"cli.copyright": {
		LangZH: "MIT License - Copyright © 2026 弈秋忘忧白帽",
		LangEN: "MIT License - Copyright © 2026 yiqiu",
	},

	// Usage 文本
	"cli.usage.desc": {
		LangZH: "Go 版双模式隔离工具（chroot/proot）",
		LangEN: "Go dual-mode isolation tool (chroot/proot)",
	},
	"cli.usage.pm": {
		LangZH: "包管理器：管理 rootfs 镜像下载和构建",
		LangEN: "Package manager: manage rootfs image download and build",
	},
	"cli.usage.pm_list": {
		LangZH: "列出可用的 rootfs 镜像",
		LangEN: "List available rootfs images",
	},
	"cli.usage.pm_config": {
		LangZH: "指定镜像配置文件路径（默认使用嵌入的配置）",
		LangEN: "Specify image config file path (default: embedded config)",
	},
	"cli.usage.pm_download": {
		LangZH: "下载 rootfs 镜像",
		LangEN: "Download rootfs images",
	},
	"cli.usage.pm_dest": {
		LangZH: "指定下载目录",
		LangEN: "Specify download directory",
	},
	"cli.usage.pm_all": {
		LangZH: "下载所有镜像",
		LangEN: "Download all images",
	},
	"cli.usage.pm_distro": {
		LangZH: "指定发行版（void/ubuntu/alpine）",
		LangEN: "Specify distro (void/ubuntu/alpine)",
	},
	"cli.usage.pm_select": {
		LangZH: "选择要下载的镜像（架构/版本/库）",
		LangEN: "Select image to download (arch/version/libc)",
	},
	"cli.usage.pm_make": {
		LangZH: "构建 rootfs 镜像",
		LangEN: "Build rootfs image",
	},
	"cli.usage.pm_make_ver": {
		LangZH: "指定发行版版本（如 12 对于 Debian, 22.04 对于 Ubuntu, v3.20 对于 Alpine, x86_64 对于 Void）",
		LangEN: "Specify version (e.g. 12 for Debian, 22.04 for Ubuntu, v3.20 for Alpine, x86_64 for Void)",
	},
	"cli.usage.pm_make_arch": {
		LangZH: "指定架构（amd64/aarch64）",
		LangEN: "Specify architecture (amd64/aarch64)",
	},
	"cli.usage.pm_make_type": {
		LangZH: "指定构建类型：minimal（精简版）、standard（标准版）、full（完整版）（可选，默认 standard）",
		LangEN: "Build type: minimal, standard, full (default: standard)",
	},
	"cli.usage.pm_make_dest": {
		LangZH: "指定构建目录",
		LangEN: "Specify build directory",
	},
	"cli.usage.pm_make_mirror": {
		LangZH: "指定镜像源：tsinghua（清华）、ustc（中科大）、official（官方）或自定义 URL（可选，默认 official）",
		LangEN: "Mirror: tsinghua, ustc, official, or custom URL (default: official)",
	},
	"cli.usage.chroot": {
		LangZH: "chroot 模式：指定 rootfs 目录（需要 root 权限）",
		LangEN: "chroot mode: specify rootfs directory (requires root)",
	},
	"cli.usage.proot": {
		LangZH: "proot 模式：指定 rootfs 目录（无需 root 权限）",
		LangEN: "proot mode: specify rootfs directory (no root required)",
	},
	"cli.usage.compat": {
		LangZH: "专属 proot 兼容模式：指定发行版（alpine/debian），然后指定 rootfs 目录",
		LangEN: "Proot compat mode: specify distro (alpine/debian), then rootfs directory",
	},
	"cli.usage.shell": {
		LangZH: "指定容器目录的 shell 解释器路径（例如：/bin/ash、/bin/bash）",
		LangEN: "Specify shell interpreter path (e.g.: /bin/ash, /bin/bash)",
	},
	"cli.usage.cleanup": {
		LangZH: "清理指定 rootfs 下的残留挂载点",
		LangEN: "Cleanup leftover mount points under specified rootfs",
	},
	"cli.usage.verbose": {
		LangZH: "启用详细日志",
		LangEN: "Enable verbose logging",
	},
	"cli.usage.debug": {
		LangZH: "启用调试日志",
		LangEN: "Enable debug logging",
	},
	"cli.usage.list": {
		LangZH: "列出支持的发行版列表",
		LangEN: "List supported distributions",
	},
	"cli.usage.download": {
		LangZH: "打开浏览器选择并下载 rootfs 镜像",
		LangEN: "Open browser to select and download rootfs images",
	},
	"cli.usage.check": {
		LangZH: "检查设备是否符合要求（可加 proot/chroot 参数指定检查类型）",
		LangEN: "Check device requirements (optionally specify proot/chroot)",
	},
	"cli.usage.net": {
		LangZH: "为 chroot 创建独立网络命名空间（仅 chroot 模式有效）",
		LangEN: "Create isolated network namespace for chroot (chroot mode only)",
	},
	"cli.usage.termux_lang": {
		LangZH: "配置 Termux 语言（生成 ~/.termux/locale.conf）",
		LangEN: "Configure Termux language (generate ~/.termux/locale.conf)",
	},

	// 提示消息
	"cli.hint.install_whiptail": {
		LangZH: "提示: whiptail 没有找到，尝试安装它以获得更好的交互体验...",
		LangEN: "Hint: whiptail not found, trying to install for better UI...",
	},
	"cli.hint.detect_distro": {
		LangZH: "检测到当前系统是: %s\n",
		LangEN: "Detected system: %s\n",
	},
	"cli.hint.install_newt_fail": {
		LangZH: "安装 newt 失败: %v\n",
		LangEN: "Failed to install newt: %v\n",
	},
	"cli.hint.install_newt_xbps_fail": {
		LangZH: "未找到 xbps-install: %v\n",
		LangEN: "xbps-install not found: %v\n",
	},
	"cli.hint.xbps_installing": {
		LangZH: "找到 xbps-install，正在安装 newt 包...",
		LangEN: "Found xbps-install, installing newt package...",
	},
	"cli.hint.apt_installing": {
		LangZH: "找到 apt，正在安装 newt 包...",
		LangEN: "Found apt, installing newt package...",
	},
	"cli.hint.pacman_installing": {
		LangZH: "找到 pacman，正在安装 newt 包...",
		LangEN: "Found pacman, installing newt package...",
	},
	"cli.hint.text_mode": {
		LangZH: "将使用文本交互模式",
		LangEN: "Using text interactive mode",
	},

	// 发行版列表
	"cli.distro.supported": {
		LangZH: "发行版",
		LangEN: "Distro",
	},
	"cli.distro.full_support": {
		LangZH: "完美支持",
		LangEN: "Full Support",
	},
	"cli.distro.chroot_only": {
		LangZH: "仅 chroot 模式支持",
		LangEN: "Chroot Mode Only",
	},
	"cli.distro.note": {
		LangZH: "说明",
		LangEN: "Note",
	},
	"cli.distro.title": {
		LangZH: "Groot 支持的发行版列表",
		LangEN: "Groot Supported Distributions",
	},
	"cli.distro.whiptail_title": {
		LangZH: "Groot 发行版支持列表",
		LangEN: "Groot Distribution Support List",
	},

	// 下载菜单
	"cli.download.title": {
		LangZH: "---- 选择要下载的发行版 ----",
		LangEN: "---- Select distro to download ----",
	},
	"cli.download.cancel": {
		LangZH: "0. 取消",
		LangEN: "0. Cancel",
	},
	"cli.download.hint": {
		LangZH: "如果没找到你想要的rootfs请使用pacstrap、debootstrap、apkstrap、dnfstrap自行构建吧[QwQ]",
		LangEN: "If your rootfs is not listed, try pacstrap, debootstrap, apkstrap, or dnfstrap to build one [QwQ]",
	},
	"cli.download.prompt": {
		LangZH: "请输入数字: ",
		LangEN: "Enter number: ",
	},
	"cli.download.cancelled": {
		LangZH: "已取消",
		LangEN: "Cancelled",
	},
	"cli.download.opening": {
		LangZH: "正在打开 %s 下载页面: %s\n",
		LangEN: "Opening %s download page: %s\n",
	},
	"cli.download.open_fail": {
		LangZH: "无法自动打开浏览器，请手动访问：%s\n",
		LangEN: "Cannot open browser automatically, please visit: %s\n",
	},
	"cli.download.opened": {
		LangZH: "浏览器已打开，若未弹出请检查浏览器设置",
		LangEN: "Browser opened, check browser settings if not shown",
	},

	// ==========================================
	// Logger (internal/logger/logger.go)
	// ==========================================

	"logger.banner": {
		LangZH: "[Groot] 如果你喜欢请前往 gyscan.space 下载 gyscan 吧～",
		LangEN: "[Groot] If you like it, visit gyscan.space to download gyscan~",
	},

	// ==========================================
	// Chroot (internal/chroot/chroot.go)
	// ==========================================

	"chroot.start": {
		LangZH: "开始 chroot 模式，rootfs 路径: %s",
		LangEN: "Starting chroot mode, rootfs: %s",
	},
	"chroot.path_fail": {
		LangZH: "转换绝对路径失败: %v",
		LangEN: "Failed to convert to absolute path: %v",
	},
	"chroot.cleanup_warn": {
		LangZH: "清理 rootfs 环境失败: %v",
		LangEN: "Failed to cleanup rootfs environment: %v",
	},
	"chroot.running_as_root": {
		LangZH: "以真实 root 权限运行",
		LangEN: "Running as real root",
	},
	"chroot.running_user_ns": {
		LangZH: "以普通用户运行，使用 User Namespace 隔离",
		LangEN: "Running as non-root, using User Namespace isolation",
	},
	"chroot.fixing_perms": {
		LangZH: "正在修复权限...",
		LangEN: "Fixing permissions...",
	},
	"chroot.using_exe": {
		LangZH: "使用可执行文件: %s",
		LangEN: "Using executable: %s",
	},
	"chroot.creating_child": {
		LangZH: "创建子进程并启用命名空间隔离...",
		LangEN: "Creating child process with namespace isolation...",
	},
	"chroot.start_fail": {
		LangZH: "启动子进程失败: %v",
		LangEN: "Failed to start child process: %v",
	},
	"chroot.net_setup_fail": {
		LangZH: "设置网络命名空间失败: %v",
		LangEN: "Failed to setup network namespace: %v",
	},
	"chroot.net_setup_done": {
		LangZH: "网络命名空间配置完成",
		LangEN: "Network namespace configuration complete",
	},
	"chroot.entering": {
		LangZH: "进入 chroot 子进程",
		LangEN: "Entering chroot child process",
	},
	"chroot.hostname_set": {
		LangZH: "设置主机名为: %s",
		LangEN: "Setting hostname to: %s",
	},
	"chroot.hostname_fail": {
		LangZH: "设置主机名失败: %v",
		LangEN: "Failed to set hostname: %v",
	},
	"chroot.mounting_vfs": {
		LangZH: "正在挂载虚拟文件系统...",
		LangEN: "Mounting virtual filesystems...",
	},
	"chroot.mount_done": {
		LangZH: "虚拟文件系统挂载成功: %v",
		LangEN: "Virtual filesystems mounted: %v",
	},
	"chroot.starting_shell": {
		LangZH: "启动 shell: %s",
		LangEN: "Starting shell: %s",
	},
	"chroot.shell_fail": {
		LangZH: "启动 shell 失败: %v",
		LangEN: "Failed to start shell: %v",
	},
	"chroot.shell_exit": {
		LangZH: "shell退出，退出码: %d",
		LangEN: "Shell exited with code: %d",
	},
	"chroot.shell_error": {
		LangZH: "shell运行失败: %v",
		LangEN: "Shell error: %v",
	},
	"chroot.unmount_fail": {
		LangZH: "卸载失败: %v",
		LangEN: "Unmount failed: %v",
	},
	"chroot.unmounted": {
		LangZH: "虚拟文件系统已卸载",
		LangEN: "Virtual filesystems unmounted",
	},

	// ==========================================
	// Proot (internal/proot/proot.go)
	// ==========================================

	"proot.start": {
		LangZH: "开始 proot 模式，rootfs 路径: %s",
		LangEN: "Starting proot mode, rootfs: %s",
	},
	"proot.not_installed": {
		LangZH: "系统未安装 proot，请先安装：sudo apt install proot",
		LangEN: "proot not installed, please install: sudo apt install proot",
	},
	"proot.found": {
		LangZH: "找到系统 proot: %s",
		LangEN: "Found system proot: %s",
	},
	"proot.rootfs_not_exist": {
		LangZH: "rootfs 不存在: %s",
		LangEN: "rootfs does not exist: %s",
	},
	"proot.passwd_fail": {
		LangZH: "无法读取 passwd 文件，使用默认用户信息: %v",
		LangEN: "Cannot read passwd file, using default user info: %v",
	},
	"proot.exec_cmd": {
		LangZH: "执行 proot 命令: %s %v",
		LangEN: "Executing proot: %s %v",
	},
	"proot.start_fail": {
		LangZH: "启动 proot 失败: %v",
		LangEN: "Failed to start proot: %v",
	},
	"proot.run_fail": {
		LangZH: "proot 运行失败: %v",
		LangEN: "proot failed: %v",
	},

	// ==========================================
	// Alpine Proot (internal/proot/alpine-proot.go)
	// ==========================================

	"alpine.start": {
		LangZH: "Alpine 专属模式启动！rootfs: %s",
		LangEN: "Alpine dedicated mode started! rootfs: %s",
	},
	"alpine.no_proot": {
		LangZH: "没找到 proot：sudo apt install proot",
		LangEN: "proot not found: sudo apt install proot",
	},
	"alpine.exec_cmd": {
		LangZH: "执行完美 proot 命令：%s %v",
		LangEN: "Executing proot: %s %v",
	},

	// ==========================================
	// Mount (internal/mount/mount.go)
	// ==========================================

	"mount.prepare_dirs_fail": {
		LangZH: "准备目录失败: %v",
		LangEN: "Failed to prepare directories: %v",
	},
	"mount.already_mounted": {
		LangZH: "跳过已挂载: %s",
		LangEN: "Skip already mounted: %s",
	},
	"mount.create_dir_fail": {
		LangZH: "创建目录失败 %s: %v",
		LangEN: "Failed to create directory %s: %v",
	},
	"mount.mounting": {
		LangZH: "挂载: %s -> %s",
		LangEN: "Mounting: %s -> %s",
	},
	"mount.mount_fail": {
		LangZH: "挂载失败 %s: %v",
		LangEN: "Mount failed %s: %v",
	},
	"mount.required_fallback": {
		LangZH: "必需挂载点挂载失败 %s: %v，尝试备用方案",
		LangEN: "Required mount %s failed: %v, trying fallback",
	},
	"mount.tmpfs_fallback_ok": {
		LangZH: "使用 tmpfs 备用方案挂载成功: %s",
		LangEN: "Tmpfs fallback mounted: %s",
	},
	"mount.devfiles_fail": {
		LangZH: "创建设备文件失败: %v",
		LangEN: "Failed to create device files: %v",
	},
	"mount.skip_source": {
		LangZH: "源不存在，跳过: %s",
		LangEN: "Source not found, skipping: %s",
	},
	"mount.create_target_fail": {
		LangZH: "创建目标文件失败 %s: %v",
		LangEN: "Failed to create target file %s: %v",
	},
	"mount.alarch_detected": {
		LangZH: "检测到 Arch Linux，设置 pacman 专用目录...",
		LangEN: "Arch Linux detected, setting up pacman directories...",
	},
	"mount.unmount_skip": {
		LangZH: "跳过未挂载: %s",
		LangEN: "Skip not mounted: %s",
	},
	"mount.unmounting": {
		LangZH: "卸载: %s",
		LangEN: "Unmounting: %s",
	},
	"mount.unmount_lazy_skip": {
		LangZH: "跳过无法解析的挂载点: %s (命名空间销毁时会自动清理)",
		LangEN: "Skip unresolvable mount: %s (will be cleaned on namespace destroy)",
	},
	"mount.unmount_fail": {
		LangZH: "卸载失败 %s: %v",
		LangEN: "Unmount failed %s: %v",
	},
	"mount.rootfs_not_exist": {
		LangZH: "rootfs 不存在: %v",
		LangEN: "rootfs does not exist: %v",
	},
	"mount.missing_dir": {
		LangZH: "缺少必需目录 %s",
		LangEN: "Missing required directory %s",
	},
	"mount.missing_shell": {
		LangZH: "缺少 shell: %s",
		LangEN: "Missing shell: %s",
	},
	"mount.still_remaining": {
		LangZH: "仍有 %d 个挂载未清理",
		LangEN: "%d mount points still remaining",
	},
	"mount.cleanup_done": {
		LangZH: "挂载清理完成",
		LangEN: "Mount cleanup complete",
	},

	// ==========================================
	// Check (internal/check/check.go)
	// ==========================================

	"check.proot_report": {
		LangZH: "Proot 环境检查报告",
		LangEN: "Proot Environment Check Report",
	},
	"check.chroot_report": {
		LangZH: "Chroot 环境检查报告",
		LangEN: "Chroot Environment Check Report",
	},
	"check.full_report": {
		LangZH: "Groot 设备检查报告",
		LangEN: "Groot Device Check Report",
	},
	"check.device": {
		LangZH: "设备",
		LangEN: "Device",
	},
	"check.system": {
		LangZH: "系统",
		LangEN: "System",
	},
	"check.environment": {
		LangZH: "环境",
		LangEN: "Environment",
	},
	"check.arch": {
		LangZH: "架构",
		LangEN: "Architecture",
	},
	"check.items": {
		LangZH: "检查项目",
		LangEN: "Check Items",
	},
	"check.all_pass": {
		LangZH: "所有检查通过",
		LangEN: "All checks passed",
	},
	"check.proot_supported": {
		LangZH: "您的设备完全支持 proot 模式！",
		LangEN: "Your device fully supports proot mode!",
	},
	"check.chroot_supported": {
		LangZH: "您的设备完全支持 chroot 模式！",
		LangEN: "Your device fully supports chroot mode!",
	},
	"check.pass": {
		LangZH: "通过",
		LangEN: "PASS",
	},
	"check.fail": {
		LangZH: "失败",
		LangEN: "FAIL",
	},
	"check.proot_limited": {
		LangZH: "proot 模式可能受限",
		LangEN: "Proot mode may be limited",
	},
	"check.chroot_limited": {
		LangZH: "chroot 模式可能受限",
		LangEN: "Chroot mode may be limited",
	},
	"check.hint": {
		LangZH: "提示：",
		LangEN: "Hint: ",
	},
	"check.termux_env": {
		LangZH: "Termux 环境",
		LangEN: "Termux Environment",
	},
	"check.container_env": {
		LangZH: "容器环境",
		LangEN: "Container Environment",
	},
	"check.detected_container": {
		LangZH: "检测到容器环境",
		LangEN: "Container environment detected",
	},
	"check.run_env": {
		LangZH: "运行环境",
		LangEN: "Runtime Environment",
	},
	"check.standard_linux": {
		LangZH: "标准 Linux 环境",
		LangEN: "Standard Linux Environment",
	},
	"check.root_perm": {
		LangZH: "Root 权限",
		LangEN: "Root Privileges",
	},
	"check.has_root": {
		LangZH: "当前具有 root 权限",
		LangEN: "Currently has root privileges",
	},
	"check.device_rooted": {
		LangZH: "设备已 root（可通过 su 获取）",
		LangEN: "Device is rooted (accessible via su)",
	},
	"check.no_root": {
		LangZH: "未检测到 root 权限",
		LangEN: "No root privileges detected",
	},
	"check.android_ver": {
		LangZH: "Android 版本",
		LangEN: "Android Version",
	},
	"check.device_model": {
		LangZH: "设备型号",
		LangEN: "Device Model",
	},
	"check.cpu_arch": {
		LangZH: "CPU 架构",
		LangEN: "CPU Architecture",
	},
	"check.arch_32bit_compat": {
		LangZH: "32位兼容",
		LangEN: "32-bit Compatibility",
	},
	"check.cpu_info": {
		LangZH: "CPU 信息",
		LangEN: "CPU Info",
	},
	"check.native_32bit": {
		LangZH: "原生 32 位架构",
		LangEN: "Native 32-bit architecture",
	},
	"check.has_lib32": {
		LangZH: "支持 (有 /lib32)",
		LangEN: "Supported (has /lib32)",
	},
	"check.no_32bit_compat": {
		LangZH: "未检测到 32 位兼容层",
		LangEN: "No 32-bit compatibility layer detected",
	},
	"check.kernel_ver": {
		LangZH: "内核版本",
		LangEN: "Kernel Version",
	},
	"check.selinux": {
		LangZH: "SELinux 状态",
		LangEN: "SELinux Status",
	},
	"check.user_ns": {
		LangZH: "用户命名空间",
		LangEN: "User Namespace",
	},
	"check.unpriv_ns_support": {
		LangZH: "支持非特权用户命名空间",
		LangEN: "Unprivileged user namespace supported",
	},
	"check.ns_support": {
		LangZH: "命名空间支持",
		LangEN: "Namespace Support",
	},
	"check.pid_ns": {
		LangZH: "PID 命名空间",
		LangEN: "PID Namespace",
	},
	"check.net_ns": {
		LangZH: "网络命名空间",
		LangEN: "Network Namespace",
	},
	"check.uts_ns": {
		LangZH: "UTS 命名空间",
		LangEN: "UTS Namespace",
	},
	"check.ipc_ns": {
		LangZH: "IPC 命名空间",
		LangEN: "IPC Namespace",
	},
	"check.cgroup": {
		LangZH: "cgroup 支持",
		LangEN: "cgroup Support",
	},
	"check.veth": {
		LangZH: "虚拟以太网设备",
		LangEN: "Virtual Ethernet Device",
	},
	"check.bridge": {
		LangZH: "网桥支持",
		LangEN: "Bridge Support",
	},
	"check.storage": {
		LangZH: "存储空间",
		LangEN: "Storage Space",
	},
	"check.storage_fail": {
		LangZH: "无法获取存储信息",
		LangEN: "Cannot get storage info",
	},
	"check.termux_data": {
		LangZH: "Termux 数据目录",
		LangEN: "Termux Data Directory",
	},
	"check.no_proot": {
		LangZH: "未安装 proot",
		LangEN: "proot not installed",
	},
	"check.proot_ns": {
		LangZH: "proot 需要用户命名空间支持",
		LangEN: "proot requires user namespace support",
	},
	"check.sysctl": {
		LangZH: "sysctl 支持",
		LangEN: "sysctl Support",
	},
	"check.enabled": {
		LangZH: "已启用",
		LangEN: "Enabled",
	},
	"check.seccomp": {
		LangZH: "Seccomp 支持",
		LangEN: "Seccomp Support",
	},
	"check.fs_access": {
		LangZH: "文件系统访问",
		LangEN: "Filesystem Access",
	},
	"check.tmp_ok": {
		LangZH: "可以访问 /tmp",
		LangEN: "Can access /tmp",
	},
	"check.proc_fs": {
		LangZH: "proc 文件系统",
		LangEN: "proc Filesystem",
	},
	"check.proc_ok": {
		LangZH: "可以访问 /proc",
		LangEN: "Can access /proc",
	},
	"check.proc_fail": {
		LangZH: "无法访问 /proc",
		LangEN: "Cannot access /proc",
	},
	"check.chroot_needs_root": {
		LangZH: "chroot 需要 root 权限",
		LangEN: "chroot requires root privileges",
	},
	"check.chroot_cmd": {
		LangZH: "未找到 chroot 命令",
		LangEN: "chroot command not found",
	},
	"check.loop_device": {
		LangZH: "Loop 设备",
		LangEN: "Loop Device",
	},
	"check.loop_ok": {
		LangZH: "支持 loop 设备",
		LangEN: "Loop device supported",
	},
	"check.loop_fail": {
		LangZH: "未检测到 loop 设备支持",
		LangEN: "Loop device support not detected",
	},
	"check.bind_mount": {
		LangZH: "支持 bind mount",
		LangEN: "Bind mount supported",
	},
	"check.bind_mount_fail": {
		LangZH: "未检测到 bind mount 支持",
		LangEN: "Bind mount support not detected",
	},
	"check.proc_mount": {
		LangZH: "proc 挂载",
		LangEN: "proc Mount",
	},
	"check.proc_mount_ok": {
		LangZH: "/proc 可用",
		LangEN: "/proc available",
	},
	"check.proc_mount_fail": {
		LangZH: "/proc 不可用",
		LangEN: "/proc unavailable",
	},
	"check.dev_nodes": {
		LangZH: "设备节点",
		LangEN: "Device Nodes",
	},
	"check.dev_nodes_found": {
		LangZH: "找到 %d/%d 个必要设备",
		LangEN: "Found %d/%d required devices",
	},
	"check.dev_nodes_partial": {
		LangZH: "仅找到 %d/%d 个必要设备",
		LangEN: "Only found %d/%d required devices",
	},
	"check.overlay": {
		LangZH: "支持 OverlayFS",
		LangEN: "OverlayFS supported",
	},
	"check.overlay_fail": {
		LangZH: "未检测到 OverlayFS 支持",
		LangEN: "OverlayFS support not detected",
	},
	"check.tmp_writable": {
		LangZH: "临时目录可写",
		LangEN: "Temp directory writable",
	},
	"check.sys_libs": {
		LangZH: "系统库",
		LangEN: "System Libraries",
	},
	"check.sys_libs_found": {
		LangZH: "找到 %d 个",
		LangEN: "Found %d",
	},
	"check.sys_libs_missing": {
		LangZH: "未找到必要的系统库",
		LangEN: "Required system libraries not found",
	},
	"check.user_groups": {
		LangZH: "用户组",
		LangEN: "User Groups",
	},
	"check.group_count": {
		LangZH: "%d 个组",
		LangEN: "%d groups",
	},
	"check.has_usrlib32": {
		LangZH: "支持 (有 /usr/lib32)",
		LangEN: "Supported (has /usr/lib32)",
	},
	"check.sys_arch": {
		LangZH: "系统架构",
		LangEN: "System Architecture",
	},
	"check.storage_msg": {
		LangZH: "可用 %.2f GB / 总计 %.2f GB",
		LangEN: "%.2f GB free / %.2f GB total",
	},
	"check.pkg_manager": {
		LangZH: "包管理器",
		LangEN: "Package Manager",
	},
	"check.need_root": {
		LangZH: "需要 root 权限检测",
		LangEN: "Root required to check",
	},
	"check.tmp_dir": {
		LangZH: "临时目录",
		LangEN: "Temp Directory",
	},
	"check.writable": {
		LangZH: "可写",
		LangEN: "Writable",
	},
	"check.container": {
		LangZH: "容器",
		LangEN: "Container",
	},
	"check.full_support_msg": {
		LangZH: "您的设备完全支持创建 Linux Rootfs 文件系统！",
		LangEN: "Your device fully supports Linux Rootfs!",
	},
	"check.proot_limited_hint": {
		LangZH: "proot 模式可能受限，建议检查失败项。",
		LangEN: "Proot mode may be limited, check failed items.",
	},
	"check.chroot_limited_hint": {
		LangZH: "chroot 模式可能受限，建议检查失败项。",
		LangEN: "Chroot mode may be limited, check failed items.",
	},
	"check.partial_limited_hint": {
		LangZH: "部分功能可能受限，建议使用 proot 模式。",
		LangEN: "Some features may be limited, consider proot mode.",
	},
	"check.desc_tar": {
		LangZH: "用于解压 rootfs",
		LangEN: "For extracting rootfs",
	},
	"check.desc_wget": {
		LangZH: "用于下载 rootfs 镜像",
		LangEN: "For downloading rootfs images",
	},
	"check.desc_curl": {
		LangZH: "用于下载 rootfs 镜像",
		LangEN: "For downloading rootfs images",
	},
	"check.desc_proot": {
		LangZH: "用于非 root 模式运行",
		LangEN: "Required for proot mode",
	},
	"check.desc_chroot": {
		LangZH: "用于 root 模式运行",
		LangEN: "Required for chroot mode",
	},

	// ==========================================
	// UserCheck (internal/usercheck/usercheck.go)
	// ==========================================

	"usercheck.no_passwd": {
		LangZH: "找不到 passwd 文件: %v",
		LangEN: "passwd file not found: %v",
	},
	"usercheck.read_passwd_fail": {
		LangZH: "无法读取 passwd 文件: %v",
		LangEN: "Cannot read passwd file: %v",
	},
	"usercheck.parse_passwd_fail": {
		LangZH: "读取 passwd 出错: %v",
		LangEN: "Error reading passwd: %v",
	},
	"usercheck.user_not_exist": {
		LangZH: "用户 \"%s\" 在 rootfs 中不存在",
		LangEN: "User \"%s\" does not exist in rootfs",
	},
	"usercheck.warn_sudo_nosuid": {
		LangZH: "警告: /bin/sudo 没有 setuid 位",
		LangEN: "Warning: /bin/sudo missing setuid bit",
	},
	"usercheck.warn_sudo_fix": {
		LangZH: "请在 root 权限下修复: chmod 4755 /bin/sudo",
		LangEN: "Fix with root: chmod 4755 /bin/sudo",
	},
	"usercheck.warn_shadow_wide": {
		LangZH: "警告: /etc/shadow 权限过宽",
		LangEN: "Warning: /etc/shadow permissions too wide",
	},
	"usercheck.warn_shadow_fix": {
		LangZH: "建议: chmod 0400 /etc/shadow",
		LangEN: "Suggestion: chmod 0400 /etc/shadow",
	},
	"usercheck.warn_nsswitch": {
		LangZH: "警告: /etc/nsswitch.conf 文件不存在",
		LangEN: "Warning: /etc/nsswitch.conf not found",
	},
	"usercheck.warn_nsswitch_fix": {
		LangZH: "建议: 确保 rootfs 完整",
		LangEN: "Suggestion: ensure rootfs is complete",
	},
	"usercheck.start_fix": {
		LangZH: "========== 开始修复权限 ==========",
		LangEN: "========== Starting permission fix ==========",
	},
	"usercheck.end_fix": {
		LangZH: "========== 权限修复完成 ==========",
		LangEN: "========== Permission fix complete ==========",
	},

	// ==========================================
	// Images (internal/images/images.go)
	// ==========================================

	"images.embed_fail": {
		LangZH: "无法读取嵌入的配置文件: %v",
		LangEN: "Cannot read embedded config file: %v",
	},
	"images.list_title": {
		LangZH: "可用的镜像列表：",
		LangEN: "Available images:",
	},
	"images.arch": {
		LangZH: "架构: %s",
		LangEN: "Arch: %s",
	},
	"images.version": {
		LangZH: "版本: %s",
		LangEN: "Version: %s",
	},
	"images.kali_visit": {
		LangZH: "访问 https://old.kali.org/nethunter-images/ 下载",
		LangEN: "Visit https://old.kali.org/nethunter-images/ to download",
	},
	"images.wget_not_found": {
		LangZH: "未找到可用的浏览器打开工具（尝试 xdg-open、open、termux-open-url）",
		LangEN: "No browser opener found (tried xdg-open, open, termux-open-url)",
	},
	"images.wget_install": {
		LangZH: "检测到系统未安装wget，正在安装...",
		LangEN: "wget not found, installing...",
	},
	"images.wget_install_fail_pkg": {
		LangZH: "无法检测到系统包管理器，请手动安装wget",
		LangEN: "Cannot detect package manager, please install wget manually",
	},
	"images.wget_install_exec_fail": {
		LangZH: "无法执行 %s: %v",
		LangEN: "Cannot execute %s: %v",
	},
	"images.wget_install_fail": {
		LangZH: "安装wget失败: %v",
		LangEN: "Failed to install wget: %v",
	},
	"images.wget_installed": {
		LangZH: "wget安装成功！",
		LangEN: "wget installed successfully!",
	},
	"images.wget_fallback": {
		LangZH: "无法使用wget，将使用默认下载方式...",
		LangEN: "Cannot use wget, using default download method...",
	},
	"images.download_fail": {
		LangZH: "下载失败: %s",
		LangEN: "Download failed: %s",
	},
	"images.downloading": {
		LangZH: "正在下载: %s -> %s\n",
		LangEN: "Downloading: %s -> %s\n",
	},
	"images.download_done": {
		LangZH: "下载完成: %s\n",
		LangEN: "Download complete: %s\n",
	},
	"images.download_all_start": {
		LangZH: "开始下载所有镜像...",
		LangEN: "Starting to download all images...",
	},
	"images.download_all_done": {
		LangZH: "所有镜像下载完成！",
		LangEN: "All images downloaded!",
	},
	"images.deprecated": {
		LangZH: "此函数已废弃，请使用交互式下载或直接指定完整的下载路径",
		LangEN: "This function is deprecated, use interactive download or specify full download path",
	},

	// ==========================================
	// Rootfs Make (internal/images/rootfs-make.go)
	// ==========================================

	"make.debootstrap_not_found": {
		LangZH: "debootstrap 未找到: %v",
		LangEN: "debootstrap not found: %v",
	},
	"make.bash_not_found": {
		LangZH: "bash 未找到: %v",
		LangEN: "bash not found: %v",
	},
	"make.pacstrap_not_found": {
		LangZH: "pacstrap 未找到: %v",
		LangEN: "pacstrap not found: %v",
	},
	"make.apt_not_found": {
		LangZH: "apt 未找到: %v",
		LangEN: "apt not found: %v",
	},
	"make.pacman_not_found": {
		LangZH: "pacman 未找到: %v",
		LangEN: "pacman not found: %v",
	},

	// ==========================================
	// Distro (internal/distro/distro.go)
	// ==========================================

	"distro.detect_fail": {
		LangZH: "无法检测发行版",
		LangEN: "Cannot detect distribution",
	},
	"distro.unsupported": {
		LangZH: "不支持的发行版: %s",
		LangEN: "Unsupported distribution: %s",
	},
	"distro.detecting": {
		LangZH: "正在检测发行版...",
		LangEN: "Detecting distribution...",
	},
	"distro.detected": {
		LangZH: "检测到发行版: %s (%s)",
		LangEN: "Detected distribution: %s (%s)",
	},
	"distro.deps_installed": {
		LangZH: "所需的依赖已经全部安装：chroot 和 proot",
		LangEN: "All required dependencies installed: chroot and proot",
	},
	"distro.chroot_installed_proot_missing": {
		LangZH: "chroot 已安装，proot 缺失",
		LangEN: "chroot installed, proot missing",
	},
	"distro.proot_installed_chroot_missing": {
		LangZH: "proot 已安装，chroot 缺失",
		LangEN: "proot installed, chroot missing",
	},
	"distro.installing_with": {
		LangZH: "使用 %s 进行安装...",
		LangEN: "Installing with %s...",
	},
	"distro.need_root": {
		LangZH: "需要 root 权限，请使用 sudo",
		LangEN: "Root privileges required, please use sudo",
	},
	"distro.apt_update_fail": {
		LangZH: "apt update 失败，继续安装: %v",
		LangEN: "apt update failed, continuing: %v",
	},
	"distro.fedora_try_direct": {
		LangZH: "Fedora 检测到，尝试直接安装 proot...",
		LangEN: "Fedora detected, trying direct proot install...",
	},
	"distro.fedora_success": {
		LangZH: "Fedora 上成功安装 proot",
		LangEN: "proot installed successfully on Fedora",
	},
	"distro.fedora_fallback": {
		LangZH: "Fedora 直接安装失败，回退到源码编译: %v",
		LangEN: "Fedora direct install failed, falling back to source build: %v",
	},
	"distro.installing_deps": {
		LangZH: "正在安装开发依赖...",
		LangEN: "Installing build dependencies...",
	},
	"distro.cloning_proot": {
		LangZH: "正在克隆 proot 源码...",
		LangEN: "Cloning proot source...",
	},
	"distro.git_not_found": {
		LangZH: "git 未找到: %v",
		LangEN: "git not found: %v",
	},
	"distro.building_proot": {
		LangZH: "正在编译 proot...",
		LangEN: "Building proot...",
	},
	"distro.make_not_found": {
		LangZH: "make 未找到: %v",
		LangEN: "make not found: %v",
	},
	"distro.installing_proot": {
		LangZH: "正在安装 proot...",
		LangEN: "Installing proot...",
	},
	"distro.proot_installed": {
		LangZH: "proot 成功安装!",
		LangEN: "proot installed successfully!",
	},
	"distro.freebsd_chroot": {
		LangZH: "FreeBSD: chroot 已默认安装",
		LangEN: "FreeBSD: chroot is installed by default",
	},
	"distro.freebsd_noproot": {
		LangZH: "FreeBSD: Linux 版 proot 无法在 FreeBSD 上运行",
		LangEN: "FreeBSD: Linux proot cannot run on FreeBSD",
	},
	"distro.freebsd_hint": {
		LangZH: "FreeBSD: 可以使用原生的 chroot 或 jail 替代",
		LangEN: "FreeBSD: Use native chroot or jail instead",
	},
	"distro.netbsd_chroot": {
		LangZH: "NetBSD: chroot 已默认安装",
		LangEN: "NetBSD: chroot is installed by default",
	},
	"distro.netbsd_noproot": {
		LangZH: "NetBSD: Linux 版 proot 无法在 NetBSD 上运行",
		LangEN: "NetBSD: Linux proot cannot run on NetBSD",
	},
	"distro.netbsd_hint": {
		LangZH: "NetBSD: 可以使用原生的 chroot 替代",
		LangEN: "NetBSD: Use native chroot instead",
	},

	// ==========================================
	// Network (internal/network/network.go)
	// ==========================================

	"network.configuring_ns": {
		LangZH: "在子进程网络命名空间中配置网络...",
		LangEN: "Configuring network in child namespace...",
	},
	"network.veth_fail": {
		LangZH: "设置 veth 对失败: %v",
		LangEN: "Failed to setup veth pair: %v",
	},
	"network.host_fail": {
		LangZH: "配置主机端网络失败: %v",
		LangEN: "Failed to configure host network: %v",
	},
	"network.guest_fail": {
		LangZH: "配置容器端网络失败: %v",
		LangEN: "Failed to configure guest network: %v",
	},
	"network.nat_fail": {
		LangZH: "设置 NAT 失败: %v",
		LangEN: "Failed to setup NAT: %v",
	},
	"network.config_done": {
		LangZH: "网络配置完成",
		LangEN: "Network configuration complete",
	},
	"network.cmd_exec": {
		LangZH: "执行: %s",
		LangEN: "Executing: %s",
	},
	"network.cmd_fail": {
		LangZH: "命令失败: %s, 输出: %s",
		LangEN: "Command failed: %s, output: %s",
	},
	"network.create_dir_fail": {
		LangZH: "创建目录失败",
		LangEN: "Failed to create directory",
	},
	"network.write_resolv_fail": {
		LangZH: "写入 resolv.conf 失败",
		LangEN: "Failed to write resolv.conf",
	},
	"network.cleanup_host": {
		LangZH: "清理主机端网络资源...",
		LangEN: "Cleaning up host network resources...",
	},
	"network.cleanup_done": {
		LangZH: "主机端网络清理完成",
		LangEN: "Host network cleanup complete",
	},
	"network.cleanup_container": {
		LangZH: "清理容器 %d 的网络资源...",
		LangEN: "Cleaning container %d network resources...",
	},
	"network.cleanup_container_done": {
		LangZH: "容器 %d 的网络资源清理完成",
		LangEN: "Container %d network cleanup complete",
	},
	"network.detect_orphan": {
		LangZH: "检测孤立网络资源...",
		LangEN: "Detecting orphaned network resources...",
	},
	"network.orphan_done": {
		LangZH: "孤立网络资源清理完成",
		LangEN: "Orphaned network cleanup complete",
	},
	"network.no_ip_forward": {
		LangZH: "IP 转发未启用，尝试启用...",
		LangEN: "IP forwarding not enabled, attempting to enable...",
	},
	"network.ip_forward_fail": {
		LangZH: "启用 IP 转发失败: %v",
		LangEN: "Failed to enable IP forwarding: %v",
	},

	// ==========================================
	// Cleanup (internal/cleanup/cleanup.go)
	// ==========================================

	"cleanup.cleaning": {
		LangZH: "正在清理 rootfs 中的宿主机环境",
		LangEN: "Cleaning host environment traces from rootfs",
	},
	"cleanup.hosts_fail": {
		LangZH: "清理 /etc/hosts 失败",
		LangEN: "Failed to clean /etc/hosts",
	},
	"cleanup.done": {
		LangZH: "宿主机环境清理完成",
		LangEN: "Host environment cleanup complete",
	},

	// ==========================================
	// Security (internal/security/security.go)
	// ==========================================

	"security.hardening_procfs": {
		LangZH: "正在硬化 procfs: %s",
		LangEN: "Hardening procfs: %s",
	},
	"security.procfs_dir_fail": {
		LangZH: "创建 proc/sys 目录失败",
		LangEN: "Failed to create proc/sys directory",
	},
	"security.procfs_remount_fail": {
		LangZH: "重新挂载 procfs 失败",
		LangEN: "Failed to remount procfs",
	},
	"security.procfs_mount_fail": {
		LangZH: "挂载硬化 procfs 失败",
		LangEN: "Failed to mount hardened procfs",
	},
	"security.procfs_done": {
		LangZH: "procfs 已成功硬化挂载",
		LangEN: "procfs hardened successfully",
	},
	"security.hide_fail": {
		LangZH: "隐藏敏感文件 %s 失败",
		LangEN: "Failed to hide sensitive file %s",
	},
	"security.hide_ok": {
		LangZH: "成功隐藏敏感文件: %s",
		LangEN: "Successfully hidden sensitive file: %s",
	},
	"security.prctl_fail": {
		LangZH: "prctl PR_SET_DUMPABLE 失败",
		LangEN: "prctl PR_SET_DUMPABLE failed",
	},
	"security.ptrace_fail": {
		LangZH: "prctl PR_SET_PTRACER 失败",
		LangEN: "prctl PR_SET_PTRACER failed",
	},
	"security.yama_fail": {
		LangZH: "设置 Yama ptrace_scope 失败",
		LangEN: "Failed to set Yama ptrace_scope",
	},
	"security.drop_caps": {
		LangZH: "尝试丢弃危险 capabilities",
		LangEN: "Attempting to drop dangerous capabilities",
	},
	"security.drop_cap_fail": {
		LangZH: "丢弃 capability %d 失败",
		LangEN: "Failed to drop capability %d",
	},
	"security.applying": {
		LangZH: "正在应用安全限制...",
		LangEN: "Applying security restrictions...",
	},
	"security.ptrace_disable_fail": {
		LangZH: "禁用 ptrace 失败",
		LangEN: "Failed to disable ptrace",
	},

	// ==========================================
	// Termux (internal/termux/termux.go)
	// ==========================================

	"termux.ld_preload_clean": {
		LangZH: "Termux 检测到 LD_PRELOAD=%s，正在清理",
		LangEN: "Termux detected LD_PRELOAD=%s, cleaning up",
	},
	"termux.proot_not_installed": {
		LangZH: "系统未安装 proot，请先安装：sudo apt install proot",
		LangEN: "proot not installed, please install: sudo apt install proot",
	},
	"termux.proot_installing": {
		LangZH: "Termux 中未找到 proot，正在自动安装...",
		LangEN: "proot not found in Termux, installing automatically...",
	},
	"termux.no_pkg": {
		LangZH: "未找到 pkg 包管理器",
		LangEN: "pkg package manager not found",
	},
	"termux.proot_install_fail": {
		LangZH: "自动安装 proot 失败",
		LangEN: "Failed to install proot automatically",
	},
	"termux.proot_installed": {
		LangZH: "proot 安装成功",
		LangEN: "proot installed successfully",
	},
	"termux.chroot_not_installed": {
		LangZH: "系统未安装 chroot，请先安装 coreutils",
		LangEN: "chroot not installed, please install coreutils",
	},
	"termux.chroot_installing": {
		LangZH: "Termux 中未找到 chroot，正在自动安装 proot...",
		LangEN: "chroot not found in Termux, installing proot...",
	},
	"termux.lookpath_not_found": {
		LangZH: "%s: 未找到或不可执行",
		LangEN: "%s: not found or not executable",
	},
	"termux.lookpath_empty_path": {
		LangZH: "%s: PATH 为空",
		LangEN: "%s: PATH is empty",
	},
	"termux.lookpath_not_in_path": {
		LangZH: "%s: 在 PATH 中未找到",
		LangEN: "%s: not found in PATH",
	},
	"termux.empty_args": {
		LangZH: "CommandSlice: 空参数",
		LangEN: "CommandSlice: empty arguments",
	},

	// ==========================================
	// Alpine Proot (internal/proot/alpine-proot.go) - additional
	// ==========================================

	"alpine.not_alpine": {
		LangZH: "这看起来不是 alpine，不过继续尝试",
		LangEN: "This doesn't look like alpine, but continuing anyway",
	},
	"alpine.cannot_read_passwd": {
		LangZH: "无法读取 passwd 文件，使用默认用户信息: %v",
		LangEN: "Cannot read passwd file, using default user info: %v",
	},
	"alpine.fix_rootfs_perm": {
		LangZH: "终极修复 rootfs 权限为当前用户",
		LangEN: "Fixing rootfs permissions for current user",
	},
	"alpine.cleanup_fail": {
		LangZH: "清理 rootfs 环境失败: %v",
		LangEN: "Failed to clean rootfs environment: %v",
	},

	// ==========================================
	// Permission (internal/permission/permission.go)
	// ==========================================

	// (mostly comments, no user-facing strings)

	// ==========================================
	// Env (internal/env/env.go)
	// ==========================================

	"env.loaded": {
		LangZH: "环境变量加载完成，共 %d 个",
		LangEN: "Environment variables loaded, total %d",
	},

	// ==========================================
	// 补充缺失的 key
	// ==========================================

	// main.go
	"cli.config_load_fail": {
		LangZH: "加载配置失败: %v",
		LangEN: "Failed to load config: %v",
	},
	"cli.browser_not_found": {
		LangZH: "未找到可用的浏览器打开工具（尝试 xdg-open、open、termux-open-url）",
		LangEN: "No browser opener found (tried xdg-open, open, termux-open-url)",
	},
	"cli.dl_mode_header": {
		LangZH: "=== download 方式（pm download） ===",
		LangEN: "=== download mode (pm download) ===",
	},
	"cli.make_mode_header": {
		LangZH: "=== make 方式（pm make） ===",
		LangEN: "=== make mode (pm make) ===",
	},
	"cli.dl_mode_table": {
		LangZH: "| download 方式（pm download） |",
		LangEN: "| download mode (pm download) |",
	},
	"cli.make_mode_table": {
		LangZH: "| make 方式（pm make）         |",
		LangEN: "| make mode (pm make)          |",
	},
	"cli.make_arch_hint": {
		LangZH: "  Arch 构建仅在 Arch Linux 系统有效",
		LangEN: "  Arch build only works on Arch Linux",
	},

	// rootfs-make.go
	"make.whiptail_not_found": {
		LangZH: "提示: whiptail 没有找到，尝试安装它以获得更好的交互体验...",
		LangEN: "Hint: whiptail not found, trying to install for better UI...",
	},
	"make.detect_distro": {
		LangZH: "检测到当前系统是: %s",
		LangEN: "Detected system: %s",
	},
	"make.xbps_installing": {
		LangZH: "找到 xbps-install，正在安装 newt 包...",
		LangEN: "Found xbps-install, installing newt package...",
	},
	"make.newt_install_fail": {
		LangZH: "安装 newt 失败: %v",
		LangEN: "Failed to install newt: %v",
	},
	"make.newt_installed_check": {
		LangZH: "安装 newt 成功，检查 whiptail 是否存在...",
		LangEN: "newt installed, checking whiptail...",
	},
	"make.whiptail_found": {
		LangZH: "找到 whiptail，启动交互式菜单...",
		LangEN: "Found whiptail, launching interactive menu...",
	},
	"make.whiptail_still_missing": {
		LangZH: "whiptail 仍然未找到: %v",
		LangEN: "whiptail still not found: %v",
	},
	"make.xbps_not_found": {
		LangZH: "未找到 xbps-install: %v",
		LangEN: "xbps-install not found: %v",
	},
	"make.text_mode": {
		LangZH: "将使用文本交互模式",
		LangEN: "Using text interactive mode",
	},

	// network.go
	"network.cmd_exec_fail": {
		LangZH: "执行 %s 失败: %v",
		LangEN: "Command %s failed: %v",
	},
	"network.no_ip_forward_file": {
		LangZH: "未找到 /proc/sys/net/ipv4/ip_forward",
		LangEN: "/proc/sys/net/ipv4/ip_forward not found",
	},
	"network.no_ip_cmd": {
		LangZH: "未找到 ip 命令",
		LangEN: "ip command not found",
	},
	"network.no_nsenter_cmd": {
		LangZH: "未找到 nsenter 命令",
		LangEN: "nsenter command not found",
	},

	// cleanup.go
	"cleanup.resolv_conf_fail": {
		LangZH: "清理 /etc/resolv.conf 失败: %v",
		LangEN: "Failed to clean /etc/resolv.conf: %v",
	},
	"cleanup.hostname_fail": {
		LangZH: "处理 /etc/hostname 失败: %v",
		LangEN: "Failed to handle /etc/hostname: %v",
	},
	"cleanup.machine_id_fail": {
		LangZH: "清理 /etc/machine-id 失败: %v",
		LangEN: "Failed to clean /etc/machine-id: %v",
	},
}
