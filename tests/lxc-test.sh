#!/bin/bash
# lxc 容器完整验证脚本（需要 root，使用 sudo 运行）
# 用法: bash tests/lxc-test.sh [容器名]
#
# 该脚本验证：
#   1. 创建容器（若不存在则创建）
#   2. 启动后 lxc-child 作为守护进程存活（checkRoot/守卫、fd 就绪握手、命名空间）
#   3. 容器内部的 /proc /sys /dev/pts /tmp /run 是否被正确挂载
#   4. 停止后宿主机无残留挂载

set -u
NAME="${1:-alpine}"
ROOTFS="${2:-/home/yiqiu/Rootfs/alpine-x86}"
SHELL="${3:-/bin/ash}"

cd "$(dirname "$0")/.."

echo "==== [1/6] 容器列表 ===="
sudo ./groot lxc ls

echo
echo "==== [2/6] 确保容器已创建 (rootfs=$ROOTFS, shell=$SHELL) ===="
sudo ./groot lxc "$ROOTFS" "$SHELL" -name "$NAME" 2>/dev/null || true

echo
echo "==== [3/6] 停止可能残留的容器 ===="
sudo ./groot lxc "$NAME" stop 2>/dev/null || true
sleep 1

echo
echo "==== [4/6] 启动容器：应看到“已启动 (PID: ...)” ===="
sudo ./groot lxc "$NAME" start
sleep 1

PID=$(sudo ./groot lxc ps "$NAME" 2>/dev/null | sed -n 's/.*PID: \([0-9]*\).*/\1/p' | head -1)
if [ -z "$PID" ]; then
    # 从 config.json 读取
    CFG="/data/data/com.termux/files/usr/share/groot/lxc/containers/$NAME/config.json"
    PID=$(sudo sed -n 's/.*"pid": *\([0-9]*\).*/\1/p' "$CFG" 2>/dev/null | head -1)
fi
echo "容器 PID: $PID"

if [ -n "$PID" ] && [ -d "/proc/$PID" ]; then
    echo "[OK] lxc-child 进程存活 (PID $PID)"
    sudo readlink "/proc/$PID/ns/mnt" | sed 's/^/[OK] mount ns: /'
    sudo readlink "/proc/$PID/ns/pid" | sed 's/^/[OK] pid ns:   /'
    sudo readlink "/proc/$PID/ns/net" | sed 's/^/[OK] net ns:   /'
    sudo cat "/proc/$PID/status" | grep -E 'State|PPid' | sed 's/^/[info] /'
else
    echo "[!!] 容器进程未存活！请检查 start 输出"
fi

echo
echo "==== [5/6] 容器内挂载验证（nsenter 查看） ===="
if [ -n "$PID" ]; then
    sudo nsenter -t "$PID" --mount --pid --root "$ROOTFS" --wd / -- /bin/sh -c \
        'echo "--- 容器内 /proc ---"; ls /proc/self >/dev/null 2>&1 && echo "[OK] /proc 可用" || echo "[!!] /proc 不可用"
         echo "--- 容器内 /dev ---"; ls /dev/null /dev/zero /dev/urandom >/dev/null 2>&1 && echo "[OK] /dev 节点可用" || echo "[!!] /dev 节点缺失"
         echo "--- 容器内 /dev/pts ---"; mountpoint -q /dev/pts 2>/dev/null && echo "[OK] /dev/pts 已挂载" || ls /dev/pts >/dev/null 2>&1 && echo "[OK] /dev/pts 存在"
         echo "--- 容器内 /tmp ---"; echo test > /tmp/.groot-test && echo "[OK] /tmp 可写" && rm -f /tmp/.groot-test
         echo "--- 容器内 shell ---"; command -v '"$SHELL"' && echo "[OK] shell 可用" || echo "[!!] shell 缺失"
         echo "--- 容器内 hostname ---"; hostname'
fi

echo
echo "==== [6/6] 停止容器并检查宿主机残留 ===="
sudo ./groot lxc "$NAME" stop
sleep 1
LEFTOVER=$(mount | grep -c "$ROOTFS")
echo "rootfs 残留挂载数: $LEFTOVER"
[ "$LEFTOVER" -eq 0 ] && echo "[OK] 宿主机挂载表干净，无残留" || echo "[!!] 有残留挂载，请手动 umount"

echo
echo "==== 完成 ===="
echo "请手动验证登录： sudo ./groot lxc $NAME"
echo "进入容器后应看到容器 shell 提示符（如 root@alpine:/#）"