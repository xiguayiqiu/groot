# groot - Go 版双模式隔离工具

groot 是一个基于 Go 语言开发的轻量级隔离工具，支持 chroot 和 proot 两种模式，可在主流 Linux 发行版环境中运行。

## 🎉 核心特性

groot 提供一站式 Linux 容器环境解决方案，从镜像下载到环境运行，全程无压力。

### ✨ 新增：pm 子命令 - 镜像管理与构建

- **`pm list`** - 列出可用的 rootfs 镜像
- **`pm download`** - 下载 rootfs 镜像
  - 支持 wget 下载，自动安装 wget
  - 双模式交互界面（优先 whiptail，回退到文本模式）
  - 自动按发行版分类存储
- **`pm make`** - 构建 rootfs 镜像
  - 使用 debootstrap 构建 Debian/Ubuntu 等发行版
  - 使用 pacstrap 构建 Arch Linux（仅限 Arch 系统）
  - 支持三种构建类型：minimal（精简版）、standard（标准版）、full（完整版）
  - 自动检测和安装所需工具（debootstrap/pacstrap）

#### 📦 支持的镜像下载列表

| 发行版        | 架构/版本   | libc 选项      |
| ---------- | ------- | ------------ |
| Void Linux | x86\_64 | musl, glibc  |
| Void Linux | arm64   | musl, glibc  |
| Void Linux | arm32   | musl, glibc  |
| Ubuntu     | 2404    | amd64, arm64 |
| Ubuntu     | 2604    | amd64, arm64 |
| Alpine     | amd64   | -            |
| Alpine     | amd32   | -            |
| Alpine     | arm64   | -            |
| Alpine     | arm32   | -            |
| Kali Linux | 多版本     | -            |

### 🔐 多模式支持

- **`-c/--chroot`** - chroot 模式：指定 rootfs 目录（需要 root 权限）
- **`-p/--proot`** - proot 模式：指定 rootfs 目录（无需 root 权限）
- **`-z`** - 专属 proot 兼容模式：指定发行版（alpine/debian/ubuntu），然后指定 rootfs 目录

### 🔒 Namespace 隔离

- 支持 Mount、PID、UTS、IPC Namespace
- 提供更强的隔离安全性

### 🐧 完美的发行版适配

| 发行版家族      | 检测文件                                                                | 特点                                                     |
| ---------- | ------------------------------------------------------------------- | ------------------------------------------------------ |
| Alpine     | `/etc/alpine-release`                                               | 纯净环境变量！                                                |
| Void Linux | `/etc/void-release`                                                 | 高效简洁，支持 musl/glibc 双 libc                              |
| Debian 系列  | `/etc/debian_version`, `/etc/kali_version`, `/etc/ubuntu_version`   | DEBIAN\_FRONTEND, APT\_LISTCHANGES\_FRONTEND, DPKG\_\* |
| RedHat 系列  | `/etc/redhat-release`, `/etc/fedora-release`, `/etc/centos-release` | RPM\_BUILD\_ROOT, RPM\_OPTS                            |
| Arch       | `/etc/arch-release`                                                 | PACMAN\_HOOKS                                          |

完美不污染！

### 🎯 完美的初始化

- **环境配置**：自动加载 /etc/profile 初始化 shell 环境
- **主机名**：自动从 rootfs 读取并设置 /etc/hostname
- **用户切换**：支持 -u 参数指定用户，自动验证用户是否存在

### 💻 自定义 Shell 与用户

- **Shell 定制**：`-b` 参数支持指定自定义 shell 解释器（/bin/ash、/bin/bash 等）
- **用户切换**：配合 `-z` 模式还支持指定用户进行权限隔离

### 🔧 自动化环境配置

- **智能挂载**：自动完成所有必要的文件系统挂载
  - /proc、/sys、/dev 等核心系统目录
  - /dev/pts、/dev/shm 等终端和共享内存
  - /tmp、/run 等临时运行目录
- **环境变量优化**：根据不同发行版自动配置合适的环境变量
- **主机名设置**：自动从 /etc/hostname 读取并设置容器主机名
- **一键就绪**：所有配置自动化完成，开箱即用

### 🛡️ 不影响宿主机

- 所有挂载完美隔离！

### 🎭 终极权限方案

- proot 完美修复权限！
- 真实用户完全访问！

### 📦 高兼容性

- 完美支持 Void Linux、Arch、RedHat、Debian、Ubuntu、Kali、Fedora、CentOS、Alpine 等

### ⚡ 轻量无依赖

- 编译后为单一二进制文件
- 超快启动！

### 📦 支持打包为系统安装包

- 使用 `build.sh` 一键生成 **deb / rpm / pacman / apk** 安装包
- 自动检测当前系统可用打包工具
- 支持指定格式：`--format deb,rpm`
- **Termux 支持**：自动识别 Termux 环境，只编译本机架构并打包 `.deb` 格式，安装到 `$PREFIX/bin`

## � 快速开始

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

- `groot_0.3_amd64.deb` — Debian/Ubuntu/Kali/Termux
- `groot-0.3-1.x86_64.rpm` — Fedora/CentOS/RHEL
- `groot-0.3-1-x86_64.pkg.tar.zst` — Arch/Manjaro
- `groot-0.3.apk` — Alpine Linux

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

```
groot/
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

## 💡 开发说明

本项目完全完美实现！

## 📖 版本历史

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
    - 三种模式统一使用 `exec shell -l`（login shell）方式启动
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
