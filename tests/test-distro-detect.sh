#!/bin/bash
# 发行版检测测试脚本
# 用法: bash tests/test-distro-detect.sh

echo "=== Groot 发行版检测测试 ==="
echo

# 测试目录
TEST_DIR="/tmp/groot-test-rootfs"
PASSED=0
FAILED=0

# 清理函数
cleanup() {
    rm -rf "$TEST_DIR"
}

# 创建模拟rootfs
create_rootfs() {
    cleanup
    mkdir -p "$TEST_DIR/etc"
    mkdir -p "$TEST_DIR/bin"
    mkdir -p "$TEST_DIR/dev"
    mkdir -p "$TEST_DIR/tmp"
}

# 测试函数
test_distro() {
    local name="$1"
    local expected="$2"
    local setup_cmd="$3"
    
    create_rootfs
    eval "$setup_cmd"
    
    result=$(detect_distro "$TEST_DIR")
    
    if [ "$result" = "$expected" ]; then
        echo "[✓] $name: $result"
        PASSED=$((PASSED + 1))
    else
        echo "[✗] $name: 期望=$expected, 实际=$result"
        FAILED=$((FAILED + 1))
    fi
}

# 模拟detect_distro函数
detect_distro() {
    local rootfsPath="$1"
    
    if [ -f "$rootfsPath/etc/os-release" ]; then
        local content=$(cat "$rootfsPath/etc/os-release" | tr '[:upper:]' '[:lower:]')
        local id=$(echo "$content" | grep '^id=' | head -1 | sed 's/^id=//' | tr -d '"')
        local idLike=$(echo "$content" | grep '^id_like=' | head -1 | sed 's/^id_like=//' | tr -d '"')
        
        case "$id" in
            alpine) echo "alpine"; return ;;
            arch|manjaro|endeavouros|garuda|arcolinux|artix) echo "arch"; return ;;
            debian|ubuntu|linuxmint|pop|elementary|zorin|kali|raspbian|mx|antix|devuan) echo "debian"; return ;;
            fedora|centos|rhel|rocky|almalinux|ol|amzn|scientific) echo "redhat"; return ;;
            void) echo "void"; return ;;
            freebsd|openbsd|netbsd|dragonflybsd|midnightbsd) echo "unix"; return ;;
        esac
        
        if echo "$idLike" | grep -q "alpine"; then echo "alpine"; return; fi
        if echo "$idLike" | grep -q "arch"; then echo "arch"; return; fi
        if echo "$idLike" | grep -q "debian\|ubuntu"; then echo "debian"; return; fi
        if echo "$idLike" | grep -q "fedora\|rhel\|centos"; then echo "redhat"; return; fi
        if echo "$idLike" | grep -q "void"; then echo "void"; return; fi
    fi
    
    [ -f "$rootfsPath/etc/alpine-release" ] && echo "alpine" && return
    [ -f "$rootfsPath/etc/void-release" ] && echo "void" && return
    [ -f "$rootfsPath/etc/debian_version" ] && echo "debian" && return
    [ -f "$rootfsPath/etc/redhat-release" ] && echo "redhat" && return
    [ -f "$rootfsPath/etc/fedora-release" ] && echo "redhat" && return
    [ -f "$rootfsPath/etc/centos-release" ] && echo "redhat" && return
    [ -f "$rootfsPath/etc/arch-release" ] && echo "arch" && return
    [ -f "$rootfsPath/etc/freebsd-update.conf" ] && echo "unix" && return
    
    echo "generic"
}

echo "--- Alpine 系列 ---"
test_distro "Alpine Linux" "alpine" 'echo "ID=alpine" > "$TEST_DIR/etc/os-release"'
test_distro "Alpine (传统)" "alpine" 'echo "3.18.0" > "$TEST_DIR/etc/alpine-release"'

echo
echo "--- Arch 系列 ---"
test_distro "Arch Linux" "arch" 'echo -e "ID=arch\nID_LIKE=arch" > "$TEST_DIR/etc/os-release"'
test_distro "Manjaro" "arch" 'echo -e "ID=manjaro\nID_LIKE=arch" > "$TEST_DIR/etc/os-release"'
test_distro "EndeavourOS" "arch" 'echo -e "ID=endeavouros\nID_LIKE=arch" > "$TEST_DIR/etc/os-release"'
test_distro "Garuda" "arch" 'echo -e "ID=garuda\nID_LIKE=arch" > "$TEST_DIR/etc/os-release"'
test_distro "Artix" "arch" 'echo -e "ID=artix\nID_LIKE=arch" > "$TEST_DIR/etc/os-release"'
test_distro "Arch (传统)" "arch" 'touch "$TEST_DIR/etc/arch-release"'

echo
echo "--- Debian 系列 ---"
test_distro "Debian" "debian" 'echo -e "ID=debian\nID_LIKE=debian" > "$TEST_DIR/etc/os-release"'
test_distro "Ubuntu" "debian" 'echo -e "ID=ubuntu\nID_LIKE=debian" > "$TEST_DIR/etc/os-release"'
test_distro "Linux Mint" "debian" 'echo -e "ID=linuxmint\nID_LIKE=ubuntu" > "$TEST_DIR/etc/os-release"'
test_distro "Pop!_OS" "debian" 'echo -e "ID=pop\nID_LIKE=ubuntu" > "$TEST_DIR/etc/os-release"'
test_distro "Elementary OS" "debian" 'echo -e "ID=elementary\nID_LIKE=ubuntu" > "$TEST_DIR/etc/os-release"'
test_distro "Kali" "debian" 'echo -e "ID=kali\nID_LIKE=debian" > "$TEST_DIR/etc/os-release"'
test_distro "Raspbian" "debian" 'echo -e "ID=raspbian\nID_LIKE=debian" > "$TEST_DIR/etc/os-release"'
test_distro "Devuan" "debian" 'echo -e "ID=devuan\nID_LIKE=debian" > "$TEST_DIR/etc/os-release"'
test_distro "Debian (传统)" "debian" 'echo "12.0" > "$TEST_DIR/etc/debian_version"'

echo
echo "--- RHEL/Fedora/CentOS 系列 ---"
test_distro "Fedora" "redhat" 'echo -e "ID=fedora\nID_LIKE=fedora" > "$TEST_DIR/etc/os-release"'
test_distro "CentOS" "redhat" 'echo -e "ID=centos\nID_LIKE=rhel fedora" > "$TEST_DIR/etc/os-release"'
test_distro "RHEL" "redhat" 'echo -e "ID=rhel\nID_LIKE=fedora" > "$TEST_DIR/etc/os-release"'
test_distro "Rocky Linux" "redhat" 'echo -e "ID=rocky\nID_LIKE=rhel centos fedora" > "$TEST_DIR/etc/os-release"'
test_distro "AlmaLinux" "redhat" 'echo -e "ID=almalinux\nID_LIKE=rhel centos fedora" > "$TEST_DIR/etc/os-release"'
test_distro "Oracle Linux" "redhat" 'echo -e "ID=ol\nID_LIKE=fedora" > "$TEST_DIR/etc/os-release"'
test_distro "Amazon Linux" "redhat" 'echo -e "ID=amzn\nID_LIKE=fedora" > "$TEST_DIR/etc/os-release"'
test_distro "CentOS (传统)" "redhat" 'echo "CentOS Linux 8" > "$TEST_DIR/etc/centos-release"'

echo
echo "--- Void Linux ---"
test_distro "Void Linux" "void" 'echo -e "ID=void\nID_LIKE=void" > "$TEST_DIR/etc/os-release"'
test_distro "Void (传统)" "void" 'echo "Void" > "$TEST_DIR/etc/void-release"'

echo
echo "--- Unix/BSD 系列 ---"
test_distro "FreeBSD" "unix" 'echo -e "ID=freebsd\nID_LIKE=freebsd" > "$TEST_DIR/etc/os-release"'
test_distro "OpenBSD" "unix" 'echo -e "ID=openbsd\nID_LIKE=openbsd" > "$TEST_DIR/etc/os-release"'
test_distro "NetBSD" "unix" 'echo -e "ID=netbsd\nID_LIKE=netbsd" > "$TEST_DIR/etc/os-release"'
test_distro "DragonFlyBSD" "unix" 'echo -e "ID=dragonflybsd\nID_LIKE=dragonflybsd" > "$TEST_DIR/etc/os-release"'
test_distro "MidnightBSD" "unix" 'echo -e "ID=midnightbsd\nID_LIKE=freebsd" > "$TEST_DIR/etc/os-release"'
test_distro "FreeBSD (传统)" "unix" 'touch "$TEST_DIR/etc/freebsd-update.conf"'

echo
echo "--- 其他 ---"
test_distro "未知发行版" "generic" 'echo -e "ID=unknown\nID_LIKE=linux" > "$TEST_DIR/etc/os-release"'

echo
echo "=== 测试结果 ==="
echo "通过: $PASSED"
echo "失败: $FAILED"

if [ $FAILED -eq 0 ]; then
    echo "所有测试通过!"
    exit 0
else
    echo "有测试失败!"
    exit 1
fi