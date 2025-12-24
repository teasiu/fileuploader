# Nginx 反向代理配置说明

## 配置文件信息

- **配置文件**: `fileuploader.conf`
- **监听端口**: 6011
- **访问路径**: `http://服务器IP:6011/filesuploader`
- **代理目标**: FileUploader 应用 (运行在 6012 端口)

## 安装步骤

### 1. 安装 Nginx

```bash
sudo apt update
sudo apt install nginx -y
```

### 2. 复制配置文件

```bash
# 复制配置文件到 Nginx 配置目录
sudo cp nginx/fileuploader.conf /etc/nginx/sites-available/

# 创建符号链接
sudo ln -sf /etc/nginx/sites-available/fileuploader.conf /etc/nginx/sites-enabled/
```

### 3. 检查配置文件语法

```bash
sudo nginx -t
```

### 4. 重启 Nginx 服务

```bash
sudo systemctl restart nginx
```

### 5. 验证服务状态

```bash
sudo systemctl status nginx
```

## 访问方式

配置完成后，可以通过以下方式访问 FileUploader:

- **直接访问**: `http://服务器IP:6012`
- **通过 Nginx 反向代理**: `http://服务器IP:6011/filesuploader`

## 配置说明

### 主要功能

1. **反向代理**: 将 6011 端口的请求代理到 6012 端口的 FileUploader 应用
2. **路径重写**: 自动处理 `/filesuploader/` 路径前缀
3. **大文件支持**: 支持最大 8GB 文件上传
4. **安全增强**: 添加了多种安全相关的 HTTP 头
5. **性能优化**: 配置了合适的缓冲区和超时设置

### 关键配置项

- `listen 6011`: 监听 6011 端口
- `location /filesuploader/`: 处理包含 `/filesuploader/` 前缀的请求
- `proxy_pass http://127.0.0.1:6012/`: 代理到本地 6012 端口
- `client_max_body_size 8G`: 允许最大 8GB 的文件上传

## 故障排查

### 常见问题

1. **无法访问**:
   - 检查 Nginx 服务是否运行
   - 检查 6011 端口是否被占用
   - 检查防火墙设置

2. **上传失败**:
   - 确认 FileUploader 服务正在运行
   - 检查 `client_max_body_size` 设置是否与 FileUploader 一致
   - 查看 Nginx 错误日志: `sudo tail -f /var/log/nginx/fileuploader_error.log`

3. **502 Bad Gateway**:
   - 确认 FileUploader 应用正在 6012 端口运行
   - 检查 `proxy_pass` 配置是否正确

### 查看日志

```bash
# Nginx 访问日志
sudo tail -f /var/log/nginx/fileuploader_access.log

# Nginx 错误日志
sudo tail -f /var/log/nginx/fileuploader_error.log
```

## 卸载

```bash
# 删除配置文件
sudo rm /etc/nginx/sites-enabled/fileuploader.conf
sudo rm /etc/nginx/sites-available/fileuploader.conf

# 重启 Nginx
sudo systemctl restart nginx
```

## 注意事项

1. 确保 FileUploader 服务在 Nginx 启动前已经运行
2. 如果修改了 FileUploader 的端口，需要同步更新 `proxy_pass` 配置
3. 生产环境中建议配置 HTTPS
4. 根据实际网络环境调整超时和缓冲区设置