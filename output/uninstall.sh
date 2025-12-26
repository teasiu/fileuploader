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
