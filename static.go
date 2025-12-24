package main

import (
	"embed"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

// StaticFileServer serves static files from the embedded filesystem
func StaticFileServer() http.Handler {
	// 直接使用整个静态文件系统，不进行子目录截取
	// 这样当请求 /css/bootstrap.min.css 时，会查找 static/css/bootstrap.min.css
	return http.FileServer(http.FS(staticFiles))
}