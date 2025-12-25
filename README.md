# fileuploader

我要用go写一个应用，名称fileuploader，以下是设计要求：
适用于Ubuntu 20.04 armhf/arm64嵌入式主板的文件上传UI程序，使用Go语言开发；
- 限制操作在`/var/www/html/files/`目录下
- 列出当前目录内容（包括软链接），隐藏"_h5ai"开头的文件夹和文件
- 支持创建目录和软链接
- 支持多选文件上传，无文件类型和大小限制
- 提供直观的Web界面进行操作
- 支持systemd自动运行
-支持nginx反代，反代链接 /fileuploader/
-程序端口默认监听端口为6012
提供预编译的二进制文件，支持两种架构：
- `fileuploader-armhf` - 适用于32位ARM架构（armv7*）
- `fileuploader-arm64` - 适用于64位ARM架构（aarch64）
设备符合上述架构之一，可以直接使用预编译版本
程序静态文件static文件夹使用embed，打包到二进制里；
nginx的反代设置必须按照如下代码逻辑保持不变：
# 文件上传器反向代理配置
    location /filesuploader/ {
        # 代理到FileUploader应用（运行在6012端口）
        proxy_pass http://127.0.0.1:6012;
        
        # 必要的代理头
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        
        # 超时设置
        proxy_connect_timeout 600;
        proxy_send_timeout 600;
        proxy_read_timeout 600;
        send_timeout 600;
        
        # 缓冲区设置
        proxy_buffering off;
        proxy_buffer_size 16k;
        proxy_buffers 4 16k;
        proxy_busy_buffers_size 32k;
        
        # 大文件上传支持
        client_max_body_size 8G;
        
        # 确保正确处理静态文件
        # 保留/filesuploader前缀，因为Go应用已经支持双路径访问
    }
    
    # 静态文件请求代理到FileUploader应用
    location ~ ^/static/ {
        proxy_pass http://127.0.0.1:6012;
        
        # 必要的代理头
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时设置
        proxy_connect_timeout 600;
        proxy_send_timeout 600;
        proxy_read_timeout 600;
        send_timeout 600;
    }
请设计这个程序并生成全部代码
