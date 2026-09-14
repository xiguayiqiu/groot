# litevm

> **本项目原名 `groot`，现已正式更名为 `litevm`。**

litevm 是一个基于 Go 语言开发的轻量级 Linux 隔离与虚拟化工具，支持 **chroot**、**proot**、**Firecracker microVM** 三种运行模式，可在主流 Linux 发行版及 Android Termux 环境中运行。

## 核心特性

- **三模式运行**：chroot（需 root）、proot（无需 root）、VMM（Firecracker microVM）
- **多发行版支持**：Alpine、Arch、Debian、Ubuntu、Fedora、CentOS、Void Linux 等自动检测
- **Namespace 隔离**：Mount、PID、UTS、IPC、NET Namespace（chroot 模式）
- **网络隔离**：chroot 模式支持独立网络命名空间；VMM 模式支持 TAP + NAT + DHCP
- **microVM 支持**：基于 Firecracker 启动轻量级虚拟机，支持 x86_64 / aarch64
- **镜像管理**：内置 `pm` 子命令，交互式下载和构建 rootfs 镜像
- **国际化 (i18n)**：中文 / 英文自动切换，基于 LANG 环境变量
- **Termux 支持**：Android 设备上也能运行

## 项目更名说明

本项目最初名为 `groot`，现正式更名为 `litevm`。所有代码模块、二进制文件、配置路径、TAP 设备名等均已完成迁移：

| 项目 | 旧名 (groot) | 新名 (litevm) |
|------|-------------|--------------|
| Go module | `groot` | `litevm` |
| 二进制文件 | `groot` | `litevm` |
| 入口目录 | `cmd/groot/` | `cmd/litevm/` |
| TAP 设备 | `groot-tap0` | `litevm-tap0` |
| systemd 服务 | `groot-network.service` | `litevm-network.service` |
| 配置路径 | `~/.config/groot/` | `~/.config/litevm/` |
| 临时文件 | `/tmp/groot-*` | `/tmp/litevm-*` |

## 快速开始

### 编译

```bash
# 直接编译
go build -o litevm ./cmd/litevm

# 或使用编译脚本（支持交叉编译和打包）
./build.sh
```

### 运行

```bash
./litevm --help
./litevm --version
```

## 运行模式

### proot 模式（无需 root）

```bash
# 默认使用 root 用户和 /bin/sh
./litevm -p /path/to/rootfs

# 使用自定义 shell
./litevm -p /path/to/rootfs -b /bin/ash

# proot 兼容模式（Alpine/Debian/Ubuntu 专属优化）
./litevm -z alpine /path/to/alpine/rootfs
./litevm -z debian /path/to/debian/rootfs -b /bin/bash
```

### chroot 模式（需要 root）

```bash
# 基本使用
sudo ./litevm -c /path/to/rootfs

# 自定义 shell 和用户
sudo ./litevm -c /path/to/rootfs -b /bin/bash -u myuser

# 启用网络隔离
sudo ./litevm -c /path/to/rootfs --net
```

### VMM 模式（Firecracker microVM）

```bash
# 1. 下载内核（交互式选择）
./litevm vmm download-kernel

# 2. 配置网络（一次性，需要 root）
sudo ./litevm vmm setup-network

# 3. 启动 VM（无需 root）
./litevm vmm run --kernel kernel/x86_64/vmlinux-6.18.44 --rootfs rootfs/rootfs.ext4 --net

# 4. 在 VM 内通过 DHCP 自动获取 IP
```

## VMM 子命令

| 子命令 | 说明 |
|--------|------|
| `litevm vmm run` | 启动 microVM |
| `litevm vmm download-kernel` | 从 Firecracker S3 下载官方内核 |
| `litevm vmm setup-network` | 配置 TAP 设备、NAT、DHCP（需要 root，一次性运行） |
| `litevm vmm rm-network` | 清除网络配置（需要 root） |

### run 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--kernel`, `-k` | Linux 内核路径 | — |
| `--rootfs`, `-r` | rootfs.ext4 虚拟磁盘路径 | — |
| `--mem` | 内存大小 (MB) | 256 |
| `--cpus` | CPU 核心数 | 2 |
| `--net` | 启用网络 | 关闭 |
| `--tap` | TAP 设备名称 | `litevm-tap0` |
| `--host-ip` | 宿主机 TAP 接口 IP | `172.16.0.1` |
| `--guest-ip` | VM 内 IP（DHCP 备用） | `172.16.0.2` |
| `--kernel-args` | 自定义内核启动参数 | `console=ttyS0,115200n8 reboot=k panic=1 nomodule` |

### 网络架构

```
  宿主机                              VM (guest)
┌─────────────┐                  ┌─────────────────┐
│  litevm-tap0│                  │  eth0 (virtio)  │
│  172.16.0.1 │◄──── NAT ──────►│  172.16.0.x     │
│             │    MASQUERADE    │  (DHCP)         │
│  dnsmasq    │                  │                 │
│  (DHCP/DNS) │                  │                 │
└─────────────┘                  └─────────────────┘
```

`setup-network` 通过 systemd 服务 (`litevm-network.service`) 管理完整的网络生命周期：
- 创建 TAP 设备并分配 IP
- 启动 dnsmasq 提供 DHCP 和 DNS
- 配置 iptables NAT 和转发规则
- 重启后自动恢复，无需重新配置

## 镜像管理 (pm)

```bash
# 列出可用镜像
./litevm pm list

# 交互式下载镜像
./litevm pm download

# 构建镜像（debootstrap/pacstrap）
sudo ./litevm pm make
sudo ./litevm pm make --distro debian --version 12 --type standard
```

支持的发行版镜像：Void Linux、Ubuntu、Alpine、Arch、Kali Linux、Debian。

## i18n 国际化

支持中文和英文，自动检测切换：

```bash
# 手动切换语言
echo "zh" > ~/.config/litevm/lang   # 中文
echo "en" > ~/.config/litevm/lang   # 英文

# 通过环境变量
export LANG=zh_CN.UTF-8
```

检测优先级：`~/.config/litevm/lang` → rootfs `/etc/locale.conf` → Termux locale → `LANG` 环境变量 → 默认英文。

## 项目结构

```
litevm/
├── cmd/litevm/          # CLI 入口
│   └── main.go
├── internal/
│   ├── chroot/          # chroot 模式（含 Namespace 隔离）
│   ├── proot/           # proot 模式（含 Alpine 专属优化）
│   ├── vmm/             # Firecracker microVM 支持
│   │   ├── vmm.go       # VM 核心逻辑
│   │   ├── network.go   # TAP/NAT/DHCP 网络（多发行版适配）
│   │   └── download.go  # 内核下载
│   ├── i18n/            # 国际化框架
│   ├── images/          # 镜像管理
│   ├── mount/           # 挂载管理
│   ├── env/             # 环境变量配置
│   ├── distro/          # 发行版检测
│   ├── network/         # 宿主机网络工具
│   ├── permission/      # 权限检测
│   ├── termux/          # Termux 适配
│   └── logger/          # 日志管理
├── kernel/              # Linux 内核（VMM 用）
├── rootfs/              # rootfs.ext4 虚拟磁盘（VMM 用）
├── docs/                # 文档
├── images_manager/      # 镜像配置
├── build/               # 编译输出
├── build.sh             # 编译与打包脚本
├── go.mod
└── README.md
```

## 系统要求

- Linux 内核 >= 3.8
- Go >= 1.21（编译）
- proot 模式需要 `proot` 命令（`apt install proot`）
- VMM 模式需要：
  - KVM 支持（`/dev/kvm`）
  - Firecracker 二进制（安装到 `~/bin/firecracker` 或 `/usr/local/bin/firecracker`）
- pm 子命令推荐：`whiptail`（交互式菜单）、`wget`（下载）

### Void Linux musl 版本

无法运行 litevm 时，安装 glibc 兼容包：

```bash
xbps-install -S void-repo-nonfree
xbps-install -S glibc-locales glibc
```

## 许可证

MIT License - Copyright (c) 2026 BiliBili-Yiqiu

详见 [LICENSE](LICENSE)

## 更新日志

详见 [CHANGELOG.md](CHANGELOG.md)
