# groot - Go 版双模式隔离工具

groot 是一个基于 Go 语言开发的轻量级隔离工具，支持 **chroot**、**proot** 两种模式，可在主流 Linux 发行版环境中运行。

## 🎉 核心特性

- **双模式支持**：chroot（需要root）和 proot（无需root）
- **多发行版支持**：Alpine、Arch、Debian、Ubuntu、Fedora、CentOS、Void Linux 等
- **Namespace 隔离**：Mount、PID、UTS、IPC、NET Namespace
- **网络隔离**：chroot 模式支持独立网络命名空间
- **智能环境配置**：自动挂载、环境变量、符号链接修复
- **pm 子命令**：镜像下载与管理
- **Termux 支持**：Android 设备上也能运行
- **国际化 (i18n)**：支持中文和英文，基于 LANG 环境变量自动切换

## 🔐 多模式支持

| 参数 | 说明 |
|------|------|
| `-c/--chroot <rootfs>` | chroot 模式（需要 root 权限） |
| `-p/--proot <rootfs>` | proot 模式（无需 root 权限） |
| `-z <distro>` | proot 兼容模式（alpine/debian/ubuntu） |
| `-b <shell>` | 指定自定义 shell（/bin/ash、/bin/bash 等） |
| `-u <user>` | 指定用户 |
| `--net / -net` | chroot 模式网络隔离 |
| `--check` | 检查设备是否符合要求 |
| `--termux-lang` | 配置 Termux 语言（生成 ~/.termux/locale.conf） |

## 🐧 支持的发行版

### 自动检测

groot 通过 `/etc/os-release` 自动检测发行版及其衍生版：

| 发行版家族 | 衍生版示例 |
|-----------|-----------|
| **Alpine** | Alpine Linux, PostmarketOS |
| **Arch** | Arch Linux, Manjaro, EndeavourOS, Garuda, Artix |
| **Debian** | Debian, Ubuntu, Linux Mint, Pop!_OS, Kali, Raspbian |
| **RHEL** | Fedora, CentOS, RHEL, Rocky Linux, AlmaLinux, Oracle Linux |
| **Void** | Void Linux (musl/glibc) |
| **Unix** | FreeBSD, OpenBSD, NetBSD, DragonFlyBSD |

### 发行版特定环境变量

groot 不再硬编码发行版特定的环境变量，而是从 rootfs 的配置文件中自动加载。用户在 rootfs 中配置的变量不会被覆盖。

## 🌍 国际化支持 (i18n)

groot 支持中文和英文两种语言，基于 LANG 环境变量自动切换：

### 语言检测优先级

1. `~/.config/groot/lang` 配置文件（用户手动设置）
2. rootfs 的 `/etc/locale.conf` 文件
3. Termux 环境：读取 `~/.termux/locale.conf`
   - `LANG=zh_CN.UTF-8` → 中文
   - `LANG=en_US.UTF-8` 或不存在 → 英文
4. `LANG` 环境变量
5. 默认英文

### 切换语言

```bash
# Termux 用户：使用 --termux-lang 参数唤起 TUI 配置
groot --termux-lang

# 手动设置语言（通用）
echo "zh" > ~/.config/groot/lang   # 中文
echo "en" > ~/.config/groot/lang   # 英文

# Termux 用户（直接编辑配置文件）
echo "LANG=zh_CN.UTF-8" > ~/.termux/locale.conf

# 或通过环境变量
export LANG=zh_CN.UTF-8
```

首次运行时，如果检测到 Termux 环境且无 `~/.termux/locale.conf` 文件，会启动交互式语言选择，选择后自动生成该文件。

## 🔧 自动化环境配置

### 智能挂载

自动挂载以下文件系统：
- `/proc`, `/sys`, `/dev`, `/dev/pts`, `/dev/shm`
- `/run`, `/tmp`, `/var/run`, `/var/tmp`

### 环境变量自动加载

groot 从 rootfs 的配置文件中自动加载环境变量，不会覆盖用户在 rootfs 中的配置：

| 配置文件 | 用途 |
|----------|------|
| `/etc/locale.conf` | 语言环境（LANG、LC_ALL、LANGUAGE） |
| `/etc/environment` | 系统级环境变量 |
| `/etc/passwd` | 用户信息（HOME、USER、SHELL） |
| `/etc/hostname` | 主机名 |

shell 脚本（`/etc/profile`、`/etc/profile.d/*.sh`、`~/.bashrc` 等）由 login shell 在启动时自行 source，groot 不会预解析它们，确保复杂的 shell 语法（如 `PS1`、`PS0`）能被正确执行。

### 设备节点

自动创建必要的设备节点：
- `/dev/null`, `/dev/zero`, `/dev/random`, `/dev/urandom`
- `/dev/tty`, `/dev/console`, `/dev/ptmx`

### 符号链接修复

自动修复 rootfs 中的符号链接：
- `/usr/bin/lua` → `lua5.4/lua5.3/luajit`
- `/usr/bin/python` → `python3`
- `/bin/sh` → `bash/dash/ash`
- `/usr/bin/cls` → `clear`

### Shell 支持

- 支持所有 POSIX shell（sh, ash, dash, bash）
- 支持 zsh, fish 等高级 shell
- 自动设置 `SHELL` 环境变量
- 支持通过 `-b` 参数或位置参数指定 shell

## ✨ pm 子命令 - 镜像管理

### 支持的镜像

| 发行版 | 架构/版本 |
|--------|----------|
| Void Linux | x86_64, arm64, arm32 (musl/glibc) |
| Ubuntu | 24.04, 26.04 (amd64, arm64) |
| Alpine | amd64, arm64, arm32 |
| Arch | arm64 |
| Kali Linux | 多版本 |

### 命令

```bash
groot pm list       # 列出可用镜像
groot pm download   # 下载镜像
groot pm make       # 构建镜像（debootstrap/pacstrap）
```

## 📦 打包支持

使用 `build.sh` 一键打包：

```bash
./build.sh                    # 自动检测并打包所有格式
./build.sh --format deb       # 只打包 deb
./build.sh --format rpm       # 只打包 rpm
./build.sh --format pacman    # 只打包 pacman
./build.sh --format apk       # 只打包 apk
```

支持格式：
- `.deb` — Debian/Ubuntu/Kali/Termux
- `.rpm` — Fedora/CentOS/RHEL
- `.pkg.tar.zst` — Arch/Manjaro
- `.apk` — Alpine Linux

## 🚀 快速开始

### 编译

```bash
go build -o groot ./cmd/groot
```

### 一键编译打包

使用 build.sh 脚本，自动编译并打包为系统安装包：

```bash
# 编译所有架构并打包所有格式 (自动检测可用工具)
./build.sh

# 只打包 deb 和 rpm
./build.sh --format deb,rpm

# 编译指定架构并打包
./build.sh build amd64 --format deb

# 仅打包已有二进制
./build.sh package amd64 --format deb

# 查看帮助
./build.sh help
```

生成的安装包在 `build/` 目录下：

- `groot_0.3.2.3_amd64.deb` — Debian/Ubuntu/Kali/Termux
- `groot-0.3.2.3-1.x86_64.rpm` — Fedora/CentOS/RHEL
- `groot-0.3.2.3-1-x86_64.pkg.tar.zst` — Arch/Manjaro
- `groot-0.3.2.3.apk` — Alpine Linux

```bash
./groot --help
```

#### 使用 pm 子命令（推荐！先下载镜像）

```bash
# 查看可用镜像列表
./groot pm list

# 交互式下载镜像（推荐）
./groot pm download

# 下载所有镜像
./groot pm download --all

# 自定义下载目录
./groot pm download --dest ~/my-images
```

pm 子命令目前支持：

- Void Linux (多种架构和 libc)
- Ubuntu (多个版本和架构)
- Alpine (多种架构)
- Kali Linux (跳转到官网下载)

#### 使用 pm make 构建镜像（推荐！）

```bash
# 交互式构建镜像（推荐）
sudo ./groot pm make

# 命令行模式构建
sudo ./groot pm make --distro debian --version 12 --type standard
sudo ./groot pm make --distro ubuntu --version 24.04 --type minimal

# 自定义构建目录
sudo ./groot pm make --distro arch --dest ~/my-rootfs
```

pm make 支持的发行版：

- **Debian**：10, 11, 12, testing, unstable
- **Ubuntu**：20.04, 22.04, 24.04
- **Arch Linux**：仅限在 Arch 系统上构建

#### 专属 proot 兼容模式

```bash
# 使用专属 proot 兼容模式
./groot -z alpine /path/to/alpine/rootfs

# 或者使用自定义 shell
./groot -z debian /path/to/debian/rootfs -b /bin/bash
```

#### chroot 模式 (需要 root)

```bash
# 默认使用 root 用户和 /bin/sh
sudo ./groot -c /path/to/rootfs

# 使用自定义 shell
sudo ./groot -c /path/to/rootfs -b /bin/bash

# 启用独立网络命名空间（创建隔离的网络栈）
sudo ./groot -c /path/to/rootfs --net
# 或
sudo ./groot -c /path/to/rootfs -net
```

#### 通用 proot 模式 (无需 root)

```bash
# 默认使用 root 用户和 /bin/sh
./groot -p /path/to/rootfs

# 使用自定义 shell
./groot -p /path/to/rootfs -b /bin/ash
```

#### 其他功能

```bash
# 清理残留挂载点
sudo ./groot -k /path/to/rootfs

# 打开浏览器下载 rootfs 镜像
./groot -d

# 列出支持的发行版
./groot -l

# 启用详细日志
./groot --verbose -p /path/to/rootfs

# 启用调试日志
./groot --debug -p /path/to/rootfs
```

## 📁 项目结构
```groot/
├── cmd/
│   └── groot/
│       └── main.go           # 主程序入口
├── internal/
│   ├── cleanup/             # 宿主机环境清理
│   ├── termux/              # Termux 环境检测与适配
│   ├── images/              # 镜像下载管理
│   │   └── images.go       # pm 子命令实现
│   ├── chroot/
│   │   └── chroot.go         # chroot 模式完美实现（含 Namespace）
│   ├── proot/
│   │   ├── proot.go          # 通用 proot 模式
│   │   └── alpine-proot.go  # Alpine 专属完美模式
│   ├── mount/
│   │   └── mount.go         # 完美挂载管理
│   ├── env/
│   │   └── env.go            # 完美发行版识别变量配置
│   ├── i18n/
│   │   ├── i18n.go           # 国际化框架与语言检测
│   │   └── messages.go       # 中英文翻译字符串
│   ├── permission/
│   │   └── permission.go     # 权限检测
│   └── logger/
│       └── logger.go          # 日志管理
├── images_manager/          # 新增：镜像配置
│   └── images_update.jsonc # 镜像下载源配置
├── alpine/                   # Alpine Linux rootfs (示例)
├── build.sh                 # 编译与打包脚本（支持 deb/rpm/pacman/apk）
├── go.mod
├── go.sum
└── README.md
```

## 📋 系统要求

- Linux 内核 >= 3.8 (支持 Namespace)
- proot 模式需要系统 proot 命令
  - Debian/Ubuntu: `sudo apt install proot`
- pm 子命令推荐（可选）：
  - `whiptail` - 提供交互式菜单界面
  - `wget` - 提供更好的下载体验（自动安装）

### void Linux的musl版本无法运行groot？

- 安装官方的兼容版本glibc即可
  ```
  xbps-install -S void-repo-nonfree
  xbps-install -S glibc-locales glibc
  ```

## 📋 更新日志

### 2026-9-13 — V0.3.2.3 (当前版本)

#### 🔧 修复与优化

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

### 2026-9-12 — V0.3.2.2

#### 🌍 国际化 (i18n)

- **新增 i18n 国际化支持**：所有用户可见的字符串支持中文和英文
  - 新增 `internal/i18n/` 包，提供翻译框架和 200+ 翻译 key
  - 基于 LANG 环境变量自动检测语言，支持从配置文件、locale.conf、环境变量读取
  - Termux 环境自动读取 `~/.termux/locale.conf` 判断语言（`zh_CN.UTF-8` → 中文，其他 → 英文）
  - 新增 `--termux-lang` 参数，可随时唤起 TUI 切换 Termux 语言
  - 用户可通过 `~/.config/groot/lang` 手动切换语言
- **替换所有源文件中的硬编码中文字符串**：
  - `cmd/groot/main.go`：CLI 帮助文本、错误消息
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

#### 🔧 修复与优化

- **修复 rootfs 配置加载问题**：彻底移除所有硬编码环境变量，改为从 rootfs 配置文件自动加载
  - 移除硬编码：`LANG`、`LC_ALL`、`LC_CTYPE`、`EDITOR`、`VISUAL`、`PAGER`、`LESS`、`TMPDIR`、`MAIL`、`TZ`、`HISTFILE`、`HISTSIZE`、`HISTFILESIZE`、`XDG_*` 等
  - 移除所有发行版特定硬编码：`DEBIAN_FRONTEND`、`PACMAN`、`DNF`、`XBPS_*`、`SELINUX` 等
  - 现在仅设置 groot 自身必需的变量：`HOME`、`USER`、`LOGNAME`、`SHELL`、`PWD`、`HOSTNAME`
  - 从 `/etc/locale.conf` 自动加载语言环境，并智能推导 `LC_ALL`、`LC_CTYPE`、`LANGUAGE`
  - 从 `/etc/environment` 自动加载系统级环境变量
  - shell 脚本（`/etc/profile`、`/etc/profile.d/*.sh`、`~/.bashrc`）由 login shell 自行 source，不再由 groot 预解析
  - 修复了预解析 shell 脚本导致 `PS0`/`PS1` 等包含 shell 语法的变量被错误设为字面值的问题
- **修复 pacman wrapper 问题**：移除 wrapper 中硬编码的 `export LC_ALL=C`，允许用户 locale 设置生效
- **移除 pacman wrapper DEBUG 信息**：清理 `echo "DEBUG: ..."` 输出
- **改善卸载容错性**：`UnmountAll` 对 `EINVAL` 错误降级为 Debug 日志，命名空间销毁时自动清理

### 2026-9.11 — V0.3.2

#### 🆕 全新功能

##### 1. chroot 网络隔离模式 (`--net` / `-net`)

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
sudo ./groot -c /path/to/rootfs --net
# 或
sudo ./groot -c /path/to/rootfs -net
```

**实现细节：**
- 在 `internal/chroot/chroot.go` 中添加 `netMode bool` 参数
- 在 `internal/network/network.go` 中实现完整的网络配置
- namespace 创建后等待 100ms 确保子进程已完成网络命名空间初始化
- 子进程退出时自动清理宿主机网络资源（veth、主机端 iptables 规则）

##### 2. LXC 容器管理子命令

新增 `lxc` 子命令，提供完整的 LXC 风格轻量级容器管理能力，实现对 rootfs 的完整隔离虚拟化。

**核心特性：**
- **完整的 namespace 隔离**：mount、PID、UTS、IPC、NET 五大命名空间完全隔离
- **守护进程模式**：容器 init 进程在后台运行，作为容器内的 PID 1，不依赖终端
- **自动挂载虚拟文件系统**：/proc、/sys、/dev、/dev/pts、/dev/shm、/tmp、/run 自动挂载到 rootfs 内
- **主机名隔离**：容器拥有独立的 UTS namespace，主机名不影响宿主机
- **就绪握手机制**：通过文件描述符管道（fd 3），父进程等待容器 init 就绪后再返回
- **容器配置持久化**：容器配置存储在 `/data/data/com.termux/files/usr/share/groot/lxc/containers/` 下的 JSON 文件中

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

**使用示例：**
```bash
# 创建容器
sudo ./groot lxc /path/to/rootfs /bin/sh -name mycontainer

# 启动容器（后台运行）
sudo ./groot lxc mycontainer start

# 登录容器
sudo ./groot lxc mycontainer

# 查看容器列表
sudo ./groot lxc ls

# 停止容器
sudo ./groot lxc mycontainer stop

# 删除容器
sudo ./groot lxc mycontainer rm
```

#### 🔧 修复与优化

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

## 💡 开发说明

本项目完全完美实现！

## 📖 版本历史

- 2026-9-9-18:00->V0.3.2-LXC 登录修复与终端安全增强
  - **修复**: LXC 登录时 `shell-init: error retrieving current directory` 错误
    - 在 `LoginContainer` 中添加 `os.Chdir("/")` 确保登录前切换到安全目录
    - 使用 `env -i` 清除继承的环境变量，防止错误的 `PWD`/`OLDPWD` 传递给容器 shell
    - 使用包装命令 `sh -c 'cd / 2>/dev/null; exec bash -l'` 确保所有配置文件在正确目录下执行
  - **修复**: LXC stop 容器导致宿主机伪终端失效的严重问题
    - 使用 `defer signal.Reset(syscall.SIGINT)` 确保信号状态一定会被恢复
    - 移除 `cleanupNetworkInChild()` 中的 `ip link del lo` 危险命令，避免误删宿主机 loopback 接口
    - 添加网络清理超时机制，防止命令阻塞容器退出
  - **改进**: LXC 配置存储路径改为标准 Linux 路径
    - 从 Termux 特定路径 `/data/data/com.termux/files/usr/share/groot/lxc` 改为 `/usr/local/groot/lxc`
    - LXC 是标准 Linux 功能，配置应存储在标准位置
  - **修复**: 伪终端无法分配和桌面程序无法启动
    - 添加 `/dev/ptmx` 设备节点创建（符号链接到 `/dev/pts/ptmx`）
    - 添加 X11 socket 目录挂载 (`/tmp/.X11-unix`)，支持容器内 GUI 程序连接到宿主机显示服务器
  - **改进**: 简化登录命令结构，移除不必要的 wrapper 嵌套层
  - **改进**: 增强容器退出时的资源清理安全性

- 2026-9.11->V0.3.1-LXC 容器与 chroot net 模式
  - **新增**: chroot 网络隔离模式 (`--net` / `-net`)
    - 使用 `clone(CLONE_NEWNET)` 创建独立的网络命名空间
    - 宿主机与容器通过 veth 虚拟以太网对通信
    - 提供 DNS 配置和 NAT 转发能力
    - 子进程退出时自动清理网络资源
  - **新增**: LXC 容器管理子命令
    - 支持容器的创建、启动、停止、登录、删除、列举等操作
    - 完整的 namespace 隔离（mount、PID、UTS、IPC、NET）
    - 守护进程模式，容器 init 作为 PID 1 运行
    - 自动挂载虚拟文件系统
    - 就绪握手机制，父进程等待容器就绪
    - 容器配置持久化存储
- 2026-8.13->V0.3.1-Termux 跨平台崩溃修复
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
- 2026-7.25->V0.3-功能增强与 Termux 适配
  - **新增**: Termux 环境检测与自动适配（`internal/termux`）
    - Termux 环境自动清理 `LD_PRELOAD` 避免冲突
    - Android root 权限检测，无 root 自动提示使用 proot 模式
    - Termux 中 proot/chroot 未安装时自动 `pkg install -y proot`
    - 安全的 `SafeLookPath` 绕过 proot 嵌套中 `faccessat2` 被 seccomp 拦截的问题
  - **新增**: 宿主机环境自动清理（`internal/cleanup`）
    - 清理 `/etc/hosts` 中的宿主机私有 IP 条目，保留 rootfs 本地回环
    - 清理 `/etc/resolv.conf` 中的宿主机私有 DNS
    - 保留 rootfs 自己的 hostname，仅在缺失时补充默认
    - 清空 machine-id、shell 历史、SSH known_hosts、/var/log、/tmp
    - chroot 与 proot 模式均自动执行
  - **改进**: Profile 加载方式
    - 两种模式统一使用 `exec shell -l`（login shell）方式启动
    - 自动加载 rootfs 自己的 `/etc/profile` 和 `/etc/profile.d/*.sh`
  - **改进**: 日志系统支持 ANSI 彩色输出
    - `[DEBUG]` 蓝色、`[INFO]` 绿色、`[WARN]` 黄色、`[ERROR]` 红色
    - 非终端输出时自动禁用颜色
  - **改进**: 默认只显示 WARN/ERROR 级别日志，`--verbose` 显示完整 INFO 流程
  - **新增**: 启动后打印彩色广告横幅
  - **新增**: 支持位置参数作为自定义 shell（`groot -c rootfs /bin/bash`）
  - **改进**: 所有 `exec.LookPath` 替换为 `termux.SafeLookPath`，避免 SIGSYS 崩溃
  - **修复**: Termux 下执行 `pkg install proot` 自动安装时再次崩溃（同样是 faccessat2 被拦截），改为先找到 pkg 完整路径再执行
  - **新增**: `-d/--download` 参数，文本菜单选择发行版并用浏览器打开下载页面
    - 支持 Void Linux、Alpine、Kali、Arch 四款发行版下载链接
    - 纯文本菜单兼容 Termux
    - 优先使用 `termux-open-url` 支持 Termux 环境
    - 彩色多色提示文字
  - **新增**: `build.sh` 支持一键打包为系统安装包
    - `.deb` — Debian/Ubuntu/Kali (需 dpkg-deb)
    - `.rpm` — Fedora/CentOS/RHEL (需 rpmbuild)
    - `.pkg.tar.zst` — Arch/Manjaro (需 bsdtar + zstd)
    - `.apk` — Alpine Linux (需 tar)
    - 自动检测可用打包工具，支持 `--format` 指定格式
     - 仅在本机架构上打包，避免交叉打包问题
     - **Termux 兼容**：自动检测 Termux 环境，只编译本机架构，打包 .deb 到 `$PREFIX/bin`
- 2026-5.4->V0.2-pm make 功能增强
  - 新增 pm make 子命令，支持从源码构建 rootfs 镜像
  - 支持 Debian/Ubuntu 使用 debootstrap 构建
  - 支持 Arch Linux 使用 pacstrap 构建（仅限 Arch 系统）
  - 新增三种构建类型：minimal（精简版）、standard（标准版）、full（完整版）
  - 自动检测并安装所需构建工具
  - 完善的交互式界面（whiptail + 文本回退）
  - 更新 pm download 支持 Kali Linux（跳转到官网下载）
  - 修复和优化多个功能
- 2026-5.3->V0.1-groot的第一个版本

## 📄 许可证

MIT License

Copyright (c) 2026 groot

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
