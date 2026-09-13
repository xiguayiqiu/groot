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

## 🚀 快速开始

### 编译

```bash
go build -o groot ./cmd/groot
```

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

## 🖥️ VMM 虚拟机模式

groot 支持使用 Firecracker 启动轻量级 microVM，需要 rootfs.ext4 虚拟磁盘。

### 依赖

- Linux 内核（x86_64: `kernel/amd64/vmlinux.bin`，arm64: `kernel/arm64/vmlinux.bin`）
- Firecracker 二进制（安装到 `~/bin/firecracker` 或 `/usr/local/bin/firecracker`）
- rootfs.ext4（ext4 格式虚拟磁盘，详见 [创建 rootfs.ext4](docs/create-rootfs.md)）

### 子命令

| 子命令 | 说明 |
|--------|------|
| `groot vmm run` | 启动 microVM |
| `groot vmm download-kernel` | 从 Firecracker S3 下载官方内核 |
| `groot vmm setup-network` | 配置 VMM 网络（需要 root，一次性运行） |
| `groot vmm rm-network` | 清除 VMM 网络节点（需要 root） |

### 快速开始

```bash
# 1. 下载内核（交互式选择）
./groot vmm download-kernel

# 2. 配置网络（一次性，需要 root）
sudo ./groot vmm setup-network

# 3. 启动 VM（无需 root）
./groot vmm run --kernel kernel/x86_64/vmlinux-6.18.44 --rootfs rootfs/rootfs.ext4 --net

# 4. SSH 访问 VM（DHCP 分配 IP）
ssh root@172.16.0.25   # 密码: root
```

### 下载内核

```bash
# 下载当前架构的官方内核（交互式选择）
./groot vmm download-kernel

# 下载所有可用内核（当前架构）
./groot vmm download-kernel --all

# 下载指定架构的内核
./groot vmm download-kernel --arch x86_64
./groot vmm download-kernel --arch aarch64

# 下载所有架构的所有内核
./groot vmm download-kernel --all --arch all
```

### 启动 VM

```bash
# 基本启动（无网络）
./groot vmm run --kernel kernel/x86_64/vmlinux-6.18.44 --rootfs rootfs/rootfs.ext4

# 启用网络
./groot vmm run --kernel kernel/x86_64/vmlinux-6.18.44 --rootfs rootfs/rootfs.ext4 --net

# 自定义配置
./groot vmm run \
  --kernel kernel/x86_64/vmlinux-6.18.44 \
  --rootfs rootfs/rootfs.ext4 \
  --mem 512 \
  --cpus 4 \
  --net

# 自定义内核参数
./groot vmm run \
  --kernel kernel/x86_64/vmlinux-6.18.44 \
  --rootfs rootfs/rootfs.ext4 \
  --net \
  --kernel-args "console=ttyS0,115200n8 reboot=k panic=1 nomodule systemd.unit=multi-user.target"
```

### 网络配置

#### 方式一：一次性配置（推荐）

```bash
# 配置网络（需要 root）
sudo ./groot vmm setup-network

# 清除网络（需要 root）
sudo ./groot vmm rm-network
```

`setup-network` 会自动完成：
- 创建 TAP 设备（`groot-tap0`），分配给当前用户
- 配置 IP 地址和 NAT 转发
- 启动 dnsmasq DHCP 服务器
- 授权 `/dev/kvm` 和 `/dev/net/tun`

配置完成后，日常使用无需 root：

```bash
./groot vmm run --kernel ... --rootfs ... --net
```

#### 方式二：root 运行

如果不运行 `setup-network`，也可以直接使用 root：

```bash
sudo ./groot vmm run --kernel ... --rootfs ... --net
```

### run 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--kernel`, `-k` | Linux 内核路径 | `kernel/amd64/vmlinux.bin` |
| `--rootfs`, `-r` | rootfs.ext4 虚拟磁盘路径 | `rootfs/rootfs.ext4` |
| `--mem` | 内存大小 (MB) | 256 |
| `--cpus` | CPU 核心数 | 2 |
| `--net` | 启用网络（TAP + NAT + DHCP） | 关闭 |
| `--tap` | TAP 设备名称 | `groot-tap0` |
| `--host-ip` | 宿主机 TAP 接口 IP | `172.16.0.1` |
| `--guest-ip` | VM 内静态 IP（DHCP 备用） | `172.16.0.2` |
| `--kernel-args` | 自定义内核启动参数（覆盖默认值） | `console=ttyS0,115200n8 reboot=k panic=1 nomodule` |

### 网络架构

启用 `--net` 时：
1. 宿主机创建 TAP 设备（`groot-tap0`）
2. 宿主机配置 NAT 和 IP 转发
3. dnsmasq 提供 DHCP 和 DNS 服务
4. VM 内核通过 `ip=dhcp` 自动获取 IP
5. VM 通过 NAT 访问外部网络

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
│   ├── vmm/                  # Firecracker microVM 支持
│   │   ├── vmm.go            # VM 核心逻辑（启动、配置、信号处理）
│   │   ├── network.go        # TAP 设备、NAT、DHCP 网络支持
│   │   └── download.go       # 从 Firecracker S3 下载内核
│   ├── i18n/
│   │   ├── i18n.go           # 国际化框架与语言检测
│   │   └── messages.go       # 中英文翻译字符串
│   ├── permission/
│   │   └── permission.go     # 权限检测
│   └── logger/
│       └── logger.go          # 日志管理
├── kernel/                    # Linux 内核（用于 VMM）
│   └── amd64/
│       └── vmlinux.bin
├── rootfs/                    # rootfs 虚拟磁盘（用于 VMM）
│   └── rootfs.ext4
├── docs/
│   └── create-rootfs.md      # 创建 rootfs.ext4 教程
├── images_manager/          # 新增：镜像配置
│   └── images_update.jsonc # 镜像下载源配置
├── alpine/                   # Alpine Linux rootfs (示例)
├── build.sh                 # 编译脚本（支持交叉编译和 tar.xz 打包）
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

详见 [CHANGELOG.md](CHANGELOG.md)

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
