# 创建 rootfs.ext4 虚拟磁盘

本文档说明如何创建用于 Firecracker microVM 的 rootfs.ext4 虚拟磁盘。

## 前置条件

```bash
# 需要 root 权限
sudo -i

# 安装必要工具
apt install -y debootstrap e2fsprogs     # Debian/Ubuntu
pacman -S debootstrap e2progs            # Arch
apk add e2fsprogs                        # Alpine
```

## 方法一：使用 debootstrap（推荐）

### 1. 创建基础系统

```bash
# 安装 debootstrap（如果没有）
apt install -y debootstrap

# 创建 Alpine 最小系统
debootstrap --arch=amd64 --variant=minbase alpine /tmp/rootfs http://dl-cdn.alpinelinux.org/alpine/v3.21/main

# 或者创建 Debian 最小系统
# debootstrap --arch=amd64 bookworm /tmp/rootfs http://deb.debian.org/debian
```

### 2. 进入 chroot 配置系统

```bash
mount --bind /dev /tmp/rootfs/dev
mount --bind /proc /tmp/rootfs/proc
mount --bind /sys /tmp/rootfs/sys
chroot /tmp/rootfs /bin/sh
```

### 3. 在 chroot 中安装必要软件

```bash
# 设置 APK 源（Alpine）
echo "https://dl-cdn.alpinelinux.org/alpine/v3.21/main" > /etc/apk/repositories
echo "https://dl-cdn.alpinelinux.org/alpine/v3.21/community" >> /etc/apk/repositories

# 基础系统
apk add --no-cache \
  alpine-base \
  busybox \
  openrc \
  util-linux \
  musl \
  libc6-compat

# 网络工具
apk add --no-cache \
  iproute2 \
  iptables \
  curl \
  wget

# 开发工具（可选）
apk add --no-cache \
  gcc \
  make \
  git

# Shell 工具
apk add --no-cache \
  bash \
  shadow

# 配置 root 密码
echo "root:root" | chpasswd

# 启用 getty 自动登录
mkdir -p /etc/init.d
cat > /etc/inittab << 'EOF'
::sysinit:/sbin/openrc -o sysinit -o default
::respawn:/sbin/getty -n -L 115200 ttyS0 vt100
::ctrlaltdel:/sbin/reboot
EOF

exit
```

### 4. 清理并卸载

```bash
umount /tmp/rootfs/dev
umount /tmp/rootfs/proc
umount /tmp/rootfs/sys
```

### 5. 创建 ext4 镜像

```bash
# 创建 1GB 空白镜像
dd if=/dev/zero of=rootfs/rootfs.ext4 bs=1M count=1024

# 格式化为 ext4
mkfs.ext4 -F rootfs/rootfs.ext4

# 挂载并复制文件
mkdir -p /mnt/rootfs
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs
cp -a /tmp/rootfs/* /mnt/rootfs/

# 配置网络接口
cat > /mnt/rootfs/etc/network/interfaces << 'EOF'
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
EOF

# 清理
umount /mnt/rootfs
```

## 方法二：手动创建（更精细控制）

```bash
# 1. 创建空白镜像
dd if=/dev/zero of=rootfs/rootfs.ext4 bs=1M count=1024
mkfs.ext4 -F rootfs/rootfs.ext4

# 2. 挂载
mkdir -p /mnt/rootfs
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs

# 3. 安装 Alpine 最小系统（使用 apk --initdb）
apk --initdb --allow-untrusted --arch x86_64 --root /mnt/rootfs \
  --repository https://dl-cdn.alpinelinux.org/alpine/v3.21/main \
  add alpine-base busybox musl

# 4. 配置系统
cat > /mnt/rootfs/etc/inittab << 'EOF'
::sysinit:/sbin/openrc -o sysinit -o default
::respawn:/sbin/getty -n -L 115200 ttyS0 vt100
::ctrlaltdel:/sbin/reboot
EOF

# 5. 配置网络
cat > /mnt/rootfs/etc/network/interfaces << 'EOF'
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
EOF

# 6. 设置 root 密码
chroot /mnt/rootfs passwd root
# 输入密码：root

# 7. 卸载
umount /mnt/rootfs
```

## 方法三：从现有 rootfs 压缩包转换

如果有 rootfs 压缩包（如 alpine-minirootfs）：

```bash
# 下载 Alpine 最小 rootfs
wget https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz

# 创建镜像
dd if=/dev/zero of=rootfs/rootfs.ext4 bs=1M count=512
mkfs.ext4 -F rootfs/rootfs.ext4

# 挂载并解压
mkdir -p /mnt/rootfs
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs
tar xzf alpine-minirootfs-*.tar.gz -C /mnt/rootfs

# 配置网络和登录（同上）
...

umount /mnt/rootfs
```

## 关键配置说明

### /sbin/init（智能检测启动脚本）

为了让 Firecracker VM 正常启动并支持多种 init 系统，使用智能检测的 `/sbin/init`：

```bash
cat > /mnt/rootfs/sbin/init << 'EOF'
#!/bin/sh
export PATH=/sbin:/bin:/usr/sbin:/usr/bin

# 挂载虚拟文件系统
mount -t proc proc /proc 2>/dev/null
mount -t sysfs sysfs /sys 2>/dev/null
[ ! -e /dev/null ] && mount -t devtmpfs devtmpfs /dev 2>/dev/null
mkdir -p /dev/pts /dev/shm /run/lock
mount -t tmpfs tmpfs /tmp 2>/dev/null
mount -t tmpfs tmpfs /run 2>/dev/null
mount -t devpts devpts /dev/pts -o gid=5,mode=620 2>/dev/null
chmod 1777 /tmp /run /dev/shm 2>/dev/null

# 设置主机名
hostname localhost

# 配置 loopback
ip link set lo up 2>/dev/null
ip addr show lo | grep -q "127.0.0.1" || ip addr add 127.0.0.1/8 dev lo 2>/dev/null

# 配置网络（如果有 eth0）
if [ -d /sys/class/net/eth0 ]; then
    ip link set eth0 up 2>/dev/null
    if ! ip addr show eth0 | grep -q "inet "; then
        command -v udhcpc >/dev/null 2>&1 && udhcpc -i eth0 -n -q -t 5 2>/dev/null
    fi
fi

# 自动检测 init 系统
if [ -x /sbin/openrc-init ]; then
    exec /sbin/openrc-init
elif [ -d /run/systemd/system ] || [ -x /lib/systemd/systemd ]; then
    exec /lib/systemd/systemd
elif [ -x /sbin/runit-init ]; then
    exec /sbin/runit-init
elif [ -x /sbin/runit ]; then
    exec /sbin/runit
else
    # 回退到 openrc 或 getty
    if [ -x /sbin/openrc ]; then
        /sbin/openrc -o sysinit -o default 2>/dev/null
    fi
    exec /sbin/getty -L 115200 ttyS0 vt100
fi
EOF

chmod +x /mnt/rootfs/sbin/init
```

此脚本会自动检测并执行可用的 init 系统：
- **openrc-init**：Alpine 新版独立 init
- **systemd**：需要 glibc 基础系统（Debian/Ubuntu）
- **runit**：轻量级 init（Alpine/Void Linux）
- **openrc**：Alpine 默认，通过 busybox init 调用
- **getty**：回退到最小 shell

### /etc/inittab（避免 openrc 错误）

如果使用自定义 `/sbin/init`，可以精简 `/etc/inittab`：

```
::respawn:/sbin/getty -L 115200 ttyS0 vt100
::ctrlaltdel:/sbin/reboot
```

## 验证 rootfs

```bash
# 检查文件系统
debugfs -R 'ls /sbin/init' rootfs/rootfs.ext4

# 挂载验证
mkdir -p /mnt/rootfs
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs
ls -la /mnt/rootfs/sbin/init
cat /mnt/rootfs/sbin/init
umount /mnt/rootfs
```

## 常见问题

### 1. "can't run '/sbin/openrc': No such file or directory"

说明 `/sbin/init` 仍然指向 busybox 的 init，它会读取 `/etc/inittab` 并尝试运行 openrc。

**解决方法**：删除 `/sbin/init` 符号链接，替换为自定义脚本：

```bash
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs
rm -f /mnt/rootfs/sbin/init
# 写入上面的自定义 /sbin/init 脚本
chmod +x /mnt/rootfs/sbin/init
umount /mnt/rootfs
```

### 2. VM 启动后没有网络

确保：
- 宿主机启用 IP 转发：`sysctl -w net.ipv4.ip_forward=1`
- 配置 NAT：`iptables -t nat -A POSTROUTING -o <外网接口> -j MASQUERADE`
- VM 内核参数包含 `ip=dhcp`
**有可能是没有使用sudo+ `--net`参数**
### 3. VM 无法启动

检查：
- 内核路径是否正确
- rootfs.ext4 是否为有效的 ext4 文件系统
- Firecracker 二进制是否可执行

```bash
file rootfs/rootfs.ext4    # 应显示 "Linux rev 1.0 ext4 filesystem"
file kernel/amd64/vmlinux.bin  # 应显示 ELF 64-bit LSB executable
```
### 启动命令
```
sudo ./groot vmm --kernel [内核] --rootfs [ext4的rootfs文件系统磁盘] --mem [内存] --cpus [cpu核心数] --net
```

## 完整的自动化脚本

```bash
#!/bin/bash
set -e

ROOTFS_IMG="rootfs/rootfs.ext4"
ROOTFS_SIZE_MB=1024
MOUNT_DIR="/mnt/rootfs"

echo "=== 创建 rootfs.ext4 虚拟磁盘 ==="

# 创建目录
mkdir -p "$(dirname $ROOTFS_IMG)"
mkdir -p "$MOUNT_DIR"

# 创建空白镜像
echo "创建 ${ROOTFS_SIZE_MB}MB 镜像..."
dd if=/dev/zero of="$ROOTFS_IMG" bs=1M count=$ROOTFS_SIZE_MB

# 格式化
echo "格式化 ext4..."
mkfs.ext4 -F "$ROOTFS_IMG"

# 挂载
echo "挂载镜像..."
mount -o loop "$ROOTFS_IMG" "$MOUNT_DIR"

# 安装 Alpine 最小系统
echo "安装 Alpine Linux..."
apk --initdb --allow-untrusted --arch x86_64 --root "$MOUNT_DIR" \
  --repository https://dl-cdn.alpinelinux.org/alpine/v3.21/main \
  add alpine-base busybox musl openrc

# 配置 /sbin/init
echo "配置启动脚本..."
cat > "$MOUNT_DIR/sbin/init" << 'INITEOF'
#!/bin/sh
export PATH=/sbin:/bin:/usr/sbin:/usr/bin
mount -t proc proc /proc 2>/dev/null
mount -t sysfs sysfs /sys 2>/dev/null
[ ! -e /dev/null ] && mount -t devtmpfs devtmpfs /dev 2>/dev/null
mkdir -p /dev/pts /dev/shm /run/lock
mount -t tmpfs tmpfs /tmp 2>/dev/null
mount -t tmpfs tmpfs /run 2>/dev/null
mount -t devpts devpts /dev/pts -o gid=5,mode=620 2>/dev/null
chmod 1777 /tmp /run /dev/shm 2>/dev/null
hostname localhost
ip link set lo up 2>/dev/null
ip addr show lo | grep -q "127.0.0.1" || ip addr add 127.0.0.1/8 dev lo 2>/dev/null
ip link set eth0 up 2>/dev/null
udhcpc -i eth0 -n -q -t 5 2>/dev/null
if [ -x /sbin/openrc-init ]; then
    exec /sbin/openrc-init
elif [ -d /run/systemd/system ] || [ -x /lib/systemd/systemd ]; then
    exec /lib/systemd/systemd
elif [ -x /sbin/runit-init ]; then
    exec /sbin/runit-init
elif [ -x /sbin/runit ]; then
    exec /sbin/runit
else
    [ -x /sbin/openrc ] && /sbin/openrc -o sysinit -o default 2>/dev/null
    exec /sbin/getty -L 115200 ttyS0 vt100
fi
INITEOF
chmod +x "$MOUNT_DIR/sbin/init"

# 配置网络
cat > "$MOUNT_DIR/etc/network/interfaces" << 'NETEOF'
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
NETEOF

# 设置 root 密码
chroot "$MOUNT_DIR" /bin/sh -c 'echo "root:root" | chpasswd'

# 卸载
echo "清理..."
umount "$MOUNT_DIR"

echo "=== 完成 ==="
echo "镜像文件: $ROOTFS_IMG"
echo "大小: $(du -h $ROOTFS_IMG | cut -f1)"
```

将上述脚本保存为 `scripts/make-rootfs.sh`，然后运行：

```bash
sudo bash scripts/make-rootfs.sh
```
