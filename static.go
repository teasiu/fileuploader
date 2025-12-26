package main

import (
	"embed"
	"net/http"
	"mime"
	"path/filepath"
)

//go:embed static
var staticFiles embed.FS

// StaticFileServer serves static files from the embedded filesystem
func StaticFileServer() http.Handler {
	// 返回一个基于 embed.FS 的文件服务器。
	// 这里不做 StripPrefix，调用方可以根据需要 wrap。
	return http.FileServer(http.FS(staticFiles))
}

// 可选：辅助函数，根据文件扩展名设置 Content-Type（若需要单独使用）
func contentTypeByName(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}
	// fallback 在 main.go 中也实现了
	return "application/octet-stream"
}