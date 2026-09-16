# rootfs.ext4 制作速查（纯实操版，无理论）

> 理论请读同目录 `create-rootfs.md`。这里只有**能直接复制粘贴的命令**。
> 约定：`<ROOTFS>` = 你的 rootfs 目录；命令默认以 root 执行（先 `sudo -i`）。

---

## 0. 一次性准备宿主工具（选你宿主对应的）

```bash
# Debian / Ubuntu
apt-get update && apt-get install -y e2fsprogs debootstrap rsync xz-utils wget

# Arch
pacman -S --noconfirm e2fsprogs debootstrap arch-install-scripts rsync xz wget

# Fedora / Rocky
dnf install -y e2fsprogs debootstrap rsync xz wget
```

---

## 1. 得到一个 rootfs 目录（三选一）

### 方式 A：官方 tarball（最快）

```bash
# Alpine（x86_64, v3.21）
mkdir -p /opt/mkrootfs/alpine
wget -O /tmp/a.tar.gz https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.3-x86_64.tar.gz
tar xzf /tmp/a.tar.gz -C /opt/mkrootfs/alpine

# Ubuntu base（24.04 amd64）
mkdir -p /opt/mkrootfs/ubuntu
wget -O /tmp/u.tar.gz https://cdimage.ubuntu.com/ubuntu-base/releases/24.04/release/ubuntu-base-24.04-base-amd64.tar.gz
tar xzf /tmp/u.tar.gz -C /opt/mkrootfs/ubuntu

# Void Linux（x86_64 glibc）
mkdir -p /opt/mkrootfs/void
wget -O /tmp/v.tar.xz https://mirrors.tuna.tsinghua.edu.cn/voidlinux/live/current/void-x86_64-ROOTFS-20250202.tar.xz
tar xJf /tmp/v.tar.xz -C /opt/mkrootfs/void

# Arch bootstrap（注意解压后会多一层 root.x86_64/）
mkdir -p /opt/mkrootfs/arch
wget -O /tmp/arch.tar.gz https://geo.mirror.pkgbuild.com/iso/latest/archlinux-bootstrap-x86_64.tar.gz
tar xzf /tmp/arch.tar.gz -C /opt/mkrootfs/arch
mv /opt/mkrootfs/arch/root.x86_64 /tmp/_a && rmdir /opt/mkrootfs/arch && mv /tmp/_a /opt/mkrootfs/arch
```

### 方式 B：bootstrap 生成（最小最干净）

```bash
# Debian 12 (bookworm) x86_64
debootstrap --arch=amd64 --variant=minbase bookworm /opt/mkrootfs/debian http://deb.debian.org/debian

# Ubuntu 24.04 (noble)
debootstrap --arch=amd64 noble /opt/mkrootfs/ubuntu http://archive.ubuntu.com/ubuntu

# Arch Linux
pacstrap -c /opt/mkrootfs/arch base

# Fedora 40
dnf -y --releasever=40 --installroot=/opt/mkrootfs/fedora \
  --disablerepo='*' --enablerepo=fedora \
  install systemd passwd dnf vim-minimal openssh-server iproute

# Rocky Linux 9
dnf -y --installroot=/opt/mkrootfs/rocky --releasever=9 --setopt=install_weak_deps=False \
  install rocky-release systemd passwd dnf vim-minimal openssh-server iproute
```

### 方式 C：litevm 自带命令

```bash
./litevm pm download                          # 交互下载官方 tarball
sudo ./litevm pm make --distro debian --version 12 --arch amd64 --type standard
# 产物目录：--dest 参数指定（默认 ./rootfs/）
```

---

## 2. 配置（设密码 + 放 init/getty）

把 `<ROOTFS>` 换成你的目录：

```bash
ROOTFS=/opt/mkrootfs/alpine
```

### 2.1 进 chroot 前的挂载（包管理器/设密码需要）

```bash
mount --bind /dev $ROOTFS/dev
mount --bind /dev/pts $ROOTFS/dev/pts
mount -t proc proc $ROOTFS/proc
mount -t sysfs sysfs $ROOTFS/sys
mount --bind /run $ROOTFS/run
```

### 2.2 root 密码

```bash
chroot $ROOTFS /bin/sh -c 'echo root:root | chpasswd'
```

### 2.3 通用 init 脚本（任何发行版都行，开机直接进 root shell）

```bash
cat > $ROOTFS/sbin/init <<'EOF'
#!/bin/sh
export PATH=/sbin:/bin:/usr/sbin:/usr/bin
mount -t proc proc /proc 2>/dev/null
mount -t sysfs sysfs /sys 2>/dev/null
[ -e /dev/null ] || mount -t devtmpfs devtmpfs /dev 2>/dev/null
mkdir -p /dev/pts /dev/shm /run/lock
mount -t devpts devpts /dev/pts -o gid=5,mode=620 2>/dev/null
mount -t tmpfs tmpfs /tmp 2>/dev/null
mount -t tmpfs tmpfs /run 2>/dev/null
chmod 1777 /tmp /dev/shm 2>/dev/null
hostname localhost
ip link set lo up 2>/dev/null
[ -d /sys/class/net/eth0 ] && ip link set eth0 up 2>/dev/null
command -v udhcpc >/dev/null 2>&1 && udhcpc -i eth0 -n -q -t 5 2>/dev/null
exec /bin/sh
EOF
chmod +x $ROOTFS/sbin/init
```

### 2.4 要用原生 init 就别写 2.3，换成下面之一

busybox init（Alpine，不依赖 openrc）：
```bash
cat > $ROOTFS/etc/inittab <<'EOF'
::sysinit:/bin/mount -t proc proc /proc
::sysinit:/bin/mount -t sysfs sysfs /sys
::sysinit:/bin/mount -t devtmpfs devtmpfs /dev
::respawn:/sbin/getty -L 115200 ttyS0 vt100
::ctrlaltdel:/sbin/reboot
EOF
```

systemd（Debian/Ubuntu/Arch/Fedora，开机自动登录 ttyS0）：
```bash
mkdir -p $ROOTFS/etc/systemd/system/serial-getty@ttyS0.service.d/
cat > $ROOTFS/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf <<'EOF'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --keep-baud 115200,57600,38400,9600 ttyS0 vt220
EOF
```

runit（Void）：
```bash
# 把 tty1 改成 ttyS0（下面路径按实际版本调整）
grep -rl 'agetty' $ROOTFS/etc/runit/ | xargs sed -i 's/agetty .*/agetty -8 -L 115200 ttyS0/' 2>/dev/null || true
```

### 2.5 卸载挂载、清理

```bash
rm -f $ROOTFS/usr/bin/qemu-*-static
printf 'nameserver 172.16.0.1\n' > $ROOTFS/etc/resolv.conf
umount $ROOTFS/run 2>/dev/null
umount $ROOTFS/sys 2>/dev/null
umount $ROOTFS/proc 2>/dev/null
umount $ROOTFS/dev/pts 2>/dev/null
umount $ROOTFS/dev 2>/dev/null
```

---

## 3. 打成 rootfs.ext4（3 行）

```bash
mkdir -p rootfs
truncate -s 2G rootfs/rootfs.ext4                     # 2G 起步，嫌大用 512M
mkfs.ext4 -F -q -d $ROOTFS rootfs/rootfs.ext4          # -d = 免挂载直装；e2fsprogs≥1.43
e2fsck -f rootfs/rootfs.ext4                           # 校验，输出 clean 即合格
```

没有 `-d` 的旧工具就用挂载法：

```bash
mkdir -p /mnt/rootfs
mount -o loop rootfs/rootfs.ext4 /mnt/rootfs
cp -a $ROOTFS/. /mnt/rootfs/
sync && umount /mnt/rootfs
e2fsck -f rootfs/rootfs.ext4
```

---

## 4. 启动

```bash
# 一次性：下载内核、配网络
./litevm vmm download-kernel
sudo ./litevm vmm setup-network

# 启动（内核路径换成实际下载的文件名）
./litevm vmm run --kernel kernel/x86_64/vmlinux-6.18.44 \
  --rootfs rootfs/rootfs.ext4 --net

# 进去后验证
#   ip addr show eth0        # 有 172.16.0.x 即网络通
#   poweroff                 # 关机（litevm 会自动退出）
```

---

## 5. 报错只看这 4 条

| 现象 | 直接改法 |
|------|---------|
| `VFS: Unable to mount root fs` | `mkfs.ext4 -F rootfs/rootfs.ext4` 重新格式化（先备份） |
| `can't run '/sbin/init'` | 检查镜像里有没有 `/sbin/init`：`debugfs -R 'ls /sbin/init' rootfs/rootfs.ext4` |
| 没登录提示 | getty 必须盯 ttyS0，检查 2.3/2.4 是否写对 |
| 没网络 | 启动加上 `--net`（root 运行或先 `sudo ./litevm vmm setup-network`） |

```bash
# 万能调试：跳过 init 直进 shell 看系统
./litevm vmm run --kernel <内核> --rootfs rootfs/rootfs.ext4 \
  --kernel-args "console=ttyS0,115200n8 reboot=k panic=1 nomodule init=/bin/sh"
```