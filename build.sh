#!/bin/bash

# 构建脚本 - 文件上传工具
# 支持 armhf 和 arm64 架构

set -e

# 应用名称
APP_NAME="fileuploader"

# 版本信息
VERSION="1.0.0"

# 输出目录
OUTPUT_DIR="output"

# 构建时间
BUILD_TIME=$(date +"%Y-%m-%d %H:%M:%S")

# 清理输出目录
echo "清理输出目录..."
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# 检查Go环境
echo "检查Go环境..."
if ! command -v go &> /dev/null; then
    echo "错误: 未找到Go编译器，请安装Go 1.23.3或更高版本"
    exit 1
fi

GO_VERSION=$(go version)
echo "Go版本: $GO_VERSION"

# 检查upx是否可用
UPX_AVAILABLE=false
if command -v upx &> /dev/null; then
    UPX_AVAILABLE=true
    echo "检测到UPX，将使用UPX进行压缩"
else
    echo "未检测到UPX，将跳过压缩步骤"
fi

# 构建函数
build_arch() {
    local arch=$1
    local output_name=$2
    local goos=$3
    local goarch=$4
    local goarm=$5
    
    echo ""
    echo "========================================="
    echo "构建 $arch 架构..."
    echo "========================================="
    
    # 设置环境变量
    export GOOS=$goos
    export GOARCH=$goarch
    if [ -n "$goarm" ]; then
        export GOARM=$goarm
    fi
    
    # 构建命令
    echo "开始构建 $output_name..."
    go build \
        -ldflags "-s -w" \
        -o "$OUTPUT_DIR/$output_name" \
        main.go
    
    # 检查构建结果
    if [ ! -f "$OUTPUT_DIR/$output_name" ]; then
        echo "错误: 构建 $output_name 失败"
        exit 1
    fi
    
    # 检查文件大小
    local file_size=$(du -h "$OUTPUT_DIR/$output_name" | cut -f1)
    echo "构建成功: $output_name ($file_size)"
    
    # 使用upx压缩
#    if $UPX_AVAILABLE; then
#        echo "使用UPX压缩 $output_name..."
#        upx --best --ultra-brute "$OUTPUT_DIR/$output_name" || echo "警告: UPX压缩失败，保留原始文件"
#        local compressed_size=$(du -h "$OUTPUT_DIR/$output_name" | cut -f1)
#        echo "压缩后大小: $compressed_size"
#    fi
    
    # 显示文件信息
    echo "文件信息:"
    file "$OUTPUT_DIR/$output_name"
}

# 构建armhf架构 (32位ARM)
build_arch "armhf" "$APP_NAME-armhf" "linux" "arm" "6"

# 构建arm64架构 (64位ARM)
build_arch "arm64" "$APP_NAME-arm64" "linux" "arm64" ""

# 创建安装脚本
echo ""
echo "创建安装脚本..."
cat > "$OUTPUT_DIR/install.sh" << 'EOF'
#!/bin/bash

# 安装脚本 - 文件上传工具

set -e

# 应用名称
APP_NAME="fileuploader"

# 应用目录
APP_DIR="/opt/fileuploader"

# 上传目录
UPLOAD_DIR="/var/www/html/files"

# 服务名称
SERVICE_NAME="fileuploader"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查是否以root用户运行
if [ "$(id -u)" != "0" ]; then
    echo -e "${RED}错误: 请以root用户运行此脚本${NC}"
    exit 1
fi

# 检测系统架构
echo "检测系统架构..."
ARCH=$(uname -m)

case "$ARCH" in
    arm*|armv7*)
        BINARY_NAME="${APP_NAME}-armhf"
        ;;
    aarch64|arm64)
        BINARY_NAME="${APP_NAME}-arm64"
        ;;
    *)
        echo -e "${RED}错误: 不支持的架构: $ARCH${NC}"
        echo -e "${YELLOW}支持的架构: armhf (32位ARM), arm64 (64位ARM)${NC}"
        exit 1
        ;;
esac

echo -e "${GREEN}检测到架构: $ARCH, 使用二进制文件: $BINARY_NAME${NC}"

# 检查二进制文件是否存在
if [ ! -f "$BINARY_NAME" ]; then
    echo -e "${RED}错误: 未找到二进制文件: $BINARY_NAME${NC}"
    echo -e "${YELLOW}请确保二进制文件在当前目录中${NC}"
    exit 1
fi

# 创建应用目录
echo "创建应用目录..."
mkdir -p "$APP_DIR"

# 创建上传目录
echo "创建上传目录..."
mkdir -p "$UPLOAD_DIR"
chmod 755 "$UPLOAD_DIR"

# 复制二进制文件
echo "安装二进制文件..."
cp "$BINARY_NAME" "$APP_DIR/fileuploader"
chmod +x "$APP_DIR/fileuploader"

# 创建systemd服务文件
echo "创建systemd服务..."
cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF2
[Unit]
Description=File Uploader Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/fileuploader
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF2

# 重新加载systemd
echo "重新加载systemd配置..."
systemctl daemon-reload

# 启动服务
echo "启动服务..."
systemctl start "$SERVICE_NAME"

# 启用服务自启动
echo "设置服务自启动..."
systemctl enable "$SERVICE_NAME"

# 检查服务状态
echo ""
echo "检查服务状态..."
systemctl status "$SERVICE_NAME" --no-pager

# 显示安装信息
echo ""
echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}安装完成!${NC}"
echo -e "${GREEN}=========================================${NC}"
echo ""
echo -e "${YELLOW}应用信息:${NC}"
echo -e "  应用目录: ${GREEN}$APP_DIR${NC}"
echo -e "  上传目录: ${GREEN}$UPLOAD_DIR${NC}"
echo -e "  服务名称: ${GREEN}$SERVICE_NAME${NC}"
echo -e "  访问地址: ${GREEN}http://服务器IP:6012${NC}"
echo ""
echo -e "${YELLOW}常用命令:${NC}"
echo -e "  查看服务状态: ${GREEN}systemctl status $SERVICE_NAME${NC}"
echo -e "  启动服务: ${GREEN}systemctl start $SERVICE_NAME${NC}"
echo -e "  停止服务: ${GREEN}systemctl stop $SERVICE_NAME${NC}"
echo -e "  重启服务: ${GREEN}systemctl restart $SERVICE_NAME${NC}"
echo -e "  查看日志: ${GREEN}journalctl -u $SERVICE_NAME -f${NC}"
echo ""
EOF

# 创建卸载脚本
echo "创建卸载脚本..."
cat > "$OUTPUT_DIR/uninstall.sh" << 'EOF'
#!/bin/bash

# 卸载脚本 - 文件上传工具

set -e

# 应用名称
APP_NAME="fileuploader"

# 应用目录
APP_DIR="/opt/fileuploader"

# 上传目录
UPLOAD_DIR="/var/www/html/files"

# 服务名称
SERVICE_NAME="fileuploader"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查是否以root用户运行
if [ "$(id -u)" != "0" ]; then
    echo -e "${RED}错误: 请以root用户运行此脚本${NC}"
    exit 1
fi

# 停止服务
echo "停止服务..."
systemctl stop "$SERVICE_NAME" || echo "警告: 服务可能未运行"

# 禁用服务自启动
echo "禁用服务自启动..."
systemctl disable "$SERVICE_NAME" || echo "警告: 服务可能未设置自启动"

# 删除systemd服务文件
echo "删除systemd服务文件..."
rm -f "/etc/systemd/system/${SERVICE_NAME}.service"

# 重新加载systemd
echo "重新加载systemd配置..."
systemctl daemon-reload

# 删除应用目录
echo "删除应用目录..."
rm -rf "$APP_DIR"

# 询问是否删除上传目录
echo ""
read -p "是否删除上传目录 $UPLOAD_DIR 及其所有文件？(y/N): " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "删除上传目录..."
    rm -rf "$UPLOAD_DIR"
    echo -e "${YELLOW}警告: 上传目录及其所有文件已被删除${NC}"
else
    echo -e "${GREEN}保留上传目录: $UPLOAD_DIR${NC}"
fi

echo ""
echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}卸载完成!${NC}"
echo -e "${GREEN}=========================================${NC}"
echo ""
echo -e "${YELLOW}已删除的内容:${NC}"
echo -e "  应用目录: ${RED}$APP_DIR${NC}"
echo -e "  服务配置: ${RED}/etc/systemd/system/${SERVICE_NAME}.service${NC}"
echo -e "  系统服务: ${RED}$SERVICE_NAME${NC}"
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${GREEN}保留的内容:${NC}"
    echo -e "  上传目录: ${GREEN}$UPLOAD_DIR${NC}"
fi
echo ""
EOF

# 创建README文件
echo "创建README文件..."
cat > "$OUTPUT_DIR/README.txt" << 'EOF'
文件上传工具
============

一个基于Go语言开发的轻量级文件上传Web应用，专为Ubuntu 20.04 armhf/arm64嵌入式主板设计。

功能特性
--------
- 文件上传：支持多文件上传，最大文件大小8G
- 目录管理：创建、浏览、删除目录
- 文件操作：重命名、删除文件
- 软链接支持：创建和管理软链接
- 拖拽上传：支持拖拽文件到页面上传
- 响应式设计：适配不同屏幕尺寸
- 自动隐藏：自动隐藏以_h5ai开头的文件和文件夹

系统要求
--------
- Ubuntu 20.04 LTS
- armhf (32位ARM) 或 arm64 (64位ARM) 架构
- 至少512MB内存
- 足够的磁盘空间用于存储上传的文件

安装说明
--------

1. 准备工作
   - 确保系统已安装必要的依赖
   - 确保6012端口未被占用

2. 安装步骤
   ```bash
   # 切换到输出目录
   cd output
   
   # 给安装脚本添加执行权限
   chmod +x install.sh
   
   # 以root用户运行安装脚本
   sudo ./install.sh
   ```

3. 验证安装
   - 访问 http://服务器IP:6012
   - 使用 systemctl status fileuploader 检查服务状态

4. 卸载
   ```bash
   # 切换到输出目录
   cd output
   
   # 给卸载脚本添加执行权限
   chmod +x uninstall.sh
   
   # 以root用户运行卸载脚本
   sudo ./uninstall.sh
   ```

文件说明
--------
- fileuploader-armhf: 适用于32位ARM架构的二进制文件
- fileuploader-arm64: 适用于64位ARM架构的二进制文件
- install.sh: 安装脚本
- uninstall.sh: 卸载脚本
- README.txt: 使用说明

配置信息
--------
- 监听端口：6012
- 上传目录：/var/www/html/files/
- 应用目录：/opt/fileuploader/
- 最大上传文件大小：8G
- 自动隐藏：以_h5ai开头的文件和文件夹

常见问题
--------


Q: 服务无法启动怎么办？
A: 使用 journalctl -u fileuploader -f 查看详细日志，检查端口是否被占用。

Q: 如何修改监听端口？
A: 修改源码中的listenAddr变量，重新编译部署。

Q: 如何修改上传目录？
A: 修改源码中的rootDir变量，重新编译部署。

Q: 如何修改最大上传文件大小？
A: 修改源码中的maxUploadSize变量，重新编译部署。

使用说明
--------

1. 文件上传
   - 点击"上传文件"按钮选择文件
   - 或直接拖拽文件到文件列表区域
   - 支持多文件同时上传

2. 目录管理
   - 点击"创建目录"按钮创建新目录
   - 双击目录卡片进入子目录
   - 右键点击目录可进行重命名或删除操作

3. 文件操作
   - 右键点击文件可进行重命名或删除操作
   - 支持按文件名搜索文件

4. 软链接管理
   - 点击"创建链接"按钮创建软链接
   - 输入链接名称和目标路径
   - 软链接以蓝色链接图标显示

注意事项
--------
- 请确保系统有足够的磁盘空间用于存储上传的文件
- 定期清理不需要的文件，避免磁盘空间不足
- 建议在生产环境中配置防火墙，限制访问IP
- 定期备份重要数据
- 软链接的目标路径必须是系统中存在的有效路径

更新日志
--------
v1.0.0 (2024-12-22)
- 初始版本发布
- 支持文件上传、下载、删除等基本功能
- 支持目录创建、浏览、删除等操作
- 支持软链接创建和管理
- 支持拖拽上传
- 响应式设计，适配不同屏幕尺寸
- 自动隐藏以_h5ai开头的文件和文件夹

许可证
-------
MIT License
EOF

# 创建systemd目录和服务文件
echo "创建systemd配置..."
mkdir -p "$OUTPUT_DIR/systemd"

cat > "$OUTPUT_DIR/systemd/fileuploader.service" << 'EOF'
[Unit]
Description=File Uploader Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/fileuploader
ExecStart=/opt/fileuploader/fileuploader
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# 设置脚本执行权限
chmod +x "$OUTPUT_DIR/install.sh"
chmod +x "$OUTPUT_DIR/uninstall.sh"

# 显示构建结果
echo ""
echo "========================================="
echo "构建完成!"
echo "========================================="
echo ""
echo "输出目录: $OUTPUT_DIR"
echo ""
echo "构建产物:"
ls -lh "$OUTPUT_DIR/"
echo ""
echo "安装说明:"
echo "1. 切换到输出目录: cd $OUTPUT_DIR"
echo "2. 给安装脚本添加执行权限: chmod +x install.sh"
echo "3. 以root用户运行安装脚本: sudo ./install.sh"
echo ""
echo "访问地址: http://服务器IP:6012"
echo ""
