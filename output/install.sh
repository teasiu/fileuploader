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
