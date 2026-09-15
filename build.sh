#!/bin/bash

set -e

# ============================================
#  LiteVM 编译与打包脚本
#  支持 tar.xz 打包
# ============================================

PROJECT_NAME="litevm"
OUTPUT_DIR="build"
VERSION="1.1"
PACKAGE_LICENSE="MIT"
PACKAGE_URL="https://gyscan.space"
PACKAGE_DESCRIPTION="Go 版双模式隔离工具（chroot/proot）- 轻量级 Linux 容器环境"
PACKAGE_MAINTAINER="弈秋忘忧白帽 <https://gyscan.space>"

# 定义目标架构
declare -A TARGETS
TARGETS=(
  ["amd64"]="linux/amd64"
  ["x86"]="linux/386"
  ["armv7"]="linux/arm"
  ["armv8"]="linux/arm64"
)

# Termux 检测
IS_TERMUX=0
if [ -n "$TERMUX_VERSION" ] || [ -n "$PREFIX" ]; then
  IS_TERMUX=1
fi

# 打印标题
print_header() {
  local os_name=$(uname -s)
  local arch_name=$(uname -m)
  local go_version=$(go version 2>/dev/null | awk '{print $3}' || echo "unknown")

  echo "========================================"
  echo "  $PROJECT_NAME 编译与打包脚本"
  echo "========================================"
  echo "  项目:        $PROJECT_NAME"
  echo "  版本:        $VERSION"
  echo "  Go 版本:     $go_version"
  echo "  构建环境:    $os_name/$arch_name"
  echo "  输出目录:    $OUTPUT_DIR"
  if [ "$IS_TERMUX" -eq 1 ]; then
    echo "  模式:        Termux 兼容模式"
  fi
  echo "========================================"
  echo ""
}

# 创建输出目录
create_output_dir() {
  mkdir -p "$OUTPUT_DIR"
  echo "✓ 输出目录准备完毕"
}

# 检查 Go 环境
check_go_env() {
  if ! command -v go &>/dev/null; then
    echo "✗ 错误: 未找到 Go 编译器"
    exit 1
  fi
  echo "✓ Go 环境检查通过"
}

# 检查 xz 是否可用
check_xz() {
  if ! command -v xz &>/dev/null; then
    echo "✗ 错误: 未找到 xz，请安装 xz-utils"
    exit 1
  fi
  echo "✓ xz 环境检查通过"
}

# ============================
#  二进制编译
# ============================

build_binary() {
  local arch=$1
  local target=$2
  local GOOS GOARCH BINARY_NAME

  IFS="/" read -r GOOS GOARCH <<<"$target"

  BINARY_NAME="${PROJECT_NAME}_${GOOS}_${GOARCH}"
  case "$GOARCH" in
  "arm")
    BINARY_NAME="${PROJECT_NAME}_${GOOS}_armv7"
    export GOARM=7
    ;;
  "arm64") BINARY_NAME="${PROJECT_NAME}_${GOOS}_armv8" ;;
  esac

  echo "→ 编译: $arch ($GOOS/$GOARCH)"

  local start_time=$(date +%s)
  export GOOS GOARCH

  if go build -ldflags "-s -w" -o "${OUTPUT_DIR}/${BINARY_NAME}" ./cmd/litevm 2>&1; then
    local end_time=$(date +%s)
    local file_size=$(du -h "${OUTPUT_DIR}/${BINARY_NAME}" | cut -f1)
    echo "  ✓ 编译成功 ($((end_time - start_time))s, ${file_size})"
    return 0
  else
    echo "  ✗ 编译失败: $arch"
    return 1
  fi
}

build_all_binaries() {
  local success=0 fail=0 current=0 total=${#TARGETS[@]}
  echo ""
  echo "--- 编译所有架构 ---"
  for arch in "${!TARGETS[@]}"; do
    current=$((current + 1))
    echo "[${current}/${total}]"
    if build_binary "$arch" "${TARGETS[$arch]}"; then
      success=$((success + 1))
    else
      fail=$((fail + 1))
    fi
  done
  echo ""
  echo "结果: 成功 $success / 失败 $fail / 总计 $total"
  [ $fail -eq 0 ] || return 1
  return 0
}

# ============================
#  打包函数
# ============================

# 生成 install 脚本
generate_install_script() {
  local arch=$1

  if [ "$IS_TERMUX" -eq 1 ]; then
    local install_path="/data/data/com.termux/files/usr/bin"
  else
    local install_path="/usr/local/bin"
  fi

  cat <<INSTALL
#!/bin/sh
# litevm ${VERSION} 安装脚本
# 需要 root/sudo 权限安装到系统目录

if [ "\$(id -u)" -ne 0 ] && [ -z "\$TERMUX_VERSION" ]; then
    echo "请使用 sudo 运行此安装脚本:"
    echo "  sudo sh install.sh"
    exit 1
fi

install_path="${install_path}"

echo "正在安装 litevm ${VERSION} 到 \${install_path}/ ..."
cp litevm "\${install_path}/litevm"
chmod 755 "\${install_path}/litevm"

# 创建卸载脚本
cat > "\${install_path}/litevm-uninstall" << 'UNINSTALL'
#!/bin/sh
if [ "\$(id -u)" -ne 0 ] && [ -z "\$TERMUX_VERSION" ]; then
    echo "请使用 sudo 运行卸载:"
    echo "  sudo litevm-uninstall"
    exit 1
fi
echo "正在卸载 litevm..."
rm -f /usr/local/bin/litevm
rm -f /data/data/com.termux/files/usr/bin/litevm
rm -f /usr/local/bin/litevm-uninstall
rm -f /data/data/com.termux/files/usr/bin/litevm-uninstall
echo "litevm 已卸载"
UNINSTALL
chmod 755 "\${install_path}/litevm-uninstall"

echo "安装完成！"
echo "  运行: litevm -h"
echo "  卸载: sudo litevm-uninstall"
INSTALL
}

# tar.xz 打包
package_tarxz() {
  local arch=$1 binary_name=$2
  echo "→ 打包 tar.xz (${arch})..."

  local tar_arch
  local out_name
  case "$arch" in
  amd64) tar_arch=linux_amd64 ;;
  x86) tar_arch=linux_i386 ;;
  armv7) tar_arch=linux_armv7 ;;
  armv8) tar_arch=linux_arm64 ;;
  *) tar_arch=$arch ;;
  esac

  out_name="${PROJECT_NAME}-${VERSION}-${tar_arch}.tar.xz"
  if [ "$IS_TERMUX" -eq 1 ]; then
    out_name="${PROJECT_NAME}-${VERSION}-termux_${tar_arch}.tar.xz"
  fi

  local pkg_dir="${OUTPUT_DIR}/tarxz/${out_name%.tar.xz}"
  mkdir -p "${pkg_dir}"

  # 复制二进制
  cp "${OUTPUT_DIR}/${binary_name}" "${pkg_dir}/litevm"
  chmod 755 "${pkg_dir}/litevm"

  # 生成 install 脚本
  generate_install_script "$arch" >"${pkg_dir}/install.sh"
  chmod 755 "${pkg_dir}/install.sh"

  # 打包为 tar.xz
  local abs_output_dir
  abs_output_dir="$(cd "${OUTPUT_DIR}" && pwd)"
  pushd "${pkg_dir}" >/dev/null
  if tar -cJf "${abs_output_dir}/${out_name}" litevm install.sh 2>/dev/null; then
    popd >/dev/null
    rm -rf "${OUTPUT_DIR}/tarxz"
    echo "  ✓ ${out_name}"
    return 0
  else
    popd >/dev/null
    rm -rf "${OUTPUT_DIR}/tarxz"
    echo "  ⚠ tar.xz 打包失败"
    return 1
  fi
}

# ============================
#  编译+打包
# ============================

build_and_package() {
  local arch=$1 target=$2
  local binary_name

  IFS="/" read -r GOOS GOARCH <<<"$target"
  binary_name="${PROJECT_NAME}_${GOOS}_${GOARCH}"
  case "$GOARCH" in
  "arm") binary_name="${PROJECT_NAME}_${GOOS}_armv7" ;;
  "arm64") binary_name="${PROJECT_NAME}_${GOOS}_armv8" ;;
  esac

  # 编译
  if ! build_binary "$arch" "$target"; then
    return 1
  fi

  echo ""

  # 打包为 tar.xz
  local tarxz_ok=0
  if package_tarxz "$arch" "$binary_name"; then
    tarxz_ok=1

    # armv7/armv8 额外生成 Termux 专用包
    if [ "$arch" = "armv7" ] || [ "$arch" = "armv8" ]; then
      local saved_is_termux=$IS_TERMUX
      IS_TERMUX=1
      set +e
      package_tarxz "$arch" "$binary_name" && tarxz_ok=1
      set -e
      IS_TERMUX=$saved_is_termux
    fi
  fi

  # 有包后删除二进制
  if [ "$tarxz_ok" -eq 1 ]; then
    rm -f "${OUTPUT_DIR}/${binary_name}"
  else
    echo "  ℹ 保留二进制文件: ${binary_name}"
  fi
}

# 编译所有并打包
build_all_and_package() {
  local success=0 fail=0 current=0 total=${#TARGETS[@]}

  if [ "$IS_TERMUX" -eq 1 ]; then
    # Termux 环境只编译本机架构
    local native_arch
    case "$(uname -m)" in
    armv7l) native_arch=armv7 ;;
    aarch64 | arm64 | armv8l) native_arch=armv8 ;;
    x86_64 | amd64) native_arch=amd64 ;;
    i*86) native_arch=x86 ;;
    *) native_arch=unknown ;;
    esac
    echo ""
    echo "--- Termux 模式：仅编译 $native_arch ---"
    if [ "$native_arch" != "unknown" ] && [ -n "${TARGETS[$native_arch]}" ]; then
      build_and_package "$native_arch" "${TARGETS[$native_arch]}" && success=1 || fail=1
    else
      echo "✗ 无法识别 Termux 架构"
      fail=1
    fi
  else
    echo ""
    echo "--- 编译并打包所有架构 ---"
    for arch in "${!TARGETS[@]}"; do
      current=$((current + 1))
      echo "[${current}/${total}] $arch"
      if build_and_package "$arch" "${TARGETS[$arch]}"; then
        success=$((success + 1))
      else
        fail=$((fail + 1))
      fi
      echo ""
    done
  fi

  echo "结果: 成功 $success / 失败 $fail / 总计 $total"
  [ $fail -eq 0 ] || return 1
  return 0
}

# 仅打包已有二进制
package_existing() {
  local arch=$1
  local target="${TARGETS[$arch]}"
  local GOOS GOARCH

  if [ -z "$target" ]; then
    echo "✗ 未知架构: $arch"
    return 1
  fi

  IFS="/" read -r GOOS GOARCH <<<"$target"

  local binary_name
  binary_name="${PROJECT_NAME}_${GOOS}_${GOARCH}"
  case "$GOARCH" in
  "arm") binary_name="${PROJECT_NAME}_${GOOS}_armv7" ;;
  "arm64") binary_name="${PROJECT_NAME}_${GOOS}_armv8" ;;
  esac

  if [ ! -f "${OUTPUT_DIR}/${binary_name}" ]; then
    echo "✗ 未找到二进制: ${OUTPUT_DIR}/${binary_name}"
    echo "  请先编译: $0 build $arch"
    return 1
  fi

  package_tarxz "$arch" "$binary_name"

  # armv7/armv8 额外生成 Termux 专用包
  if [ "$arch" = "armv7" ] || [ "$arch" = "armv8" ]; then
    local saved_is_termux=$IS_TERMUX
    IS_TERMUX=1
    set +e
    package_tarxz "$arch" "$binary_name"
    set -e
    IS_TERMUX=$saved_is_termux
  fi
}

# 列出输出文件
list_output() {
  echo ""
  echo "--- 输出文件 ---"
  echo "----------------------------------------"
  if [ -d "$OUTPUT_DIR" ]; then
    ls -lh "$OUTPUT_DIR"/*.tar.xz 2>/dev/null | grep -v '^total' || echo "  (无 tar.xz 文件)"
  fi
  echo "----------------------------------------"
}

# 清理
clean() {
  echo "清理构建输出..."
  if [ -d "$OUTPUT_DIR" ]; then
    rm -rf "$OUTPUT_DIR"/*
    echo "✓ 清理完成"
  else
    echo "✓ 输出目录不存在，无需清理"
  fi
}

# 帮助
show_help() {
  cat <<EOF

用法: $0 <命令> [选项]

命令:
  all               编译所有架构并打包 (默认)
  build <arch>      仅编译指定架构并打包
  package <arch>    仅打包已有二进制为 tar.xz
  clean             清理构建文件
  help              显示帮助

架构:
  amd64   64位 x86 (PC/服务器)
  x86     32位 x86
  armv7   32位 ARM (树莓派2/3)
  armv8   64位 ARM (树莓派4/5/Termux)

Termux 支持:
  在 Termux 中运行本脚本会自动检测并进入 Termux 兼容模式:
    - 只编译本机架构 (armv8 或 armv7)
    - tar.xz 包中的 install 脚本安装到 \$PREFIX/bin
    - 附带 litevm-uninstall 卸载脚本

示例:
  $0                         # 编译全部并打包
  $0 build amd64             # 仅编译 amd64 并打包 tar.xz
  $0 package amd64           # 仅打包已有二进制为 tar.xz
  $0 clean                   # 清理

输出:
  .tar.xz 文件包含:
    litevm             二进制文件
    install.sh        安装脚本 (安装到 /usr/local/bin/ 或 Termux 的 \$PREFIX/bin/，自动生成卸载命令)
    armv7/armv8 额外生成 termux_ 前缀的 Termux 专用包

EOF
}

# ============================
#  主函数
# ============================

main() {
  local cmd=""

  # 简单解析参数
  while [ $# -gt 0 ]; do
    case "$1" in
    all | build | package | clean | help | --help | -h)
      cmd="$1"
      ;;
    amd64 | x86 | armv7 | armv8)
      ARCH="$1"
      ;;
    *)
      echo "✗ 未知选项: $1"
      show_help
      exit 1
      ;;
    esac
    shift
  done

  print_header
  check_go_env
  check_xz
  create_output_dir

  case "${cmd:-all}" in
  help | --help | -h)
    show_help
    ;;
  clean)
    clean
    ;;
  build)
    if [ -z "$ARCH" ]; then
      echo "✗ 请指定架构"
      show_help
      exit 1
    fi
    if [ -z "${TARGETS[$ARCH]}" ]; then
      echo "✗ 未知架构: $ARCH"
      exit 1
    fi
    build_and_package "$ARCH" "${TARGETS[$ARCH]}"
    list_output
    ;;
  package)
    if [ -z "$ARCH" ]; then
      # 打包所有已有二进制
      echo "打包所有已有二进制..."
      for arch in "${!TARGETS[@]}"; do
        package_existing "$arch"
      done
    else
      package_existing "$ARCH"
    fi
    list_output
    ;;
  all | *)
    build_all_and_package
    list_output
    ;;
  esac
}

main "$@"
