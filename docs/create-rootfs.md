# 创建 Linux rootfs.ext4 虚拟磁盘 —— 从 rootfs 目录到可启动 ext4 镜像的最全教程

> **适用对象**：litevm `vmm` 模式（Firecracker microVM）用户
> **目标**：让读者能独立制作 **Debian / Ubuntu / Arch / Alpine / Fedora / Rocky / openSUSE / Void 等任意发行版** 的 `rootfs.ext4` 启动盘，
> 并搞清楚 rootfs 目录准备、init 程序/脚本、ext4 镜像前期配置这三段的全部细节。
>
> 本文素材直接来自本仓库 `internal/vmm/`（vmm.go、network.go、download.go）与 `internal/images/`（rootfs-make.go）的实现，所有参数、路径、默认值均与代码一致。

---

## 目录

- [0. 原理：litevm vmm 是怎么把 rootfs 跑起来的](#0-原理litevm-vmm-是怎么把-rootfs-跑起来的)
- [1. 准备：宿主环境、工具与权限](#1-准备宿主环境工具与权限)
- [2. 三条制作路线总览](#2-三条制作路线总览)
- [3. rootfs 目录骨架与 /dev 设备节点](#3-rootfs-目录骨架与-dev-设备节点)
- [4. 路线 A：官方 rootfs tarball（最快）](#4-路线-a官方-rootfs-tarball最快)
- [5. 路线 B：用发行版 bootstrap 工具生成目录](#5-路线-b用发行版-bootstrap-工具生成目录)
- [6. chroot 内基础配置清单（所有发行版通用）](#6-chroot-内基础配置清单所有发行版通用)
- [7. init 系统详解（最核心章节）](#7-init-系统详解最核心章节)
- [8. 网络配置（rootfs 侧）](#8-网络配置rootfs-侧)
- [9. 生成 rootfs.ext4 镜像](#9-生成-rootfsext4-镜像)
- [10. 一键脚本模板](#10-一键脚本模板)
- [11. 验证与首次启动](#11-验证与首次启动)
- [12. 常见发行版速查表](#12-常见发行版速查表)
- [13. 常见问题排查（FAQ）](#13-常见问题排查faq)
- [14. 参考](#14-参考)

---

## 0. 原理：litevm vmm 是怎么把 rootfs 跑起来的

### 0.1 启动链路

litevm 的 `vmm` 模式基于 **Firecracker**，与普通虚拟机/QEMU 有本质区别，理解这条链路是制作 rootfs 的前提：

```
宿主 shell
  └─ ./litevm vmm run --kernel kernel/x86_64/vmlinux... --rootfs rootfs.ext4
       └─ Firecracker（由 firecracker-go-sdk 驱动）
            ├─ 启动 vmlinux 内核（ELF 直接引导，无 BIOS/UEFI/GRUB）
            ├─ 挂载 rootfs.ext4 为 virtio-blk 根设备（读+写）
            ├─ 串口 ttyS0 接到宿主终端的 stdout/stdin
            └─ 内核按 cmdline 执行 PID 1（即 /sbin/init）
```

关键点（全部来自 `internal/vmm/vmm.go`）：

1. **没有 bootloader、没有 MBR/GPT**：rootfs.ext4 不需要分区表，内核由 Firecracker 直接加载，文件系统直接就是整块盘。
2. **默认没有 initramfs**：Firecracker 不支持 initrd，因此"根文件系统必须具备自启动能力"——`/sbin/init` 必须在 rootfs 里真实存在，不能靠 initramfs 临时拼接。
3. **rootfs 是读写挂载的根设备**（`IsReadOnly: false`），所以镜像本身要留足运行期写入空间（/var、日志、SSH key 等）。
4. **内核参数默认值**（`defaultKernelCmdline`）：

   ```
   console=ttyS0,115200n8 reboot=k panic=1 nomodule loglevel=5
   ```

   - `console=ttyS0,115200n8`：内核日志 + 控制台都走串口 ttyS0，guest 里的 getty 必须跑在 ttyS0 上才能看到"登录提示符"。
   - `reboot=k`：用键盘控制器复位方式执行重启（Firecracker 无 ACPI 复位路径）。
   - `panic=1`：内核 panic 1 秒后自动重启，方便调试。
   - `nomodule`：**禁止加载内核模块**！所有必需驱动（virtio-blk、virtio-net、ext4、devtmpfs……）都必须编译进内核，rootfs 内不要依赖 `modprobe`/模块包。
   - 启用 `--net` 时，litevm 会**追加 `ip=dhcp`**（见 `buildConfig`），内核在 init 之前就会自动用 DHCP 配置 eth0。
5. **litevm 会对串口输出做"关机/重启标记"识别**（`guestShutdownMarkers` / `guestRebootMarkers`），guest 执行 `poweroff` 后，litevm 读到内核的 `reboot: System halted` / `Power down` 等固定文本即自动退出并清理资源。所以 rootfs 里的 init 体系**必须能真正完成关机**（至少让内核打印 halt 标记）。

### 0.2 rootfs 的"验收标准"

一个合格的 rootfs.ext4，在 litevm 里必须做到：

| 编号 | 要求 | 原因 |
|------|------|------|
| 1 | 文件系统是 ext4（或 ext3/ext2） | Firecracker root 设备要求，`file rootfs.ext4` 应显示 "Linux rev 1.0 ext4 filesystem" |
| 2 | 存在可执行的 `/sbin/init`（或通过 `--kernel-args init=...` 指定） | 内核首进程 |
| 3 | init 能把 `/proc /sys /dev` 等虚拟文件系统挂起来 | getty、包管理器、udev 都需要 |
| 4 | 有跑在 `ttyS0` 的真 terminal（getty / agetty / 登录 shell） | 否则连不上控制台 |
| 5 | 网络（可选）能被配置 | `--net` 时内核自带 `ip=dhcp`；或用 udhcpc / systemd-networkd / 静态 IP |
| 6 | 静态内容自足 | `nomodule` + 无 initramfs，一切在 rootfs 内 |
| 7 | 可以 `poweroff`/`halt` | litevm 靠串口标记收尾 |

### 0.3 路径与命名约定（与代码一致）

| 项 | 默认值 / 约定 | 出处 |
|----|--------------|------|
| 默认 rootfs 路径 | `rootfs/rootfs.ext4` | `defaultRootfsPath` |
| 默认内核路径（amd64） | `kernel/amd64/vmlinux.bin` | `defaultKernelPath` |
| `download-kernel` 下载目录 | `kernel/x86_64/<vmlinux-版本>`（或 `kernel/aarch64/...`） | `download.go` |
| 启动命令 | `./litevm vmm run --kernel <k> --rootfs <r> [--net]` | `main.go` |
| TAP 设备 / 网段 | `litevm-tap0`，宿主 `172.16.0.1/24` | `defaultNetworkConfig` |
| DHCP 租约范围 | `172.16.0.10 ~ 172.16.0.100`，12h | `network.go` |

> 注意：代码默认内核路径（`kernel/amd64/...`）与下载内核的落盘路径（`kernel/x86_64/...`）命名不同，所以实际使用时**总是用 `--kernel` 显式指定**下载下来的内核，例如 `--kernel kernel/x86_64/vmlinux-6.18.44`。

---

## 1. 准备：宿主环境、工具与权限

### 1.1 权限

制作 rootfs 分两个阶段：

- **bootstrap 阶段**（生成目录）：`debootstrap`/`pacstrap`/`dnf --installroot`/`apk --root` 等**通常需要 root**；仅解压官方 tarball 到目录**不需要 root**（但保留设备节点/属主最好还是 root）。
- **chroot 配置阶段**：必须 root。
- **生成 ext4 阶段**：`mkfs.ext4`、loop 挂载**必须 root**（`mkfs.ext4 -d` 直装法也需要 root 来保留属主与设备节点）。

建议全程：

```bash
sudo -i   # 或 sudo -s
```

### 1.2 工具清单

| 用途 | 工具 | 说明 |
|------|------|------|
| 生成 ext4 镜像 | `dd` 或 `truncate` | 创建空白文件（推荐 truncate，秒级且稀疏） |
| 格式化 | `mkfs.ext4`（e2fsprogs） | 必须；`-d` 直装功能需要 e2fsprogs ≥ 1.43 |
| 挂载 | `mount -o loop`，`umount` | 卷副本法必用 |
| 复制 | `cp -a` 或 `rsync -aHAX` | 保留属主/链接/设备节点 |
| 检查 | `e2fsck`、`debugfs`、`file`、`tune2fs` | 镜像体检 |
| 其他磁盘格式 | `qemu-img`（qemu-utils / qemu-img） | litevm `--drive` 非 raw 自动转换、以及 `CreateRawDisk` 建盘方式 |
| bootstrap | 见 1.3 | 因发行版而异 |
| 跨架构 | `qemu-user-static` + `binfmt_misc` | 在 x86_64 宿主上做 aarch64 rootfs 时用 |

### 1.3 发行版专用构建工具

| 目标发行版 | bootstrap 工具 | 宿主安装命令（Debian/Ubuntu） | 宿主安装命令（Arch/Fedora） |
|-----------|---------------|------------------------------|------------------------------|
| Debian / Ubuntu | `debootstrap` | `apt install -y debootstrap` | `pacman -S debootstrap` / `dnf install debootstrap` |
| Arch | `pacstrap`（arch-install-scripts） | `apt install -y arch-install-scripts` | `pacman -S arch-install-scripts` |
| Alpine | `apk`（apk-tools）或官方 minirootfs tarball | `apt install -y alpine-keys`（解包即可，不强求 apk） | `pacman -S alpine-keyring` |
| Fedora / RHEL / Rocky / Alma | `dnf --installroot` | 各仓库的 dnf 包 | 本身就是 dnf |
| openSUSE | `zypper --root` | — | — |
| Void | `xbps-install -r` | — | — |

> 本项目 `internal/images/rootfs-make.go` 只有 debootstrap / pacstrap 两条自动化路径（`./litevm pm make`），但**手工路线**可以覆盖上面所有发行版——这正是本文的价值所在。

### 1.4 架构

- litevm vmm 支持 **x86_64** 与 **aarch64** 两种 guest 架构（`supportedArchs`）。
- 内核文件必须与架构匹配：x86_64 用 `kernel/x86_64/...`，aarch64 用 `kernel/aarch64/...`（`./litevm vmm download-kernel --arch <arch>`）。
- rootfs 架构必须与内核架构一致；跨架构构建见 [5.7](#57-交叉架构构建)。

---

## 2. 三条制作路线总览

| 路线 | 命令/途径 | 需要一个发行版整体能力 | 速度 | 适用场景 |
|------|-----------|----------------------|------|---------|
| **A. 官方 rootfs tarball** | 下载官方基础系统包 → 解压到目录 | 否（只需 tar + 目录） | 最快 | Alpine minirootfs、Ubuntu base、Void ROOTFS、Arch Linux bootstrap 快照 |
| **B. bootstrap 工具** | `debootstrap` / `pacstrap` / `dnf --installroot` / `zypper --root` / `xbps-install -r` / `apk --root` | 否（工具从宿主仓库跑） | 中 | 想要"干净的、最小的"基础系统，后续完全自控 |
| **C. litevm 半自动** | `./litevm pm make`（debootstrap/pacstrap）、`./litevm pm download`（下载 tarball） | 是（仍需按第 3、6、7 章配置 init） | 快 | 想偷懒但能接受发版口味 |

三条路线殊途同归：**最终产物都是一个 rootfs 目录**，之后的 init 配置（第 6、7 章）和 ext4 打包（第 9 章）完全一样。

```
官方tarball ─┐
bootstrap ───┼─→ /opt/mkrootfs/<distro> 目录 ──→ 配置 init/网络/密码 ──→ rootfs.ext4
litevm pm ───┘
```

---

## 3. rootfs 目录骨架与 /dev 设备节点

### 3.1 标准目录树

无论哪个发行版，最终 rootfs 目录都应该包含这些（缺的补上，这本身也是一种"手工构建 rootfs"的方式）：

```bash
cd /opt/mkrootfs
mkdir -p rootfs/{bin,boot,dev,etc,home,lib,lib64,media,mnt,opt,proc,root,run,sbin,srv,sys,tmp,usr,var}
# usr 内部再展开一层（现代发行版 usr 合并布局）
mkdir -p rootfs/usr/{bin,sbin,lib,lib64,share}
# 运行时需要可写的地方
mkdir -p rootfs/var/{log,run,cache,lib,tmp,spool}
mkdir -p rootfs/run/lock rootfs/dev/pts rootfs/dev/shm rootfs/tmp
chmod 1777 rootfs/tmp rootfs/dev/shm
chmod 755 rootfs rootfs/var rootfs/run
```

目录用途速记：

| 目录 | 作用 | 镜像中是否必须 |
|------|------|--------------|
| `/bin /sbin /usr/bin /usr/sbin` | 可执行程序 | 必须 |
| `/lib /lib64 /usr/lib /usr/lib64` | 动态库、内核模块、firmware | 必须 |
| `/etc` | 全部静态配置 | 必须 |
| `/dev` | 设备节点（devtmpfs 挂载后自动生成；下详） | 必须存在（可以是空目录） |
| `/proc /sys` | procfs / sysfs 挂载点 | 必须存在（空目录） |
| `/run` | 运行时数据（进程跑起来才能写） | 必须存在，init 里挂 tmpfs |
| `/tmp` | 临时文件 | 建议挂 tmpfs |
| `/var` | 日志、缓存、锁 | 建议 |
| `/home /root` | 用户数据 | 建议 |
| `/media /mnt /srv /opt /boot` | 惯例目录 | 可选 |

### 3.2 /dev 设备节点：devtmpfs vs 手工 mknod

- 现代发行版内核都编译了 `CONFIG_DEVTMPFS`（甚至 `CONFIG_DEVTMPFS_MOUNT`），你只要在 init 里 `mount -t devtmpfs devtmpfs /dev`，内核就会**自动填充**全部设备节点（ttyS0、vda、null、zero……），手工 `mknod` 通常不需要。
- 总线驱动器（virtio）配合 devtmpfs，`/dev/vda`、`/dev/vdb` 在 rootfs 挂载后马上可用。
- **不要**在 rootfs 目录里放进"挂载着宿主 /dev 的副本"（`cp -a /dev/.`），那会复制一堆宿主机专属节点与文件；正确做法是：rootfs 里 `/dev` 只是个空目录，运行时由 devtmpfs 填充。
- 如果你确实需要"无 devtmpfs 也能跑"（极老内核），至少要手工建这些节点：

  ```bash
  cd /opt/mkrootfs/rootfs/dev
  mknod -m 600 console c 5 1
  mknod -m 666 null   c 1 3
  mknod -m 666 zero   c 1 5
  mknod -m 666 random c 1 8
  mknod -m 666 urandom c 1 9
  mknod -m 666 ttyS0  c 4 64
  mknod -m 666 vda    b 254 0
  ```

### 3.3 /etc 关键文件清单（静态配置集中地）

| 文件 | 用途 | 谁生成 |
|------|------|--------|
| `/etc/inittab` | busybox/sysvinit 的 init 配置 | 手工 |
| `/etc/getty.sh / etc/gettytab` | getty 行为（一般不用改） | 发行版 |
| `/etc/init.d/` | OpenRC/SysV 服务脚本 | 发行版 |
| `/etc/rc.conf / etc/conf.d/*` | OpenRC 配置 | 发行版 |
| `/etc/fstab` | 静态挂载表（可选，init 脚本里 mount 也行） | 手工 |
| `/etc/passwd / etc/shadow / etc/group` | 账户数据库 | bootstrap 生成 |
| `/etc/os-release` | 发行版标识（litevm `detectDistroFamily` 依赖它） | bootstrap 生成 |
| `/etc/resolv.conf` | DNS | 手工/脚本 |
| `/etc/hostname`、`/etc/hosts` | 主机名与静态 hosts | 手工 |
| `/etc/localtime`、`/etc/timezone` | 时区 | 手工 |
| `/etc/ssh/sshd_config` | SSH 服务 | 手工 |

> 提示：litevm 的 chroot 模式、proot 模式和网络配置都会读 `/etc/os-release`、`/etc/debian_version`、`/etc/arch-release`、`/etc/redhat-release` 来判断发行版。用 tarball 或 bootstrap 生成的系统天然带这些文件，手工拼装的记着造一个 `/etc/os-release`。

---

## 4. 路线 A：官方 rootfs tarball（最快）

原理：发行版官方会发布"基础系统压缩包"，里面已经是一个完整的 rootfs 目录。下载、解压即可，之后直接进第 6、7 章配置。

### 4.1 Alpine minirootfs（示例：x86_64 / v3.21）

```bash
# 官方页面：https://alpinelinux.org/downloads/
# 或国内镜像：https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.21/releases/x86_64/

cd /opt/mkrootfs
wget https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.3-x86_64.tar.gz

mkdir -p alpine-rootfs
tar xzf alpine-minirootfs-3.21.3-x86_64.tar.gz -C alpine-rootfs

# 检查骨架是否完整
ls alpine-rootfs            # 应看到 bin dev etc home lib media mnt opt proc root run sbin srv sys tmp usr var
file alpine-rootfs/bin/busybox
```

特点：Alpine minirootfs 是**最小可运行系统**，自带 `busybox`（含 init、udhcpc、getty），开箱即有 `/sbin/init`（指向 /bin/busybox）——只需再补 inittab/getty/网络。

### 4.2 Ubuntu base（示例：26.04 base amd64）

```bash
# 官方：https://cdimage.ubuntu.com/ubuntu-base/releases/
# 仓库里已有示例产物：rootfs/ubuntu-base-26.04-base-amd64.tar.gz
cd /opt/mkrootfs
wget https://cdimage.ubuntu.com/ubuntu-base/releases/26.04/release/ubuntu-base-26.04-base-amd64.tar.gz

mkdir -p ubuntu-rootfs
tar xzf ubuntu-base-26.04-base-amd64.tar.gz -C ubuntu-rootfs
```

特点：Ubuntu base 是 debootstrap 的 second-stage 产物，**不含内核、init（systemd）也不完整**，用户需要 `chroot` 进去装 `systemd` 或用自定义 init（第 6、7 章）。

### 4.3 Void Linux rootfs（示例：x86_64 glibc）

```bash
# 仓库里已有示例产物：rootfs/void-x86_64-ROOTFS-20250202.tar.xz
cd /opt/mkrootfs
wget https://mirrors.tuna.tsinghua.edu.cn/voidlinux/live/current/void-x86_64-ROOTFS-20250202.tar.xz

mkdir -p void-rootfs
tar xJf void-x86_64-ROOTFS-20250202.tar.xz -C void-rootfs
ls void-rootfs/usr/bin/xbps-*   # 自带 xbps 包管理器
```

特点：Void ROOTFS 自带 runit init（/sbin/init → runit），配好 getty 即可直接跑。

### 4.4 Arch Linux bootstrap 快照（示例）

```bash
# 官方：https://archlinux.org/download/ 搜索 "bootstrap"
mkdir -p arch-rootfs
tar xzf archlinux-bootstrap-x86_64.tar.gz -C arch-rootfs
# 解压结果是 arch-rootfs/root.x86_64/，里面才是真正的 rootfs
mv arch-rootfs/root.x86_64 arch-rootfs-tmp && rmdir arch-rootfs && mv arch-rootfs-tmp arch-rootfs
```

特点：bootstrap 快照自带 `pacman`，但**没有 init 与多数程序**，参照 5.2 节补 `base`，或第 6/7 章手工配置。

### 4.5 通用解压注意事项

| 注意点 | 说明 |
|--------|------|
| 用 `tar -x`（保留属主/权限），**别用普通解压软件** | 需要 root，否则 /dev 设备节点、setuid 位都会丢 |
| 检查 tar 是否有目录前缀 | Alpine/Ubuntu/Void 无前缀；Arch bootstrap 有 `root.x86_64/` 前缀，要挪出来 |
| 解压后 `ls -la` 检查 `/dev` | 若节点丢失，用 3.2 节 mknod 补齐（Alpine/Ubuntu base 一般只有 console/null，够用了） |
| 校验 SHA256 | 官方页面提供 checksum，`sha256sum -c` |

---

## 5. 路线 B：用发行版 bootstrap 工具生成目录

通用套路（五步曲）：**工具准备 → bootstrap 到目录 → 补挂载 → chroot 配置 → 清理卸载**。
chroot 内的配置事项统一放第 6 章。

### 5.1 Debian / Ubuntu：debootstrap（推荐入门）

本仓库 `internal/images/rootfs-make.go` 的自动化路径就是它。

```bash
# 1) 安装工具（Debian/Ubuntu 宿主）
apt install -y debootstrap e2fsprogs rsync

# 2) 制作 Debian 12 (bookworm) x86_64 minbase
debootstrap --arch=amd64 --variant=minbase \
    --components=main \
    bookworm /opt/mkrootfs/debian-rootfs \
    http://deb.debian.org/debian

# 国内镜像加速（清华 / 中科大）
# debootstrap --arch=amd64 --variant=minbase bookworm /opt/mkrootfs/debian-rootfs \
#     https://mirrors.tuna.tsinghua.edu.cn/debian

# Ubuntu 24.04 (noble)
# debootstrap --arch=amd64 noble /opt/mkrootfs/ubuntu-rootfs \
#     http://archive.ubuntu.com/ubuntu
# 国内：https://mirrors.tuna.tsinghua.edu.cn/ubuntu
```

参数详解：

| 参数 | 含义 |
|------|------|
| `--arch=amd64` | 目标架构，可换 `arm64`（跨架构见 5.7） |
| `--variant=minbase` | 只有 apt/dpkg/基础库的最小 base；不写则更接近 default |
| `--components=main` | Ubuntu 至少 `main,restricted,universe`；Debian 主要 `main` |
| 最后参数 | 镜像源 URL（支持 HTTP/HTTPS） |

debootstrap 会自动完成**第一阶段+chroot 第二阶段**，产物是可直接使用的最小系统，自带 `/sbin/init`（sysvinit 或指向 systemd 的链接，取决于发行版）。

**跨架构 debootstrap**（x86_64 宿主做 arm64 Ubuntu）：

```bash
apt install -y qemu-user-static binfmt-support

debootstrap --arch=arm64 --foreign noble /opt/mkrootfs/ub-arm64 http://ports.ubuntu.com/ubuntu-ports
cp /usr/bin/qemu-aarch64-static /opt/mkrootfs/ub-arm64/usr/bin/
chroot /opt/mkrootfs/ub-arm64 /debootstrap/debootstrap --second-stage
```

### 5.2 Arch Linux：pacstrap

本仓库 `internal/images/rootfs-make.go` 的自动化路径之一（`sudo ./litevm pm make --distro arch --type standard`）。

```bash
# 1) 宿主安装
apt install -y arch-install-scripts     # Debian/Ubuntu；Arch 宿主自带
# 2) 生成目录
mkdir -p /opt/mkrootfs/arch-rootfs

# 3) 安装 base（--c = 在干净目录上生成，不假设 / 已挂载）
pacstrap -c /opt/mkrootfs/arch-rootfs base
```

> pacstrap 的 `base` 组包含 linux 内核+initramfs 等 microVM 用不到的大件，可用 `--exclude` 去掉：
> `pacstrap -c /opt/mkrootfs/arch-rootfs base --exclude linux,linux-firmware,linux-headers`
> microVM 的内核由 litevm 单独提供，rootfs 里不需要内核。

产物：含 `/sbin/init`（systemd 符号链接）、`pacman` 可用、`/etc/pacman.conf` 就绪的完整最小系统。

### 5.3 Alpine：apk --initdb

```bash
# 宿主有 apk-tools 时可直接建（否则用 4.1 的 tarball）
apk --initdb --allow-untrusted --arch x86_64 --root /opt/mkrootfs/alpine-rootfs \
  --repository https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.21/main \
  --repository https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.21/community \
  add alpine-base busybox musl libc-dev openrc

# 之后每次安装软件：
# chroot /opt/mkrootfs/alpine-rootfs /bin/sh -c 'apk add openssh'
```

`--initdb` 会生成 `/etc/apk/world`、密钥和基础目录；`--allow-untrusted` 跳过密钥校验方便引导。之后配置走第 6/7 章。

### 5.4 Fedora / RHEL / Rocky / Alma：dnf --installroot

```bash
# Fedora 40
dnf -y --releasever=40 --installroot=/opt/mkrootfs/fedora-rootfs \
  --disablerepo='*' --enablerepo=fedora \
  install systemd passwd dnf fedora-release rootfiles \
  vim-minimal openssh-server iproute dnf-plugins-core

# Rocky Linux 9
dnf -y --installroot=/opt/mkrootfs/rocky-rootfs \
  --releasever=9 --setopt=install_weak_deps=False \
  install rocky-release systemd passwd dnf vim-minimal openssh-server iproute
```

说明：

- **--installroot 目标目录必须为空或不存在**，否则报错。
- `systemd` 显式安装才有 PID 1；`passwd` 用于第 6 章设置 root 密码；rootfs 内再装包还要 `dnf`。
- `iproute`（ip 命令）在 init 脚本和调试里很常用。
- `--setopt=install_weak_deps=False` 能显著减小体积。

### 5.5 openSUSE：zypper --root

```bash
mkdir -p /opt/mkrootfs/opensuse-rootfs

# 添加仓库（以 Leap 15.6 清华镜像为例）
zypper --root /opt/mkrootfs/opensuse-rootfs \
  addrepo https://mirrors.tuna.tsinghua.edu.cn/opensuse/distribution/leap/15.6/repo/oss/ repo-oss
zypper --root /opt/mkrootfs/opensuse-rootfs \
  addrepo https://mirrors.tuna.tsinghua.edu.cn/opensuse/distribution/leap/15.6/repo/non-oss/ repo-nonoss

# 安装基础包
zypper --root /opt/mkrootfs/opensuse-rootfs --non-interactive \
  install systemd shadow iproute2 openssh-server

# 更新到一致状态
zypper --root /opt/mkrootfs/opensuse-rootfs --non-interactive dist-upgrade
```

### 5.6 Void Linux：xbps-install -r

直接推荐第 4.3 节的 `void-x86_64-ROOTFS-*.tar.xz`（自带 runit + 基础工具），若要手动构建：

```bash
mkdir -p /opt/mkrootfs/void-rootfs

# 初始化 xbps keyring
mkdir -p /opt/mkrootfs/void-rootfs/var/db/xbps/keys
cp /usr/share/xbps/keys/* /opt/mkrootfs/void-rootfs/var/db/xbps/keys/ 2>/dev/null || true

# 安装基础系统（可选，较慢）
XBPS_ARCH=x86_64 xbps-install -r /opt/mkrootfs/void-rootfs -Sy \
  --repository=https://repo-default.voidlinux.org/current \
  base-system
```

### 5.7 交叉架构构建

| 场景 | 解法 |
|------|------|
| debootstrap 跨架构 | 5.1 节 `--foreign` + `--second-stage` + `qemu-user-static` |
| pacstrap（Arch）跨架构 | 用 qemu-arch-static 或直接下 archlinuxarm bootstrap tarball |
| Alpine / Void / Ubuntu base 跨架构 | **直接下对应架构的官方 tarball（第 4 章），零成本** |

通用要点：

1. 进入 chroot 前 `cp /usr/bin/qemu-<arch>-static <rootfs>/usr/bin/`，并确认 `binfmt_misc` 使能（装 qemu-user-static + binfmt-support 自动生效）；
2. **做完所有 chroot 操作后必须删掉 qemu 文件**：`rm <rootfs>/usr/bin/qemu-*-static`，否则既增大镜像，也会在 guest 内产生无用的解释器。

---

## 6. chroot 内基础配置清单（所有发行版通用）

不管 rootfs 怎么来的，接下来都要进 chroot 做"移植手术"。**以下所有命令的 `<ROOTFS>` 都指你的 rootfs 目录**（如 `/opt/mkrootfs/debian-rootfs`）。

### 6.1 进入 chroot 前的挂载准备

chroot 里跑包管理器、设密码都需要 /proc /sys /dev：

```bash
ROOTFS=/opt/mkrootfs/debian-rootfs

mount --bind /dev   $ROOTFS/dev      # 借用宿主 /dev（注意第 3.2 节说的：仅开发期借用）
mount --bind /dev/pts $ROOTFS/dev/pts
mount -t proc proc  $ROOTFS/proc
mount -t sysfs sysfs $ROOTFS/sys
mount --bind /run   $ROOTFS/run      # 部分包管理器（systemd 系的 dnf/zypper）需要

# 需要网络时（绝大多数情况）：
# 不要拷宿主 resolv.conf 到镜像里！直接 bind：
mount --bind /etc/resolv.conf $ROOTFS/etc/resolv.conf
```

> chroot 期间使用宿主 /dev 只是开发期便利。**镜像打包前**要卸载所有 bind，且确保 rootfs 内的 /dev 恢复为"空目录/最小节点"状态（见 3.2）。

### 6.2 进入 chroot

```bash
chroot $ROOTFS /bin/sh    # 或 /bin/bash
```

进去了之后 `pwd` 是 `/`，看到的就是将来 guest 的根。**注意**：Debian/Ubuntu 的 chroot 里没有 init 常驻进程，命令执行完 `exit` 即可；systemd 系的发行版在 chroot 里跑 `systemctl` 会报错，属正常。

### 6.3 必做配置（在 chroot 内）

```sh
# 1) root 密码（必须！否则登录不了）
echo 'root:root' | chpasswd
#    或交互式：passwd root

# 2) 主机名与 /etc/hosts
echo "litevm" > /etc/hostname
cat > /etc/hosts <<'EOF'
127.0.0.1   localhost
::1         localhost ip6-localhost ip6-loopback
EOF

# 3) 时区
# Debian/Ubuntu：
ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
echo "Asia/Shanghai" > /etc/timezone
# Alpine/Void（需要 tzdata 包）：cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

# 4) fstab（可选，init 脚本里已 mount 时可空）
cat > /etc/fstab <<'EOF'
proc     /proc  proc   defaults 0 0
sysfs    /sys   sysfs  defaults 0 0
tmpfs    /tmp   tmpfs  mode=1777 0 0
tmpfs    /run   tmpfs  mode=0755,nosuid,nodev 0 0
EOF
```

### 6.4 各发行版 chroot 内额外安装（推荐软件集）

| 发行版 | 必装 | 建议 |
|--------|------|------|
| Debian/Ubuntu | （minbase 已含 apt/bash/coreutils） | `apt-get update && apt-get install -y --no-install-recommends vim-tiny iproute2 openssh-server systemd util-linux` |
| Arch | （base 已含） | `pacman -S --noconfirm openssh iproute2 vim` |
| Alpine | （已含 busybox） | `apk add --no-cache openssh openrc iproute2 util-linux` |
| Fedora/RHEL | （bootstrap 时已装） | `dnf install -y openssh-server iproute` |
| Void | （ROOTFS 自带 sshd） | `xbps-install -Sy openssh iproute2` |

> 仅当你想用 SSH 访问 guest 时才需要 openssh；只用串口登录可以不装。

### 6.5 清理与卸载（打包前必做）

```sh
# 退出 chroot 后
ROOTFS=/opt/mkrootfs/debian-rootfs

# 清理包管理器缓存，大幅缩小镜像
# （Debian/Ubuntu）已退出 chroot 的可用：chroot $ROOTFS apt-get clean
chroot $ROOTFS apt-get clean 2>/dev/null || true
chroot $ROOTFS rm -rf /var/lib/apt/lists/* 2>/dev/null || true
# Arch：pacman -Scc --noconfirm
# Alpine：apk cache clean

# 删除开发期借用物
rm -f  $ROOTFS/usr/bin/qemu-*-static   # 跨架构时
rm -f  $ROOTFS/etc/resolv.conf
# 或保留一个指向 172.16.0.1（dnsmasq）的 stub：
printf 'nameserver 172.16.0.1\n' > $ROOTFS/etc/resolv.conf

# 清空运行期目录
rm -rf $ROOTFS/run/* $ROOTFS/tmp/* $ROOTFS/var/tmp/* $ROOTFS/var/run/* 2>/dev/null

# 卸载所有挂载（必须按反序）
umount $ROOTFS/etc/resolv.conf 2>/dev/null
umount $ROOTFS/run 2>/dev/null
umount $ROOTFS/sys  2>/dev/null
umount $ROOTFS/proc 2>/dev/null
umount $ROOTFS/dev/pts 2>/dev/null
umount $ROOTFS/dev  2>/dev/null

# 确认没有残留挂载
mount | grep "$ROOTFS" || echo "clean"
```

> **为什么 resolv.conf 要这样处理**：如果把宿主的 `/etc/resolv.conf` 直接拷进去，镜像里会写死宿主 DNS（比如 systemd-resolved 的 127.0.0.53），guest 里根本不可用；而留一个 `nameserver 172.16.0.1`（litevm 的 dnsmasq）是最稳妥的默认。

---

## 7. init 系统详解（最核心章节）

### 7.0 内核如何找到 PID 1

内核挂载完根文件系统后，按以下顺序找首进程：

1. 命令行 `init=` 参数（如 `--kernel-args "init=/bin/sh"` 直进 shell 调试）；
2. 默认 `/sbin/init`；
3. 找不到则打印 `Kernel panic - not syncing: No working init found. Try passing init= option to kernel.`

因此**你的 rootfs 必须能提供一个 PID 1**。它有四种选择：

| 方案 | 代表 | 说明 | 适用 |
|------|------|------|------|
| **A. 自定义 /sbin/init 脚本** | 本文给出 | 自己挂载/起 getty，最可控、最通用 | 任何发行版，microVM 首选 |
| **B. BusyBox init + /etc/inittab** | Alpine 原生 | busybox 解释 inittab，简单稳定 | Alpine / 精简系统 |
| **C. 发行版原生 init** | systemd / openrc-init / runit | 完整服务管理，重 | Debian/Ubuntu/Fedora/Arch / Alpine+Void |
| **D. 静态单进程 init** | 手工拼装的 rootfs | 一个 shell 循环即可 | 极简场景 |

> litevm 文档与社区常见做法是 **方案 A**：一个 ~40 行的 /sbin/init 脚本，把 microVM 需要的"挂载 + 网络 + getty + init 探测"全部包圆，与具体发行版解耦。

### 7.1 方案 A：自定义 /sbin/init 脚本（推荐）

以下脚本是经过 litevm VMM 验证的通用 init，可放在任何发行版上（Alpine/Debian/Ubuntu/Arch/Fedora 都行），它做了四件事：

1. 挂载 /proc /sys /dev（devtmpfs）/dev/pts /tmp /run；
2. 设置主机名与 loopback；
3. 探测 eth0 并用 `udhcpc` 拿 DHCP（对应 litevm `--net` 注入的 `ip=dhcp` 内核参数）；
4. 依次探测可用的 init 系统（openrc-init → systemd → runit → openrc → getty 兜底）。

```sh
# 写入 <ROOTFS>/sbin/init
cat > <ROOTFS>/sbin/init <<'EOF'
#!/bin/sh
# litevm microVM 通用 init（PID 1）
export PATH=/sbin:/bin:/usr/sbin:/usr/bin

# ---- 1. 虚拟文件系统 ----
mount -t proc proc /proc 2>/dev/null
mount -t sysfs sysfs /sys 2>/dev/null
# devtmpfs：内核自动填充设备节点（vda、ttyS0、null……）
[ -e /dev/null ] || mount -t devtmpfs devtmpfs /dev 2>/dev/null
mkdir -p /dev/pts /dev/shm /run/lock
mount -t devpts devpts /dev/pts -o gid=5,mode=620 2>/dev/null
mount -t tmpfs tmpfs /tmp 2>/dev/null
mount -t tmpfs tmpfs /run 2>/dev/null
chmod 1777 /tmp /dev/shm 2>/dev/null

# ---- 2. 主机名与 loopback ----
hostname localhost 2>/dev/null
ip link set lo up 2>/dev/null
ip addr show lo | grep -q "127.0.0.1" || ip addr add 127.0.0.1/8 dev lo 2>/dev/null

# ---- 3. 网络：优先用内核 ip=dhcp 已配置好的 eth0 ----
if [ -d /sys/class/net/eth0 ]; then
    ip link set eth0 up 2>/dev/null
    if ! ip addr show eth0 | grep -q "inet "; then
        # 内核级 DHCP 没成功（或没传 ip=dhcp）时，用 busybox udhcpc 补上
        command -v udhcpc >/dev/null 2>&1 && \
            udhcpc -i eth0 -n -q -t 5 2>/dev/null
    fi
fi

# ---- 4. init 系统探测（按需接管） ----
if [ -x /sbin/openrc-init ]; then
    exec /sbin/openrc-init            # Alpine(新)/Void: openrc 独立 init
elif [ -d /run/systemd/system ] || [ -x /lib/systemd/systemd ]; then
    exec /lib/systemd/systemd         # Debian/Ubuntu/Fedora/Arch
elif [ -x /sbin/runit-init ]; then
    exec /sbin/runit-init             # Void: runit
elif [ -x /sbin/runit ]; then
    exec /sbin/runit
elif [ -x /sbin/openrc ]; then
    /sbin/openrc -o sysinit -o default 2>/dev/null
fi

# ---- 5. 兜底：直接起一个 root 串口 shell ----
exec /bin/sh
EOF
chmod +x <ROOTFS>/sbin/init
```

**逐段解释**（写作业时最容易被问到的点）：

| 段 | 为什么这样做 |
|----|------------|
| `[ -e /dev/null ] || mount devtmpfs` | 内核若已自动挂 devtmpfs（CONFIG_DEVTMPFS_MOUNT=y）就跳过；否则手动挂，保证 /dev/vda、/dev/ttyS0 存在 |
| `mount -t tmpfs tmpfs /run` | PID 1 及 systemd/udev 依赖 /run 可写；firecracker 默认没有 /run |
| `ip link set eth0 up` | 无 eth0 时命令失败但不致命（`2>/dev/null`） |
| `udhcpc` 分支 | litevm `--net` 已给内核传 `ip=dhcp`，多数情况内核自己配好了 eth0，这里只是兜底；udhcpc 来自 busybox（Alpine 自带）或 dhcpcd/dhclient |
| `exec` | 让 init 系统**取代**这个脚本成为 PID 1，保证信号语义正确（Ctrl+C 等会送给真正的 init） |
| 兜底 `/bin/sh` | 什么 init 都没有也能进 shell 调试，绝不"空转" |

> **如果只想跑一个能开机的 shell（不想管 init 系统）**，第 4 段可以直接干掉，改成：
> ```sh
> exec /sbin/getty -L 115200 ttyS0 vt100   # 然后你会获得一个 ttyS0 登录
> ```
> 或调试期直接 `exec /bin/sh`。

### 7.2 方案 B：BusyBox init + /etc/inittab（Alpine 原生）

BusyBox 自带 init 实现：当 `/sbin/init` 是 `/bin/busybox` 的符号链接时，busybox 会去读 `/etc/inittab`，为每个条目 fork 出对应进程，并在条目进程退出时按 `respawn` 重启它。**inittab 写好了，getty 就保证活着**。

Alpine minirootfs 默认就有 `/sbin/init -> /bin/busybox`，开箱即用；你只需确保 /etc/inittab 里有一条 ttyS0 的 getty。最小配置：

```
# <ROOTFS>/etc/inittab
::sysinit:/sbin/openrc sysinit
::sysinit:/sbin/openrc boot
::respawn:/sbin/getty -n -L 115200 ttyS0 vt100
::ctrlaltdel:/sbin/reboot
::shutdown:/sbin/umount -a -r
```

条目字段含义：`<id>:<runlevels>:<action>:<process>`。关键 action：

| action | 含义 |
|--------|------|
| `sysinit` | 系统初始化时只跑一次 |
| `respawn` | 进程退出后反复重启（getty 就用它） |
| `ctrlaltdel` | 收到 Ctrl-Alt-Del 信号时执行（firecracker 无该键，勿依赖） |
| `shutdown` | 关机时执行 |
| `wait` | 启动时执行并等待其结束 |
| `once` | 启动时执行一次，不等待 |

> `getty -n` 表示不读 /etc/issue 的 "login:" 提示的备用处理；`-L` 强制本地行（本地化登录提示）；`ttyS0` 与 litevm 的内核参数 `console=ttyS0` 对应，**绝不能写成 tty1**（tty1 不在串口上）。

### 7.3 方案 C1：openrc-init（Alpine ≥ 3.15 / Void）

OpenRC 从 0.50 起提供独立的 `openrc-init`，可替代 busybox init 作为 PID 1：

```sh
# chroot 内
# Alpine：apk add openrc
# Void：已默认含
# 确认 /sbin/openrc-init 存在
ls -l /sbin/openrc-init
```

这时 `/sbin/init` 应指向 `openrc-init`（或脚本里 `exec /sbin/openrc-init`）。它会按 runlevel：`sysinit`（挂载/udev）→ `boot` → `default`（服务）依次跑 `/etc/init.d/*`。Alpine 里 getty 服务名是 `agetty`（`rc-update add agetty default`）或写进 inittab 的 respawn。Void + runit 场景下一般用 runit，见 7.4。

```sh
# Alpine 用 openrc 作为 PID1 时的最小操作：
rc-update add devfs sysinit 2>/dev/null || true   # 挂 devtmpfs
rc-update add sysfs sysinit 2>/dev/null || true
rc-update add agetty default                      # serial getty（默认配置就监听 ttyS0）
```

### 7.4 方案 C2：runit（Void Linux 原生）

Void 的 `/sbin/init → runit-init`。runit 从 `/etc/runit/1`（一步启动）→ `/etc/runit/2`（常驻 supervisor）→ `/etc/runit/3`（关机）三个脚本驱动，服务目录在 `/etc/sv/`，用 `ln -s /etc/sv/<svc> /var/service/` 启用。

打 microVM 镜像是注意两点：

1. **确认 `/etc/runit/2` 里 `agetty` / `sulogin` 的 tty**：Void 默认 `agetty tty1`，要改成 `ttyS0`：
   ```sh
   sed -i 's/agetty tty1/agetty ttyS0 115200 vt100/' <ROOTFS>/etc/runit/runsvdir/default # 视版本而定
   ```
   或直接看 `/etc/sv/agetty-*`，把 run 脚本的 tty 参数改成 `ttyS0`。
2. **默认不自动登录 root**：`Empty` 密码/`root:root` 设好即可。

### 7.5 方案 C3：systemd（Debian/Ubuntu/Fedora/Arch 原生）

glibc 发行版的"正统"选择。在 microVM 上 systemd 完全可以跑，需要注意：

**① 启用串口登录**（否则没有登录提示符）：

```sh
# 在 rootfs 内（chroot 或挂载后）
# 创建自动登录 root 到 ttyS0 的配置
mkdir -p <ROOTFS>/etc/systemd/system/serial-getty@ttyS0.service.d/
cat > <ROOTFS>/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf <<'EOF'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --keep-baud 115200,57600,38400,9600 ttyS0 vt220
EOF

# 或仅在镜像里 enable 服务（不自动登录）：
chroot <ROOTFS> systemctl enable serial-getty@ttyS0
```

因为 litevm 默认内核参数带 `console=ttyS0`，systemd 的 getty-generator 通常会自动为 ttyS0 生成 serial-getty 单元；手动 enable 更保险。

**② 网络可选用 systemd-networkd**（第 8 章有完整配置）。

**③ 无害的告警不要慌**：没有 KVM/virtio 某些设备、cgroup 控制器受限时，systemd 会打印一堆：
```
[FAILED] Failed to start ... / Failed to create /init.scope control group ...
```
多数不影响使用；若 cgroup 报错刷屏，可加内核参数 `--kernel-args "systemd.unified_cgroup_hierarchy=0"` 或 `cgroup_no_v1=1` 缓解。

**④ 不要用 live/安装镜像的 systemd 单元**：直接 debootstrap/pacstrap/dnf --installroot 生产的系统单元是最干净的。

### 7.6 串口登录 getty 全配置（自动登录 root）

不管选哪个 init，最终目标都是：**开机后串口出现 `login:` 或直接是 root shell**。三套写法：

| init 体系 | 自动登录写法 |
|-----------|-------------|
| BusyBox inittab | `::respawn:/sbin/getty -L 115200 ttyS0 vt100`（`-n` 免登录提示，配合无密码账户） |
| openrc-init | 用 `agetty` 服务：`rc-update add agetty default`，默认监听 ttyS0（Void 需改 tty） |
| systemd | 7.5 ① 的 `serial-getty@ttyS0.service.d/autologin.conf` 片段 |
| 自定义 init | `exec /sbin/getty -L 115200 ttyS0 vt100` 或 `exec /bin/sh`（无需登录） |

> 若想 SSH：确保装了 openssh-server，`/etc/ssh/sshd_config` 显式写 `PermitRootLogin yes`、`PasswordAuthentication yes`；
> Debian/Ubuntu 还要 `systemctl enable ssh`（或启用 sshd）。同时 root 密码设好（6.3）。端口默认 22。

---

## 8. 网络配置（rootfs 侧）

litevm 的 `--net` 会做三件事（见 `internal/vmm/network.go`）：

1. 创建/复用 TAP 设备 `litevm-tap0`，宿主侧 IP `172.16.0.1/24`；
2. 启动 dnsmasq：DHCP 租约范围 `172.16.0.10 ~ 172.16.0.100`（12h），同时兼作 DNS；
3. 配置 iptables NAT（MASQUERADE），让 guest 能访问外网；
4. 给你传递 `--net` 时**额外追加内核参数 `ip=dhcp`**。

所以 guest 侧的"网络配置"其实非常轻：**只要 eth0 能被拉起 + DHCP 成功，就有网了**。下面给三种层次的配置方案。

### 8.1 方案一：什么都不做（内核级 `ip=dhcp`）

因为 litevm 已追加 `ip=dhcp`，**内核在 init 之前就完成了 eth0 的 DHCP 配置**（前提：内核编译了 CONFIG_IP_PNP_DHCP，firecracker 官方内核都带）。此时 rootfs 里唯一要做的是：

- 别把 eth0 弄 down；
- `ip link set lo up`（/sbin/init 脚本第 2 段已做）。
- 查看验证：进 guest 后 `ip addr show eth0` 应有 172.16.0.x。

缺点：内核 DHCP 不写 /etc/resolv.conf（DNS 需要自己配或交给 8.2 的 udhcpc）。

### 8.2 方案二：init 脚本里 udhcpc（最通用）

对应 7.1 自定义 init 的第 3 段。依赖 `udhcpc`（busybox 自带）或 `dhcpcd`/`dhclient`：

```sh
# Debian/Ubuntu 的 init 脚本里：
command -v udhcpc >/dev/null 2>&1 && udhcpc -i eth0 -n -q -t 5 || true
# 没有 udhcpc 时：
# dhclient eth0  (ifupdown/isc-dhcp-client)
# dhcpcd eth0   (dhcpcd5)
```

关于 DNS：

- busybox `udhcpc` 的默认 `deconfig/default.script` 会把 DNS 写进 `/etc/resolv.conf`，恰好 dnsmasq 同时提供 DNS，一切自动。
- 兜底手动写：

```sh
printf 'nameserver 172.16.0.1\n' > /etc/resolv.conf
```

### 8.3 方案三：systemd-networkd / NetworkManager（glibc 发行版）

systemd 系发行版想用现代管理工具：

```ini
# <ROOTFS>/etc/systemd/network/10-eth0.network
[Match]
Name=eth0

[Network]
DHCP=yes
```

启用：

```sh
chroot <ROOTFS> systemctl enable systemd-networkd
chroot <ROOTFS> systemctl enable systemd-resolved   # 可选，DNS 解析
# 用 resolved 时把 /etc/resolv.conf 做成指向它的符号链接：
# ln -sf ../run/systemd/resolve/stub-resolv.conf <ROOTFS>/etc/resolv.conf
```

NetworkManager（桌面发行版重）：装 `NetworkManager` 后 `nmcli device` 会自动接管 eth0（默认连接 autoconnect 启），一般无需配置。

### 8.4 静态 IP 备用（无 DHCP 时）

litevm 的网络设备（eth0/NIC）**只有在 `--net` 时才会附加**给 Firecracker，且 `--net` 时 `buildConfig` 会固定追加 `ip=dhcp`（即使你显式写了 `--kernel-args` 也会追加）。因此"完全静态、不经过 DHCP"在 litevm 里没有干净的通道，一般**不需要**——dnsmasq 的租约在 172.16.0.10~100 内基本不变。

若仍要固定 IP，推荐**在 guest 内覆盖**（网络设备照常来自 `--net`，开机后把 eth0 改成静态）：

```bash
# 方案一：init 脚本内（7.1 第 3 段之后）
ip addr flush dev eth0 2>/dev/null
ip addr add 172.16.0.2/24 dev eth0
ip route add default via 172.16.0.1 dev eth0

# 方案二：systemd-networkd
# /etc/systemd/network/10-eth0.network
[Match]
Name=eth0

[Network]
Address=172.16.0.2/24
Gateway=172.16.0.1
DNS=172.16.0.1
```

> 也可以给内核传一个**非标准的静态 ip=**（如 `ip=172.16.0.2::172.16.0.1:255.255.255.0::eth0:off`），但注意 litevm 在 `--net` 时还会追加 `ip=dhcp`，多次 `ip=` 的内核解析规则是"后到者为准"，最终仍是 DHCP 生效，所以别指望用内核参数摆脱 DHCP。

### 8.5 网络排错速查

| 现象 | 排查顺序 |
|------|---------|
| guest 无 IP | 是否加了 `--net`？（root 或预建 TAP 才允许）→ `sudo ./litevm vmm setup-network` → `ip link show eth0` |
| DHCP 超时 | dnsmasq 是否在跑：`pgrep -a dnsmasq`；`sudo ./litevm vmm setup-network` 重来 |
| 能 ping 通 172.16.0.1 但上不了网 | 宿主防火墙/NAT：`sudo ip route`、`iptables -t nat -L POSTROUTING`；确认 `--net` 参数主网卡被自动识别 |
| DNS 不通 | 检查 /etc/resolv.conf 是否写死宿主 IP；改 `nameserver 172.16.0.1` |
| 静态 IP 冲突 | 手工写脚本把 IP 固定到 172.16.0.10~100 之外 |

---

## 9. 生成 rootfs.ext4 镜像

rootfs 目录就绪后，把它变成 litevm 能用的 ext4 磁盘。共四步：**建空白文件 → 格式化 → 灌数据 → 校验**。

### 9.1 大小规划

| 用途 | 建议大小 |
|------|---------|
| 最小（自定义 init + busybox，无 SSH） | 256M ~ 512M |
| 标准（systemd 发行版 + ssh + 包管理器） | 2G（项目默认推荐，仓库文档历史推荐 2GB） |
| 带开发工具（gcc/make/git + 完整 locale） | 4G ~ 8G |

判据：镜像大小 = 数据量 + **运行期写入余量**（日志、apt 缓存、SSH host key）。Firecracker 的 rootfs 是稀疏文件友好型，用 `truncate` 建大文件不占实际磁盘：

```bash
# 建一个"看起来 2G、实际占几个字节"的文件
truncate -s 2G rootfs/rootfs.ext4
du -h rootfs/rootfs.ext4    # 显示实际占用（此时几乎为 0）
```

> 不要贪小建 256M 又装完整 systemd 发行版——开机 udev/systemd 写爆 inode 会 `No space left on device`，一切归零重来。

### 9.2 四种建空白镜像的方法

| 方法 | 命令 | 优点 | 缺点 |
|------|------|------|------|
| A. truncate | `truncate -s 2G rootfs.ext4` | 秒级、稀疏 | 对文件内容先写后读的极端场景略慢 |
| B. dd | `dd if=/dev/zero of=rootfs.ext4 bs=1M count=2048` | 全零实写，最保险 | 写 2G 慢 |
| C. qemu-img | `qemu-img create -f raw rootfs.ext4 2G` | 与 litevm `CreateRawDisk`（vmm.go）同一套路；也是 `--drive` 非 raw 转换的同款工具 | 需装 qemu-utils |
| D. fallocate | `fallocate -l 2G rootfs.ext4` | 快、预分配 | 对某些文件系统不可用 |

```bash
mkdir -p rootfs
# 推荐 A（快且省空间）
truncate -s 2G rootfs/rootfs.ext4

# litevm 内部 CreateRawDisk 的做法是：
#   qemu-img create -f raw rootfs/rootfs.ext4 2G
#   mkfs.ext4 -F -q rootfs/rootfs.ext4
```

### 9.3 格式化：mkfs.ext4 参数详解

```bash
mkfs.ext4 -F -L litevm-rootfs -m 1 rootfs/rootfs.ext4
```

| 参数 | 含义 | 建议 |
|------|------|------|
| `-F` | 强制格式化（跳过交互确认） | 必须 |
| `-L <label>` | 卷标（可选） | 便于 `blkid`/`tune2fs` 识别 |
| `-m 1` | 保留块比例 1%（默认 5%） | 大镜像可省空间 |
| `-d <目录>` | 直接从目录灌数据（见 9.4 方法 B） | 免挂载神器 |
| `-O ^has_journal` | 关闭日志 | 一般不推荐：日志对崩溃恢复有用 |
| `-E discard` | 对稀疏文件丢弃未用块 | firecracker 快照友好，可加 |
| `-q` | 安静模式 | 脚本友好 |

> litevm 内置建盘（`CreateRawDisk`）用的是 `mkfs.ext4 -F -q`，与上面的等价且足够。

### 9.4 灌数据：三种方法任选

**方法 A：loop 挂载 + cp（最通用）**

```bash
mkdir -p /mnt/rootfs
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs
cp -a /opt/mkrootfs/debian-rootfs/. /mnt/rootfs/
sync
umount /mnt/rootfs
```

- `cp -a` 保留权限、属主、符号链接、设备节点。
- `rsync -aHAX /opt/mkrootfs/debian-rootfs/ /mnt/rootfs/` 效果等价，还支持增量与进度（`--numeric-ids` 建议加，避免宿主/guest UID 映射混乱）。
- 宿主没有 loop 权限时：`modprobe loop`；容器里常用 `losetup -f`。

**方法 B：mkfs.ext4 -d 直装（免挂载，e2fsprogs ≥ 1.43）**

```bash
mkfs.ext4 -F -d /opt/mkrootfs/debian-rootfs rootfs/rootfs.ext4
```

一步完成格式化+拷贝，不需要 loop 设备。注意：`-d` 仍需要 root（要保留属主与设备节点）；对超长符号链接与特殊文件支持良好。

**方法 C：tar 管道（适合先打包再移植）**

```bash
tar -C /opt/mkrootfs/debian-rootfs -cf - . \
  | ( mount -o loop rootfs/rootfs.ext4 /mnt/rootfs && tar -xf - -C /mnt/rootfs && umount /mnt/rootfs )
```

**灌完必做的几个小动作**（镜像内）：

```bash
# 若还没做过（一般 6.3 已做），确保：
#   <镜像>/sbin/init 存在且可执行
#   <镜像>/etc/inittab 或 systemd drop-in 里 ttyS0 getty 就绪
#   root 密码已设置
# 检查关键文件
ls -l /mnt/rootfs/sbin/init
cat /mnt/rootfs/etc/inittab 2>/dev/null
```

### 9.5 收尾：sync、umount、e2fsck 验证

无论用哪种方法灌数据，最后都必须：

```bash
sync                                    # 确保脏页落盘
umount /mnt/rootfs 2>/dev/null          # 方法 B/D 无挂载，跳过
# 卸载后强制校验（等价"新车落地凿个章"）
e2fsck -f rootfs/rootfs.ext4
```

`e2fsck -f` 输出 `clean, ... files, ... blocks` 即合格；有修复项也能继续用。**切勿在挂载状态下 e2fsck**。

### 9.6 运行期调整：resize2fs / tune2fs

镜像建小/建大都可事后调整：

```bash
# 变大（挂载着也能扩）
truncate -s 4G rootfs/rootfs.ext4
resize2fs rootfs/rootfs.ext4

# 变小（必须先 e2fsck -f，且目标 ≥ 数据量）
e2fsck -f rootfs/rootfs.ext4
resize2fs rootfs/rootfs.ext4 1500M

# 关闭启动强制检查（常用）
tune2fs -c 0 -i 0 rootfs/rootfs.ext4

# 查看元数据
tune2fs -l rootfs/rootfs.ext4 | head -20
blkid rootfs/rootfs.ext4
```

### 9.7 镜像体检（打包后、开机前）

不挂载也能查：

```bash
# 1. 文件系统类型
file rootfs/rootfs.ext4
#    输出应含 "Linux rev 1.0 ext4 filesystem data ... UUID=..."
#    若显示 "data" 或 "ext2"，检查你是否真的 mkfs.ext4 过

# 2. 直接查 /sbin/init 是否存在（debugfs 免挂载）
debugfs -R 'ls -l /sbin/init' rootfs/rootfs.ext4

# 3. 列根目录
debugfs -R 'ls /' rootfs/rootfs.ext4

# 4. 挂载抽查（最像 guest 的视角）
mkdir -p /mnt/check
mount -o loop rootfs/rootfs.ext4 /mnt/check
ls -la /mnt/check/sbin/init
cat /mnt/check/etc/inittab 2>/dev/null
chroot /mnt/check /bin/sh -c 'echo chroot-ok'   # 确认动态库/解释器都没问题
umount /mnt/check
```

> 这个 `chroot /mnt/check /bin/sh -c ...` 的测试等效于"内核挂根成功后会遇到的第一个问题"：init 能不能加载？此时 chroot 成功说明动态链接器(libc)、/bin/sh 都正常。

---

## 10. 一键脚本模板

把上面的流程串成一个脚本：给定一个**已配置好 init/网络/密码**的 rootfs 目录，产出 rootfs.ext4。

```bash
#!/bin/bash
# scripts/make-ext4.sh <rootfs-dir> <output.ext4> [size] [label]
# 示例：sudo bash make-ext4.sh /opt/mkrootfs/debian-rootfs rootfs/rootfs.ext4 2G
set -euo pipefail

SRC="${1:?用法: $0 <rootfs-dir> <output.ext4> [size] [label]}"
OUT="${2:?用法: $0 <rootfs-dir> <output.ext4> [size] [label]}"
SIZE="${3:-2G}"
LABEL="${4:-litevm-rootfs}"

test -d "$SRC" || { echo "错误: $SRC 不是目录"; exit 1; }
command -v mkfs.ext4 >/dev/null || { echo "缺少 e2fsprogs"; exit 1; }

mkdir -p "$(dirname "$OUT")"
rm -f "$OUT"

echo "==> 创建空白镜像 ($SIZE)"
truncate -s "$SIZE" "$OUT"

echo "==> 格式化并直装 rootfs (mkfs.ext4 -d)"
mkfs.ext4 -F -q -L "$LABEL" -m 1 -d "$SRC" "$OUT"

echo "==> 强制校验"
e2fsck -f -p "$OUT"

echo "==> 完成"
ls -lh "$OUT"
file "$OUT"
```

> 依赖 e2fsprogs ≥ 1.43（2016 年后的发行版都满足）。不想用 `-d` 时，把 `mkfs.ext4 -d` 换成 9.4 的方法 A（mount + cp -a）即可。

若还想"一条命令从零做 Alpine 最小系统"，可把它与第 4 章合体：

```bash
#!/bin/bash
set -euo pipefail
# make-alpine-mini.sh <output.ext4> [size]
OUT="${1:?用法: $0 <output.ext4> [size]}"; SIZE="${2:-512M}"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

wget -q -O "$TMP/apk.tar.gz" \
  https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.3-x86_64.tar.gz
tar xzf "$TMP/apk.tar.gz" -C "$TMP/root"

# 基础配置
echo 'root:root' | chroot "$TMP/root" chpasswd
echo litevm > "$TMP/root/etc/hostname"
printf '::respawn:/sbin/getty -L 115200 ttyS0 vt100\n::ctrlaltdel:/sbin/reboot\n' \
  > "$TMP/root/etc/inittab"

# 交给第 7.1 节的通用 init（此处给一个最小版）
cat > "$TMP/root/sbin/init" <<'EOF'
#!/bin/sh
export PATH=/sbin:/bin:/usr/sbin:/usr/bin
mount -t proc proc /proc 2>/dev/null
mount -t sysfs sysfs /sys 2>/dev/null
[ -e /dev/null ] || mount -t devtmpfs devtmpfs /dev 2>/dev/null
ip link set lo up 2>/dev/null
ip link set eth0 up 2>/dev/null
udhcpc -i eth0 -n -q -t 5 2>/dev/null || true
exec /bin/sh
EOF
chmod +x "$TMP/root/sbin/init"

# 出镜像
truncate -s "$SIZE" "$OUT"
mkfs.ext4 -F -q -d "$TMP/root" "$OUT"
e2fsck -f -p "$OUT"
echo "生成完成: $OUT"
```

---

## 11. 验证与首次启动

### 11.1 镜像体检（已并入 9.7）

```bash
file rootfs/rootfs.ext4                       # Linux rev 1.0 ext4 filesystem
debugfs -R 'ls /sbin/init' rootfs/rootfs.ext4 # init 在场
```

### 11.2 准备内核与网络（一次性）

```bash
# 1) 下载 Firecracker 官方内核（交互式选架构/版本）
./litevm vmm download-kernel

# 2) 环境自检（KVM + firecracker）
./litevm --check vmm

# 3) 配置 TAP/NAT/DHCP（需 root，一次性；systemd 服务会自动维护）
sudo ./litevm vmm setup-network
```

`setup-network` 自动完成（与 `network.go` 一致）：
- 创建 `litevm-tap0` 并配 `172.16.0.1/24`；
- 启动 dnsmasq（DHCP 172.16.0.10~100 + DNS）；
- 配 iptables NAT；
- 放权 `/dev/kvm`、`/dev/net/tun`。

### 11.3 启动 VM

```bash
./litevm vmm run \
  --kernel kernel/x86_64/vmlinux-6.18.44 \
  --rootfs rootfs/rootfs.ext4 \
  --net
```

期望看到的启动画面（顺序）：

```
Welcome to litevm!                          ← litevm 自身横幅
[    0.000000] Linux version 6.18.44 ...    ← 内核日志（loglevel=5）
[    ...    ] VFS: Mounted root (ext4 filesystem) readonly.
...
login:                                     ← 你的 init/getty 接管（或直接 root shell）
```

进入 guest 后建议马上验证：

```bash
# 基本运行
mount | head
ps aux | head

# 网络（若 --net）
ip addr show eth0        # 应有 172.16.0.x（dnsmasq 分配）
ping -c2 172.16.0.1      # 通 = TAP 正常
ping -c2 223.5.5.5       # 通 = NAT 正常
cat /etc/resolv.conf     # 应有 nameserver

# 关机验证（litevm 应自动退出）
poweroff    # 或 reboot；systemd 发行版用 systemctl poweroff
```

> 快捷键：连按 `Ctrl+A` 然后按 `x` 退出 litevm（不会关 guest）；guest 内关机用 `poweroff`。

### 11.4 SSH 访问（可选）

```bash
# guest 里确认 sshd 已起；宿主上：
ssh root@<guest-ip>      # guest-ip = 8.5 里查到的 172.16.0.x
# 密码就是 6.3 设的 root 密码
```

---

## 12. 常见发行版速查表

| 发行版 | 推荐来源 | 制作命令（核心） | 默认 init | 关键 microVM 配置 |
|--------|---------|-----------------|-----------|------------------|
| **Alpine** | minirootfs tarball | 见 4.1 / 4.5 | busybox/openrc | inittab 加 ttyS0 getty；`apk add openrc openssh` |
| **Debian** | debootstrap | `debootstrap minbase bookworm <dir> <mirror>` | systemd/sysvinit | 若 minbase 未带 systemd 则 `apt install systemd`；serial-getty@ttyS0 drop-in |
| **Ubuntu** | Ubuntu base tarball / debootstrap | 4.2 / 5.1 | systemd | 同 Debian；minbase 时不带内核是正常的 |
| **Arch** | pacstrap | `pacstrap -c <dir> base` | systemd | serial-getty@ttyS0；国内镜像测网速选源 |
| **Fedora** | dnf --installroot | 5.4 | systemd | SELinux 若报障加 `selinux=0`；serial getty |
| **Rocky/Alma** | dnf --installroot | 5.4 | systemd | 同 Fedora；`--setopt=install_weak_deps=False` |
| **openSUSE** | zypper --root | 5.5 | systemd | 同 Fedora；仓库必须有 repo-oss |
| **Void** | ROOTFS tarball | 4.3 | runit | `/etc/sv/agetty-*` 改 ttyS0；openrc 可选 |

所有发行版做完后**统一动作**：

```bash
# 设 root 密码、写 /sbin/init（或启用 getty）、清缓存、卸载挂载
echo 'root:root' | chroot <ROOTFS> chpasswd
# ...（第 6、7 章全套）
# 出镜像
truncate -s 2G rootfs/rootfs.ext4
mkfs.ext4 -F -d <ROOTFS> rootfs/rootfs.ext4
e2fsck -f rootfs/rootfs.ext4
```

---

## 13. 常见问题排查（FAQ）

### 13.1 `VFS: Unable to mount root fs on unknown-block(254,0)`

镜像不是有效的 ext4，或内核没有 EXT4 支持。

```bash
file rootfs/rootfs.ext4
# 需要："Linux rev 1.0 ext4 filesystem"
# 修复：mkfs.ext4 -F rootfs/rootfs.ext4 重做（会清空，先备份）
```

### 13.2 `can't run '/sbin/init': No such file or directory`

三种原因：

1. `/sbin/init` 真的不存在 → `debugfs -R 'ls -l /sbin/init' rootfs.ext4`；
2. `/sbin/init` 是脚本但 shebang 解释器缺失（如 `#!/bin/bash` 但没装 bash）→ 用 `#!/bin/sh` 或装 bash；
3. `/sbin/init` 是动态链接程序但缺 libc → `chroot <镜像> /bin/sh -c 'ldd /sbin/init'` 排查，或换静态 busybox。

### 13.3 `Kernel panic - not syncing: Attempted to kill init!`

init 进程退出了（脚本第一行写错、`exec` 的程序不存在、无限循环后非法退出）。对策：先用 `--kernel-args "init=/bin/sh"` 进 shell 排错：

```bash
./litevm vmm run --kernel <k> --rootfs rootfs/rootfs.ext4 \
  --kernel-args "console=ttyS0,115200n8 reboot=k panic=1 nomodule init=/bin/sh"
```

### 13.4 串口没有任何输出

- 确认内核参数里 `console=ttyS0,115200n8`（litevm 默认有，显式 `--kernel-args` 会**覆盖**默认，务必带上）；
- 确认你用的是 `kernel/` 下的 vmlinux 而非发行版自带 bzImage；
- 确认 rootfs 能挂载（否则是 13.1）。

### 13.5 开机后有内核日志但永远没有 `login:`

init 起来了，但没 spawn getty。按第 7.6 节查：inittab 的 tty 是不是写成了 tty1？systemd 的 serial-getty@ttyS0 enable 了吗？自定义 init 里 exec 到 `/bin/sh` 了吗？

### 13.6 进去了却 `poweroff` 卡住

litevm 靠串口标记收尾（`System halted` / `Power down` 等，见 0.1）。若卡住：

```bash
poweroff -f         # 强制立即关机（sysvinit/busybox 支持 -f）
echo o > /proc/sysrq-trigger   # 内核级紧急关机（debug 兜底）
# 或宿主侧 Ctrl+A 再按 x 退出 litevm
```

### 13.7 systemd 刷 cgroup 报错（Failed to create /init.scope）

firecracker cgroup 受限导致的无害告警。不等它结束也能进系统；想消音可加：
`--kernel-args "... systemd.unified_cgroup_hierarchy=0"`。

### 13.8 Fedora/RHEL 系 SELinux 拒绝启动服务

rootfs 里 SELinux 上下文未打标签（没有 initramfs 的 restorecon 阶段）。最简单：
`--kernel-args "... selinux=0"`，或在镜像内 `touch /.autorelabel && setenforce 0`。

### 13.9 镜像不断涨大 / 空间浪费

- 容器镜像/包缓存占空间：`apt-get clean` / `pacman -Scc` / `dnf clean all`；
- 想压缩体积：调整 inode 数量 `mkfs.ext4 -i 4096`、`-m 1`、去掉日志 `-O ^has_journal`；
- 运行时随时扩：9.6 的 `truncate + resize2fs`。

### 13.10 `mount -o loop` 说没有 loop 设备

```bash
modprobe loop
losetup -f   # 确认有可用 loop
# 或干脆用 9.4 方法 B：mkfs.ext4 -d（免挂载）
```

### 13.11 非 root 用户跑 `vmm run --net` 报权限

litevm 要求 `--net` 时 root 或 TAP 已预建可访问：

```bash
# 方式一：用 root 跑
sudo ./litevm vmm run --kernel ... --rootfs ... --net
# 方式二：一次性把 TAP/kvm 权限交给当前用户（docs 里有详细步骤）
sudo ./litevm vmm setup-network
./litevm vmm run --kernel ... --rootfs ... --net
```

### 13.12 需要 initramfs 的功能（加密盘、网络根、特殊驱动）

Firecracker 不支持 initrd。对策：**全部编译进内核**（firecracker 官方内核已含 virtio/ext4/devtmpfs）；需要自定义驱动时自己编内核并启用对应 CONFIG，而不是挂 initramfs。

### 13.13 guest 里报 "No space left on device"

```bash
df -h; df -i        # 块/inode 双查
# 打 bootstrap 时 minbase 已最小；运行期注意 /var/log、/var/cache
# 硬扩：truncate + resize2fs（9.6）
```

---

## 14. 参考

**本仓库代码（行为基准）**

- `internal/vmm/vmm.go`：内核参数、关机/重启标记、磁盘格式、`CreateRawDisk`、默认路径
- `internal/vmm/network.go`：TAP / dnsmasq DHCP 范围 / NAT / 发行版检测
- `internal/vmm/download.go`：Firecracker S3 内核下载（x86_64 / aarch64）
- `internal/images/rootfs-make.go`：debootstrap / pacstrap 自动化构建（`./litevm pm make`）
- `cmd/litevm/main.go`：`vmm run / download-kernel / setup-network / rm-network` 参数

**文档**

- 本仓库 `README.md`：VMM 模式快速开始
- `docs/` 其余文档

**外部资源**

- Firecracker 官方 quickstart（rootfs 要求、内核配置建议）：<https://github.com/firecracker-microvm/firecracker/tree/main/docs>
- debootstrap：<https://wiki.debian.org/Debootstrap>
- Alpine minirootfs 下载：<https://alpinelinux.org/downloads/>
- Ubuntu base：<https://cdimage.ubuntu.com/ubuntu-base/>
- Arch bootstrap / pacstrap：<https://wiki.archlinux.org/title/Install_Arch_Linux_on_a_removable_medium>
- Void Linux ROOTFS：<https://repo-default.voidlinux.org/live/current/>
- e2fsprogs（mkfs.ext4 / e2fsck / debugfs）：<https://e2fsprogs.sourceforge.net/>

---

## 附录：最小可运行 rootfs 清单（核对单）

打包完成、开机前 30 秒，照着打勾：

```text
[ ] file rootfs.ext4 显示 "Linux rev 1.0 ext4 filesystem"
[ ] debugfs -R 'ls /sbin/init' rootfs.ext4 有输出
[ ] rootfs 里 root 密码已设（6.3）
[ ] ttyS0 有 getty（7.6：inittab / serial-getty@ttyS0 / 自定义 init exec）
[ ] /proc /sys /dev 会被挂载（7.1 init 脚本第 1 段，或发行版 init 自带）
[ ] 若需联网：--net 参数 + rootfs 侧 eth0 DHCP（第 8 章）
[ ] 无 /usr/bin/qemu-*-static 残留（跨架构时）
[ ] rootfs 目录无 bind 残留：mount | grep <ROOTFS> 为空
[ ] e2fsck -f 通过
[ ] ./litevm --check vmm 全部 Passed
```

---

*文档维护：与 `internal/vmm/` 源码同步更新。发现不一致时以代码为准。*
