#!/bin/bash

set -e

# 项目配置
PROJECT_NAME="groot"
OUTPUT_DIR="build"
PARALLEL=0  # 是否并行编译，0=否，1=是

# 定义目标架构
declare -A TARGETS
TARGETS=(
    ["amd64"]="linux/amd64"
    ["x86"]="linux/386"
    ["armv7"]="linux/arm"
    ["armv8"]="linux/arm64"
)

# 打印标题
print_header() {
    local os_name=$(uname -s)
    local arch_name=$(uname -m)
    local go_version=$(go version 2>/dev/null | awk '{print $3}' || echo "unknown")
    
    echo "========================================"
    echo "  $PROJECT_NAME 编译脚本"
    echo "========================================"
    echo "  项目:        $PROJECT_NAME"
    echo "  Go 版本:     $go_version"
    echo "  构建环境:    $os_name/$arch_name"
    echo "  输出目录:    $OUTPUT_DIR"
    echo "========================================"
    echo ""
}

# 创建输出目录
create_output_dir() {
    mkdir -p "$OUTPUT_DIR"
    if [ $? -eq 0 ]; then
        echo "✓ 输出目录准备完毕"
    fi
}

# 检查 Go 环境
check_go_env() {
    if ! command -v go &> /dev/null; then
        echo "✗ 错误: 未找到 Go 编译器"
        exit 1
    fi
    echo "✓ Go 环境检查通过"
}

# 编译单个架构
build_single() {
    local arch=$1
    local target=$2
    local GOOS GOARCH ARCHIVE_NAME
    
    IFS="/" read -r GOOS GOARCH <<< "$target"
    
    case "$GOARCH" in
        "amd64")
            ARCHIVE_NAME="${PROJECT_NAME}_linux_amd64"
            ;;
        "386")
            ARCHIVE_NAME="${PROJECT_NAME}_linux_386"
            ;;
        "arm")
            ARCHIVE_NAME="${PROJECT_NAME}_linux_armv7"
            export GOARM=7
            ;;
        "arm64")
            ARCHIVE_NAME="${PROJECT_NAME}_linux_armv8"
            ;;
        *)
            echo "✗ 未知架构: $GOARCH"
            return 1
            ;;
    esac
    
    echo "→ 编译: $arch ($GOOS/$GOARCH)"
    
    # 设置环境变量
    export GOOS
    export GOARCH
    
    # 编译
    local start_time=$(date +%s)
    
    if go build -ldflags "-s -w" -o "${OUTPUT_DIR}/${ARCHIVE_NAME}" ./cmd/groot 2>&1; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        local file_size=$(du -h "${OUTPUT_DIR}/${ARCHIVE_NAME}" | cut -f1)
        
        echo "  ✓ 编译成功 (${duration}s, ${file_size})"
        
        return 0
    else
        echo "✗ 编译失败: $GOOS/$GOARCH"
        return 1
    fi
}

# 编译所有架构
build_all() {
    echo "开始编译所有架构..."
    echo ""
    
    local success_count=0
    local fail_count=0
    local total=${#TARGETS[@]}
    local current=0
    
    for arch in "${!TARGETS[@]}"; do
        current=$((current + 1))
        echo "[${current}/${total}] 处理 $arch"
        
        if build_single "$arch" "${TARGETS[$arch]}"; then
            success_count=$((success_count + 1))
        else
            fail_count=$((fail_count + 1))
        fi
        echo ""
    done
    
    # 显示统计
    echo "========================================"
    echo "构建统计:"
    echo "  成功: $success_count"
    echo "  失败: $fail_count"
    echo "  总计: $total"
    echo "========================================"
    
    if [ $fail_count -eq 0 ]; then
        echo ""
        echo "✓ 所有架构编译完成！"
        list_output
    else
        echo ""
        echo "✗ 部分构建失败，请检查错误"
        return 1
    fi
}

# 列出输出文件
list_output() {
    echo ""
    echo "输出文件列表:"
    echo "----------------------------------------"
    if [ -d "$OUTPUT_DIR" ]; then
        ls -lh "$OUTPUT_DIR"
    fi
    echo "----------------------------------------"
    echo ""
    echo "架构说明:"
    echo "  - amd64/x86: 适用于PC和服务器"
    echo "  - armv7: 适用于32位ARM设备（树莓派2/3等）"
    echo "  - armv8: 适用于64位ARM设备（树莓派4/5等）"
}

# 清理旧构建
clean() {
    echo "清理旧构建..."
    
    if [ -d "$OUTPUT_DIR" ]; then
        if rm -rf "$OUTPUT_DIR"/* 2>/dev/null; then
            echo "✓ 清理完成"
        else
            echo "⚠ 清理完成，但可能有未删除的文件"
        fi
    else
        echo "✓ 输出目录不存在，无需清理"
    fi
}

# 显示帮助
show_help() {
    cat << EOF

用法: $0 [选项]

选项:
    all         编译所有架构 (默认)
    amd64       仅编译 Linux amd64
    x86         仅编译 Linux 386
    armv7       仅编译 Linux armv7
    armv8       仅编译 Linux armv8
    clean       清理旧构建文件
    help        显示帮助信息

示例:
    $0              # 编译所有架构
    $0 amd64        # 仅编译 amd64
    $0 clean        # 清理
    $0 help         # 显示帮助

支持架构:
    - amd64  (64位 x86)
    - x86    (32位 x86)
    - armv7  (32位 ARM)
    - armv8  (64位 ARM)

适用设备:
    - PC/服务器: amd64 或 x86
    - 树莓派:
      - 树莓派 2/3: armv7 (32位)
      - 树莓派 4/5: armv8 (64位)
    - Termux/Android:
      - armv7: 适用于32位Android设备
      - armv8: 适用于64位Android设备 (推荐)

如何确定设备架构:
    在终端运行 'uname -m' 查看架构
    aarch64/arm64 → armv8
    armv7l → armv7
    x86_64 → amd64
    i686/i386 → x86

EOF
}

# 主函数
main() {
    # 初始化
    print_header
    check_go_env
    create_output_dir
    
    if [ $# -eq 0 ]; then
        # 默认编译所有
        echo ""
        build_all
    else
        case "$1" in
            all)
                echo ""
                build_all
                ;;
            amd64|x86|armv7|armv8)
                if [ -n "${TARGETS[$1]}" ]; then
                    echo ""
                    if build_single "$1" "${TARGETS[$1]}"; then
                        echo ""
                        list_output
                    fi
                else
                    echo "✗ 未知架构: $1"
                    show_help
                    exit 1
                fi
                ;;
            clean)
                echo ""
                clean
                ;;
            help|--help|-h)
                show_help
                ;;
            *)
                echo "✗ 未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    fi
}

# 运行
main "$@"
