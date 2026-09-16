# Changelog

本文档记录 litevm 的所有版本更新日志。

## V1.2 (2026-9-16) — 当前版本

### 🆕 全新功能

- **多块设备支持**：`vmm run` 新增 `--drive` 参数，可附加多个额外块设备
  - 格式：`--drive path[:id][:ro]`，支持重复使用
  - 支持 raw、qcow2、vmdk、vhd、iso 格式，非 raw 格式自动通过 `qemu-img convert` 转换
  - 示例：`--drive /data/disk.qcow2:mydata:ro`
- **ISO 光盘挂载**：新增 `--cdrom` 参数，将 ISO 镜像作为只读块设备挂载
  - 自动检测 ISO 9660 格式
  - 示例：`--cdrom /path/to/install.iso`
- **虚拟磁盘挂载**：新增 `--hd` 参数，将已有的虚拟磁盘文件作为额外块设备挂载
  - Linux 内不自动挂载，需通过 fstab 决定挂载行为
  - 示例：`--hd /path/to/data.ext4`
- **Balloon 内存气球**：新增 `--balloon` 参数，启用 virtio-balloon 设备动态调整内存
  - 配合 `--balloon-deflate-on-oom` 在 guest OOM 时自动 deflate
  - 示例：`--balloon 256 --balloon-deflate-on-oom`
- **Vsock 通信**：新增 `--vsock` 参数，启用 virtio-vsock host-guest 通信通道
  - 格式：`--vsock cid` 或 `--vsock cid:uds_path`
  - 示例：`--vsock 3` 或 `--vsock 5:/run/litevm/vsock.sock`
- **磁盘格式自动检测**：通过文件扩展名和 magic bytes 自动识别 raw/qcow2/vmdk/vhd/iso 格式

### 🔧 修复与优化

- **修复 `litevm vmm run` 无参数帮助信息**：无参数时正确显示完整的命令帮助（所有可用选项），与 `--help` 输出一致
- **显示 init 启动日志**：移除默认内核参数中的 `quiet`，将 `loglevel=3` 改为 `loglevel=5`，使 systemd/openrc/runit 等 init 系统的启动日志在串口可见
- **修复 download.go vet 警告**：修正 `fmt.Printf` 中非恒定格式字符串的问题
- **i18n 国际化**：新增 VMM 设备相关的中英文翻译消息

---

## V1.1 (2026-9-15)

### 🔧 修复与优化

- **VMM 默认内存从 256MB 调整为 1GB**：默认内存过小导致多数 Linux 发行版无法正常运行，现默认 1024MB
  - `DefaultConfig()` `MemSizeMB` 从 256 改为 1024
  - CLI `--mem` flag 默认值同步更新
- **减少 VMM 内核启动噪声**：默认内核启动参数追加 `quiet loglevel=3`，抑制启动时大量内核日志输出到串口
- **chroot 模式新增标准 Linux 虚拟设备节点**：
  - 虚拟终端：`/dev/tty0` ~ `/dev/tty63`
  - 串口设备：`/dev/ttyS0` ~ `/dev/ttyS3`
  - ARM 串口：`/dev/ttyAMA0`、`/dev/ttyAMA1`
  - 虚拟串口：`/dev/ttyV0` ~ `/dev/ttyV3`
  - virtio 虚拟控制台：`/dev/hvc0` ~ `/dev/hvc3`
  - Xen 虚拟控制台：`/dev/xvc0`、`/dev/uvhvc0`

---

## V1.0 (2026-9-14)

### 🆕 全新功能

- **VMM 环境检查**：`--check` 命令新增 VMM 模式检查，检测 KVM 支持和 firecracker 安装情况
  - `./litevm --check vmm` 单独检查 VMM 环境
  - `./litevm --check` 全量检查（proot + chroot + VMM）
- **Distro 适配网络**：`setup-network` 自动检测发行版，采用最佳持久化方式
  - Arch: systemd 服务管理 TAP 生命周期
  - Debian: systemd-networkd + NetworkManager dispatcher
  - RHEL/Fedora: NetworkManager + firewalld/iptables

### 🔧 修复与优化

- **修复 setup-network 端口冲突**：dnsmasq 由 systemd 服务统一管理，避免与 `startDHCP()` 端口 67 冲突
- **修复 VMM 关机后卡死**：guest 内执行 `poweroff`/`halt` 后，Firecracker 进程不会自行退出（已知限制），`litevm vmm run` 会一直卡在内核停机画面，无法回到宿主 shell。现在 litevm 会监视串口输出中的关机标记（`reboot: System halted`/`reboot: Power down` 等，对 systemd/openrc/runit 通用），检测到 guest 关机后自动停止并退出 microVM，恢复正常返回宿主 shell
- **VMM 支持 reboot 正常重启**：guest 内执行 `reboot` 时内核会 reset CPU 导致 Firecracker 进程退出（Firecracker 已知行为），`litevm vmm run` 之前会因此直接结束。现在 litevm 会识别重启标记（`reboot: Restarting system` 等）并自动以相同配置重新拉起 microVM，让 `reboot` 表现为主机正常重启；`poweroff` 仍然退出
- **修复 dnsmasq daemonization**：服务文件添加 `--no-daemon`，避免 Type=simple 下 dnsmasq fork 后退出
- **修复 iptables 失败终止服务**：ExecStartPost iptables 规则添加失败不杀死 dnsmasq
- **修复 ExecStopPost shell 转义**：iptables `-` 前缀需用 `--` 分隔
- **TAP 设备生命周期管理**：systemd 服务管理完整 TAP 生命周期（创建、IP、dnsmasq、NAT）
- **TAP 设备清理保护**：`CleanupTapDevice` 在服务活跃时跳过清理
- **i18n 消息清理**：移除消息值中的格式动词，统一 `fmt.Errorf("%s: %w")` 模式
- **项目更名**：正式从 `groot` 更名为 `litevm`

---

## V0.4.0 (2026-9-13)

### 🆕 全新功能

- **Firecracker VMM 支持**：新增 `vmm` 子命令，使用 Firecracker 启动轻量级 microVM
  - 支持自定义内存、CPU、网络配置
  - TAP 设备 + NAT 网络，支持 DHCP 和 SSH 访问
  - 从 Firecracker S3 下载官方内核，交互式选择（进度条 + 多选）
  - 支持 x86_64 和 aarch64 双架构
  - 一键下载所有可用内核
  - VMM 命令重构为子命令：`run`、`download-kernel`、`setup-network`、`rm-network`
- **VMM 网络配置**：新增 `litevm vmm setup-network` 和 `litevm vmm rm-network` 命令
  - `setup-network`：一次性配置 TAP 设备、NAT、/dev/kvm 权限
  - `rm-network`：清除网络节点和相关配置
  - 支持无 root 运行：配置完成后日常使用无需 sudo
- **新增 `--kernel-args` 参数**：自定义内核启动参数，覆盖默认值
  - 支持 systemd、openrc、runit 等 init 系统
  - 例：`--kernel-args "console=ttyS0,115200n8 reboot=k panic=1 nomodule systemd.unit=multi-user.target"`
- **智能 init 系统检测**：rootfs 的 `/sbin/init` 自动检测并执行可用的 init 系统
  - 支持 openrc-init、systemd、runit、openrc
  - 无需手动配置内核参数
- **SSH 访问**：VM 启动后自动检测并启动 SSH 服务，支持 `ssh root@<guest-ip>` 登录

### 🔧 修复与优化

- **修复 setup-network 未启动 DHCP 服务器**：`setup-network` 命令现在自动启动 dnsmasq，VM 可通过 DHCP 获取 IP
- **修复 SetupTapDevice 跳过 dnsmasq**：TAP 已存在时仍确保 dnsmasq 在运行，支持 `--tap` 预创建 TAP 设备
- **新增 `dnsmasqIsRunning()`**：通过 PID 文件检查 dnsmasq 是否已运行，避免重复启动
- **修复 kernel download 显示问题**：进度条 `[====] 42%` 正确显示
- **修复 host interface 检测**：`FindHostInterface` 排除 `litevm-tap*` 前缀的 TAP 设备
- **修复 sudo 非交互执行**：所有网络命令采用直接执行-降级 sudo 模式
- **修复 iptables 规则添加失败**：`|| true` 允许规则已存在时继续
- **移除 logger.Debug 吞掉错误**：改为 `fmt.Fprintf(os.Stderr, ...)` 直接输出
- **修复 runtime.GOARCH 到 S3 arch 映射**：正确映射 `amd64` → `x86_64`
- **TAP 设备复用检测**：启动时检测 TAP 是否已存在，避免重复创建

---

## V0.3.2.3 (2026-9-13)

### 🔧 修复与优化

- **修复 fish shell 启动失败**：fish 不支持 bash 语法的 `if [ ... ]; then ... fi`，添加非 POSIX shell 检测（fish/csh/tcsh/ksh），跳过 bash 特定的 fixScript 和 pacmanWrapper
- **修复 chroot 命令注入风险**：添加 `validateShellPath()` 正则验证 + `sanitizeShellArg()` 清理参数
- **修复 proot 命令注入风险**：添加 `validateShellPath()` 白名单验证 shell 路径
- **修复 proot 路径穿越风险**：添加 `..` 检查，防止 `../../etc/shadow` 类攻击逃逸出 rootfs
- **修复 ptrace 数据竞争**：`tracee.Exited` 从 `bool` 改用 `atomic.Bool`，消除 goroutine 间的数据竞争
- **修复 binding.go 路径匹配逻辑错误**：`/` 继承绑定不再优先匹配所有路径，改为按 GuestPath 长度降序排序，优先匹配最长前缀
- **修复 SIGTRAP 后信号传递错误**：处理 SIGTRAP 后传递 `signal 0` 而非 `SIGTRAP` 自身，避免进程收到额外信号
- **修复 signal.Stop 执行时机**：使用 `defer signal.Stop()` + `defer close()` 确保清理一定执行
- **修复网络清理重复调用**：合并为一处，用 `defer` 确保执行
- **修复 network sleep 竞态**：改为轮询 `/proc/<pid>/ns/net` 等待网络命名空间就绪
- **修复 Mknod 返回值忽略**：添加日志记录设备节点创建失败
- **修复 os.Chown/Chmod 返回值忽略**：添加 `logger.Debug()` 记录权限修改失败
- **添加 hostname 长度验证**：截断到内核限制 63 字节
- **删除 binding.go 死代码**：移除未使用的 `cleanPath()` 和 `isPrefix()` 函数
- **统一路径拼接**：chroot.go 中统一使用 `filepath.Join()` 替代字符串拼接
- **添加 SIGWINCH 信号转发**：终端窗口大小改变时正确转发给子进程

---

## V0.3.2.2 (2026-9-12)

### 🌍 国际化 (i18n)

- **新增 i18n 国际化支持**：所有用户可见的字符串支持中文和英文
  - 新增 `internal/i18n/` 包，提供翻译框架和 200+ 翻译 key
  - 基于 LANG 环境变量自动检测语言，支持从配置文件、locale.conf、环境变量读取
  - Termux 环境自动读取 `~/.termux/locale.conf` 判断语言（`zh_CN.UTF-8` → 中文，其他 → 英文）
  - 新增 `--termux-lang` 参数，可随时唤起 TUI 切换 Termux 语言
  - 用户可通过 `~/.config/litevm/lang` 手动切换语言
- **替换所有源文件中的硬编码中文字符串**：
  - `cmd/litevm/main.go`：CLI 帮助文本、错误消息
  - `internal/chroot/chroot.go`：chroot 模式日志
  - `internal/proot/proot.go`：proot 模式日志
  - `internal/proot/alpine-proot.go`：Alpine 专属模式
  - `internal/mount/mount.go`：挂载/卸载日志
  - `internal/images/images.go`：镜像管理
  - `internal/distro/distro.go`：发行版检测
  - `internal/network/network.go`：网络配置
  - `internal/cleanup/cleanup.go`：清理
  - `internal/security/security.go`：安全限制
  - `internal/termux/termux.go`：Termux 适配
  - `internal/logger/logger.go`：日志 Banner
  - `internal/check/check.go`：设备检查报告
  - `internal/usercheck/usercheck.go`：用户权限检查
  - `internal/env/env.go`：环境变量加载

### 🔧 修复与优化

- **修复 rootfs 配置加载问题**：彻底移除所有硬编码环境变量，改为从 rootfs 配置文件自动加载
  - 移除硬编码：`LANG`、`LC_ALL`、`LC_CTYPE`、`EDITOR`、`VISUAL`、`PAGER`、`LESS`、`TMPDIR`、`MAIL`、`TZ`、`HISTFILE`、`HISTSIZE`、`HISTFILESIZE`、`XDG_*` 等
  - 移除所有发行版特定硬编码：`DEBIAN_FRONTEND`、`PACMAN`、`DNF`、`XBPS_*`、`SELINUX` 等
  - 现在仅设置 litevm 自身必需的变量：`HOME`、`USER`、`LOGNAME`、`SHELL`、`PWD`、`HOSTNAME`
  - 从 `/etc/locale.conf` 自动加载语言环境，并智能推导 `LC_ALL`、`LC_CTYPE`、`LANGUAGE`
  - 从 `/etc/environment` 自动加载系统级环境变量
  - shell 脚本（`/etc/profile`、`/etc/profile.d/*.sh`、`~/.bashrc`）由 login shell 自行 source，不再由 litevm 预解析
  - 修复了预解析 shell 脚本导致 `PS0`/`PS1` 等包含 shell 语法的变量被错误设为字面值的问题
- **修复 pacman wrapper 问题**：移除 wrapper 中硬编码的 `export LC_ALL=C`，允许用户 locale 设置生效
- **移除 pacman wrapper DEBUG 信息**：清理 `echo "DEBUG: ..."` 输出
- **改善卸载容错性**：`UnmountAll` 对 `EINVAL` 错误降级为 Debug 日志，命名空间销毁时自动清理

---

## V0.3.2 (2026-9-11)

### 🆕 全新功能

#### 1. chroot 网络隔离模式 (`--net` / `-net`)

新增 chroot 模式的独立网络命名空间支持，容器获得与宿主机完全隔离的网络栈。

**核心特性：**
- 使用 `clone(CLONE_NEWNET)` 创建独立的网络命名空间
- 容器内部拥有独立的网络接口、路由表、iptables 规则
- 宿主机与容器通过 `veth` 虚拟以太网对通信，容器分配 `10.0.0.2/24`，宿主机端 `10.0.0.1/24`
- 支持 DNS 配置，容器自动获得 `/etc/resolv.conf`（包含公共 DNS 服务器）
- 提供 NAT 转发能力，容器可以通过宿主机访问外部网络

**使用方式：**
```bash
# 启用网络隔离
sudo ./litevm -c /path/to/rootfs --net
# 或
sudo ./litevm -c /path/to/rootfs -net
```

**实现细节：**
- 在 `internal/chroot/chroot.go` 中添加 `netMode bool` 参数
- 在 `internal/network/network.go` 中实现完整的网络配置
- namespace 创建后等待 100ms 确保子进程已完成网络命名空间初始化
- 子进程退出时自动清理宿主机网络资源（veth、主机端 iptables 规则）

#### 2. LXC 容器管理子命令

新增 `lxc` 子命令，提供完整的 LXC 风格轻量级容器管理能力，实现对 rootfs 的完整隔离虚拟化。

**核心特性：**
- **完整的 namespace 隔离**：mount、PID、UTS、IPC、NET 五大命名空间完全隔离
- **守护进程模式**：容器 init 进程在后台运行，作为容器内的 PID 1，不依赖终端
- **自动挂载虚拟文件系统**：/proc、/sys、/dev、/dev/pts、/dev/shm、/tmp、/run 自动挂载到 rootfs 内
- **主机名隔离**：容器拥有独立的 UTS namespace，主机名不影响宿主机
- **就绪握手机制**：通过文件描述符管道（fd 3），父进程等待容器 init 就绪后再返回
- **容器配置持久化**：容器配置存储在 `/usr/local/litevm/lxc/containers/` 下的 JSON 文件中

**支持的子命令：**

| 子命令 | 说明 |
| ------ | ---- |
| `lxc ls` | 列出所有容器 |
| `lxc ps [名称]` | 查看容器内部进程 |
| `lxc [rootfs] [shell] -name [别名]` | 创建容器 |
| `lxc [名称] start` | 启动容器 |
| `lxc [名称]` | 登录容器 |
| `lxc [名称] stop` | 停止容器 |
| `lxc [名称] rm` | 删除容器 |
| `lxc [rootfs] [shell] -name [别名] start` | 创建并立即启动 |

### 🔧 修复与优化

- **修复 Alpine busybox 硬链接问题**：Alpine Linux 默认使用 busybox 硬链接（多个目录项共享同一 inode），在 Termux/proot 环境下文件系统不支持硬链接导致 "command not found" 错误。添加 `--link2symlink` 标志让 proot 把 link() 转换为 symlink()
- **修复 fixAlpineRootfs 重复 chmod/chown**：添加 inode 去重机制，跳过已处理的硬链接 inode，避免重复操作和权限覆盖
- **修复 fixUltimatePermissions 权限覆盖**：同样添加 inode 去重，防止硬链接文件的权限被非可执行目录的硬链接意外覆盖
- **修复 nsenter 参数语法错误**：nsenter 的 `--root` 和 `--wd` 是可选参数选项，必须使用等号语法 `--root=path`，否则 nsenter 会把路径当作程序执行
- **修复 shell-init getcwd 错误**：添加 `cd / 2>/dev/null` 包装命令，确保登录 shell 初始化时 CWD 已经是有效的 `/`
- **修复 shell 检测误判**：Alpine 的 `/bin/ash` 是绝对路径符号链接 → `/bin/busybox`。添加 `shellExistsInRootfs()` 函数，使用 `os.Lstat` 不跟踪链接，并对绝对路径符号链接在 rootfs 内部重新解析
- **新增 lxc.go**：LXC 容器创建、启动、停止、登录等完整管理功能
- **新增 network.go**：网络命名空间配置、veth 桥接、DNS 设置等网络支持功能
- **新增 check.go**：设备检查工具，检测 proot/chroot 环境兼容性
- **新增 alpine_proot_test.go**：硬链接处理的单元测试，验证 fixAlpineRootfs 和 fixUltimatePermissions 的 inode 去重逻辑

---

## V0.3.1 (2026-9-9)

### 🆕 全新功能

- **LXC 登录修复与终端安全增强**
  - **修复**: LXC 登录时 `shell-init: error retrieving current directory` 错误
    - 在 `LoginContainer` 中添加 `os.Chdir("/")` 确保登录前切换到安全目录
    - 使用 `env -i` 清除继承的环境变量，防止错误的 `PWD`/`OLDPWD` 传递给容器 shell
    - 使用包装命令 `sh -c 'cd / 2>/dev/null; exec bash -l'` 确保所有配置文件在正确目录下执行
  - **修复**: LXC stop 容器导致宿主机伪终端失效的严重问题
    - 使用 `defer signal.Reset(syscall.SIGINT)` 确保信号状态一定会被恢复
    - 移除 `cleanupNetworkInChild()` 中的 `ip link del lo` 危险命令，避免误删宿主机 loopback 接口
    - 添加网络清理超时机制，防止命令阻塞容器退出
  - **改进**: LXC 配置存储路径改为标准 Linux 路径
    - 从 Termux 特定路径 `/data/data/com.termux/files/usr/share/litevm/lxc` 改为 `/usr/local/litevm/lxc`
    - LXC 是标准 Linux 功能，配置应存储在标准位置
  - **修复**: 伪终端无法分配和桌面程序无法启动
    - 添加 `/dev/ptmx` 设备节点创建（符号链接到 `/dev/pts/ptmx`）
    - 添加 X11 socket 目录挂载 (`/tmp/.X11-unix`)，支持容器内 GUI 程序连接到宿主机显示服务器
  - **改进**: 简化登录命令结构，移除不必要的 wrapper 嵌套层
  - **改进**: 增强容器退出时的资源清理安全性

### 🔧 修复与优化

- **chroot 网络隔离模式**：使用 `clone(CLONE_NEWNET)` 创建独立的网络命名空间
- **LXC 容器管理子命令**：支持容器的创建、启动、停止、登录、删除、列举等操作

---

## V0.3.1 (2026-8-13)

### 🔧 修复与优化

- **修复**: Termux 下 `pm download` 仍崩溃（SIGSYS/faccessat2）的问题
  - 根因：`exec.Command("wget", ...)` 等命令名调用会在**内部再次隐式调用 `LookPath`**，即使此前已替换了显式的 `exec.LookPath`，仍会触发 `faccessat2` 被 seccomp 拦截导致 `SIGSYS`
  - 在 `internal/termux` 新增 `Command(name, args...)` 与 `CommandSlice(args[])` 安全封装，先经 `SafeLookPath` 解析绝对路径再执行，避免内部的 `LookPath` 调用
  - 将 `images.go` / `rootfs-make.go` / `main.go` / `distro.go` 中所有裸命令名 `exec.Command(...)` 改为安全封装（覆盖 `wget` / `apt` / `apk` / `xbps-install` / `debootstrap` / `pacstrap` / `bash` / `git` / `make` / `lsb_release` 等）
  - 绝对路径调用（`/system/bin/sh`、`/bin/sh`）及已解析路径的变量（`whiptailPath`、`prootPath` 等）保持不变
  - 已通过 `linux/amd64`、`linux/arm64`、`linux/arm`、`android/arm64` 交叉编译与 `go vet` 验证，跨平台行为一致
- **新增**: `--check` 参数，检查设备是否符合要求
  - 支持 `--check`、`--check proot`、`--check chroot` 三种模式
  - 检测设备信息（Termux/Android 版本、架构、CPU、内核、SELinux 等）
  - 检测必要命令是否存在（chroot、mount、umount、proot、wget 等）
  - 检测文件系统空间是否充足
  - 检测共享库是否齐全
  - 输出彩色检查报告，标记通过/失败项

---

## V0.3 (2026-7-25)

### 🆕 全新功能

- **Termux 环境检测与自动适配**（`internal/termux`）
  - Termux 环境自动清理 `LD_PRELOAD` 避免冲突
  - Android root 权限检测，无 root 自动提示使用 proot 模式
  - Termux 中 proot/chroot 未安装时自动 `pkg install -y proot`
  - 安全的 `SafeLookPath` 绕过 proot 嵌套中 `faccessat2` 被 seccomp 拦截的问题
- **宿主机环境自动清理**（`internal/cleanup`）
  - 清理 `/etc/hosts` 中的宿主机私有 IP 条目，保留 rootfs 本地回环
  - 清理 `/etc/resolv.conf` 中的宿主机私有 DNS
  - 保留 rootfs 自己的 hostname，仅在缺失时补充默认
  - 清空 machine-id、shell 历史、SSH known_hosts、/var/log、/tmp
  - chroot 与 proot 模式均自动执行
- **Profile 加载方式改进**
  - 两种模式统一使用 `exec shell -l`（login shell）方式启动
  - 自动加载 rootfs 自己的 `/etc/profile` 和 `/etc/profile.d/*.sh`
- **日志系统支持 ANSI 彩色输出**
  - `[DEBUG]` 蓝色、`[INFO]` 绿色、`[WARN]` 黄色、`[ERROR]` 红色
  - 非终端输出时自动禁用颜色
  - 默认只显示 WARN/ERROR 级别日志，`--verbose` 显示完整 INFO 流程
- **启动后打印彩色广告横幅**
- **支持位置参数作为自定义 shell**：`litevm -c rootfs /bin/bash`
- **新增 `-d/--download` 参数**：文本菜单选择发行版并用浏览器打开下载页面
  - 支持 Void Linux、Alpine、Kali、Arch 四款发行版下载链接
  - 纯文本菜单兼容 Termux
  - 优先使用 `termux-open-url` 支持 Termux 环境
- **新增 `build.sh` 支持一键打包为系统安装包**
  - `.deb` — Debian/Ubuntu/Kali (需 dpkg-deb)
  - `.rpm` — Fedora/CentOS/RHEL (需 rpmbuild)
  - `.pkg.tar.zst` — Arch/Manjaro (需 bsdtar + zstd)
  - `.apk` — Alpine Linux (需 tar)
  - 自动检测可用打包工具，支持 `--format` 指定格式
  - 仅在本机架构上打包，避免交叉打包问题
  - Termux 兼容：自动检测 Termux 环境，只编译本机架构，打包 .deb 到 `$PREFIX/bin`

---

## V0.2 (2026-5-4)

### 🆕 全新功能

- 新增 `pm make` 子命令，支持从源码构建 rootfs 镜像
- 支持 Debian/Ubuntu 使用 debootstrap 构建
- 支持 Arch Linux 使用 pacstrap 构建（仅限 Arch 系统）
- 新增三种构建类型：minimal（精简版）、standard（标准版）、full（完整版）
- 自动检测并安装所需构建工具
- 完善的交互式界面（whiptail + 文本回退）
- 更新 `pm download` 支持 Kali Linux（跳转到官网下载）
- 修复和优化多个功能

---

## V0.1 (2026-5-3)

- litevm 的第一个版本
