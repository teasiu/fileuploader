package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 配置
var (
	rootDir          = "/var/www/html/files"
	listenAddr       = "0.0.0.0:6012"
	maxUploadSize    = int64(8 * 1024 * 1024 * 1024) // 8GB
	memoryBufferSize = int64(16 * 1024 * 1024)       // 16MB内存缓冲区（用于 ParseMultipartForm）
	// 程序根目录
	appRootDir = "/opt/fileuploader"
)

// FileInfo 文件信息结构
type FileInfo struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	IsDir         bool   `json:"isDir"`
	IsSymlink     bool   `json:"isSymlink"`
	ModTime       int64  `json:"modTime"`
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse 成功响应结构
type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// 初始化函数
func init() {
	// 确保上传目录存在
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		log.Fatalf("无法创建上传目录: %v", err)
	}

	// 确保应用目录存在
	if err := os.MkdirAll(appRootDir, 0755); err != nil {
		log.Fatalf("无法创建应用目录: %v", err)
	}
}

// ensurePathInRoot 确保路径在根目录内（修复了 HasPrefix 绕过问题）
func ensurePathInRoot(path string) (string, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	absRoot = filepath.Clean(absRoot)

	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(rootDir, path)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	absPath = filepath.Clean(absPath)

	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	// 如果相对路径以 .. 开头，则不在 root 内
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("路径不在允许的目录范围内")
	}
	return absPath, nil
}

// writeJSON 统一 JSON 返回
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 统一错误返回
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

// listDirectory 列出目录内容（用 os.ReadDir，更现代）
func listDirectory(path string) ([]FileInfo, error) {
	fullPath, err := ensurePathInRoot(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("指定的路径不是目录")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var fileInfos []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryPath := filepath.Join(fullPath, info.Name())
		relPath, _ := filepath.Rel(rootDir, entryPath)
		fileInfo := FileInfo{
			Name:    info.Name(),
			Path:    relPath,
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Unix(),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			fileInfo.IsSymlink = true
			if target, err := os.Readlink(entryPath); err == nil {
				fileInfo.SymlinkTarget = target
			}
		}
		fileInfos = append(fileInfos, fileInfo)
	}
	return fileInfos, nil
}

// getDirectoryTree 遍历目录树
func getDirectoryTree() ([]FileInfo, error) {
	var allFiles []FileInfo
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 跳过根目录本身
		if path == rootDir {
			return nil
		}
		// 过滤 _h5ai
		if strings.HasPrefix(strings.ToLower(info.Name()), "_h5ai") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, _ := filepath.Rel(rootDir, path)
		fileInfo := FileInfo{
			Name:    info.Name(),
			Path:    relPath,
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Unix(),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			fileInfo.IsSymlink = true
			if target, err := os.Readlink(path); err == nil {
				fileInfo.SymlinkTarget = target
			}
		}
		allFiles = append(allFiles, fileInfo)
		return nil
	})
	return allFiles, err
}

// handleDirectoryList 处理目录列表请求
func handleDirectoryList(w http.ResponseWriter, r *http.Request) {
	// 支持两种前缀形式，兼容原逻辑
	var pathParam string
	if strings.HasPrefix(r.URL.Path, "/filesuploader/api/directory/list/") {
		pathParam = strings.TrimPrefix(r.URL.Path, "/filesuploader/api/directory/list/")
	} else {
		pathParam = strings.TrimPrefix(r.URL.Path, "/api/directory/list/")
	}
	if pathParam == "" {
		pathParam = "."
	}

	files, err := listDirectory(pathParam)
	if err != nil {
		log.Printf("列出目录失败: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 如果目录存在但没有文件，返回友好提示（避免前端收到“数据格式错误”之类的提示）
	if len(files) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"path":    pathParam,
			"files":   []FileInfo{},
			"message": "暂时没有发现文件",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":  pathParam,
		"files": files,
	})
}

// handleDirectoryTree 处理目录树请求
func handleDirectoryTree(w http.ResponseWriter, r *http.Request) {
	files, err := getDirectoryTree()
	if err != nil {
		log.Printf("获取目录树失败: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 若需要同样行为（无文件时提示），也可在这里处理；目前保持原样返回 files 数组
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"files": files,
	})
}

// handleFileUpload 处理文件上传（改进：限制请求大小，文件名清洗，确保目录存在）
func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 限制整个请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	log.Printf("收到文件上传请求: 方法=%s, 内容类型=%s", r.Method, r.Header.Get("Content-Type"))

	// ParseMultipartForm 的参数是内存缓冲限制（过大的文件会使用临时文件）
	if err := r.ParseMultipartForm(memoryBufferSize); err != nil {
		log.Printf("解析multipart表单失败: %v", err)
		writeError(w, http.StatusBadRequest, fmt.Sprintf("无法解析请求: %v", err))
		return
	}

	// path 参数（相对 root）
	pathParam := r.FormValue("path")
	if pathParam == "" {
		pathParam = "."
	}
	log.Printf("上传路径: %s", pathParam)

	// 确保路径在 root 内，并获取绝对路径
	fullPath, err := ensurePathInRoot(pathParam)
	if err != nil {
		log.Printf("路径验证失败: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 确保目标目录存在
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		log.Printf("无法创建目标目录 %s: %v", fullPath, err)
		writeError(w, http.StatusInternalServerError, "无法创建目标目录")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		log.Printf("未找到上传的文件")
		writeError(w, http.StatusBadRequest, "没有找到上传的文件")
		return
	}

	var errorsList []string

	for _, fileHeader := range files {
		// 使用 Base 防止文件名中包含路径
		cleanName := filepath.Base(fileHeader.Filename)
		log.Printf("处理文件: %s (大小: %d 字节)", cleanName, fileHeader.Size)

		if fileHeader.Size > maxUploadSize {
			errMsg := fmt.Sprintf("文件 %s 太大", cleanName)
			log.Printf(errMsg)
			errorsList = append(errorsList, errMsg)
			continue
		}

		src, err := fileHeader.Open()
		if err != nil {
			errMsg := fmt.Sprintf("无法打开文件 %s: %v", cleanName, err)
			log.Printf(errMsg)
			errorsList = append(errorsList, errMsg)
			continue
		}

		dstPath := filepath.Join(fullPath, cleanName)
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			errMsg := fmt.Sprintf("无法创建文件 %s: %v", cleanName, err)
			log.Printf(errMsg)
			errorsList = append(errorsList, errMsg)
			continue
		}

		// 复制并立即关闭资源
		if _, err = io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			errMsg := fmt.Sprintf("无法保存文件 %s: %v", cleanName, err)
			log.Printf(errMsg)
			errorsList = append(errorsList, errMsg)
			continue
		}
		src.Close()
		dst.Close()

		// 设置合理权限（可按需调整）
		if err := os.Chmod(dstPath, 0644); err != nil {
			log.Printf("设置权限失败: %v", err)
		}

		log.Printf("文件上传成功: %s -> %s", cleanName, dstPath)
	}

	resp := map[string]interface{}{
		"success": len(errorsList) == 0,
		"message": "文件上传完成",
	}
	if len(errorsList) > 0 {
		resp["errors"] = errorsList
		log.Printf("上传完成，但有错误: %v", errorsList)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCreateDirectory 创建目录
func handleCreateDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "无法解析请求")
		return
	}
	parentPath := r.FormValue("parentPath")
	name := r.FormValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "目录名称不能为空")
		return
	}
	// 只保留 base 名称，避免 name 中带路径
	name = filepath.Base(name)
	fullPath := filepath.Join(parentPath, name)
	absPath, err := ensurePathInRoot(fullPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("无法创建目录: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, SuccessResponse{Message: "目录创建成功"})
}

// handleCreateSymlink 创建软链接（保留 /mnt 限制）
func handleCreateSymlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "无法解析请求")
		return
	}
	parentPath := r.FormValue("parentPath")
	name := r.FormValue("name")
	target := r.FormValue("target")
	if name == "" || target == "" {
		writeError(w, http.StatusBadRequest, "链接名称和目标路径不能为空")
		return
	}
	if !strings.HasPrefix(target, "/mnt") {
		writeError(w, http.StatusForbidden, "禁止创建/mnt以外的软链接，为了保护系统文件安全")
		return
	}
	// 限制 name
	name = filepath.Base(name)
	fullPath := filepath.Join(parentPath, name)
	absPath, err := ensurePathInRoot(fullPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Symlink(target, absPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("无法创建软链接: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, SuccessResponse{Message: "软链接创建成功"})
}

// handleRenameFile 重命名（防止 newName 中包含路径）
func handleRenameFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "无法解析请求")
		return
	}
	oldPath := r.FormValue("oldPath")
	newName := r.FormValue("newName")
	if oldPath == "" || newName == "" {
		writeError(w, http.StatusBadRequest, "原路径和新名称不能为空")
		return
	}
	absOldPath, err := ensurePathInRoot(oldPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	newName = filepath.Base(newName)
	dir := filepath.Dir(absOldPath)
	absNewPath := filepath.Join(dir, newName)
	// 再次检查是否在 root 内
	if _, err := ensurePathInRoot(absNewPath); err != nil {
		writeError(w, http.StatusBadRequest, "新路径不在允许的目录范围内")
		return
	}
	if err := os.Rename(absOldPath, absNewPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("无法重命名文件: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, SuccessResponse{Message: "文件重命名成功"})
}

// handleDeleteFile 删除文件或目录
func handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var pathParam string
	if strings.HasPrefix(r.URL.Path, "/filesuploader/api/file/delete/") {
		pathParam = strings.TrimPrefix(r.URL.Path, "/filesuploader/api/file/delete/")
	} else {
		pathParam = strings.TrimPrefix(r.URL.Path, "/api/file/delete/")
	}
	if pathParam == "" {
		writeError(w, http.StatusBadRequest, "路径不能为空")
		return
	}
	absPath, err := ensurePathInRoot(pathParam)
	if err != nil {
		log.Printf("删除路径检查失败: 请求路径=%s, 错误=%v", pathParam, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("路径不存在: %v", err))
		return
	}
	if info.IsDir() {
		if err := os.RemoveAll(absPath); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("无法删除目录: %v", err))
			return
		}
	} else {
		if err := os.Remove(absPath); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("无法删除文件: %v", err))
			return
		}
	}
	writeJSON(w, http.StatusOK, SuccessResponse{Message: "删除成功"})
}

// getContentType 改进：使用 mime 包优先判断
func getContentType(filePath string) string {
	if ext := filepath.Ext(filePath); ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
	// fallback 常见扩展
	l := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(l, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(l, ".css"):
		return "text/css"
	case strings.HasSuffix(l, ".js"):
		return "application/javascript"
	case strings.HasSuffix(l, ".json"):
		return "application/json"
	case strings.HasSuffix(l, ".png"):
		return "image/png"
	case strings.HasSuffix(l, ".jpg"), strings.HasSuffix(l, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(l, ".gif"):
		return "image/gif"
	case strings.HasSuffix(l, ".svg"):
		return "image/svg+xml"
	}
	return "application/octet-stream"
}

// handleIndex 与 handleFilesUploaderIndex 保持原有行为，但改进静态返回时加上 nosniff 头
func handleIndex(w http.ResponseWriter, r *http.Request) {
	log.Printf("首页请求: %s", r.URL.Path)
	if r.URL.Path == "/" {
		file, err := staticFiles.Open("static/index.html")
		if err != nil {
			log.Printf("无法打开index.html: %v", err)
			writeError(w, http.StatusInternalServerError, "无法打开首页")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, file)
		return
	}
	// 静态资源处理 (兼容 /static, /css, /js, /index.html)
	if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/css/") || strings.HasPrefix(r.URL.Path, "/js/") || r.URL.Path == "/index.html" {
		originalPath := r.URL.Path
		targetPath := originalPath
		if r.URL.Path == "/index.html" {
			targetPath = "static/index.html"
		} else if strings.HasPrefix(r.URL.Path, "/static/") {
			targetPath = strings.TrimPrefix(r.URL.Path, "/")
		} else if strings.HasPrefix(r.URL.Path, "/css/") || strings.HasPrefix(r.URL.Path, "/js/") {
			targetPath = "static" + r.URL.Path
		}
		log.Printf("静态文件请求: %s -> %s", originalPath, targetPath)
		file, err := staticFiles.Open(targetPath)
		if err != nil {
			log.Printf("无法打开静态文件 %s: %v", targetPath, err)
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", getContentType(targetPath))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, file)
		return
	}
	http.NotFound(w, r)
}

func handleFilesUploaderIndex(w http.ResponseWriter, r *http.Request) {
	log.Printf("FilesUploader首页请求: %s", r.URL.Path)
	if r.URL.Path == "/filesuploader" || r.URL.Path == "/filesuploader/" {
		file, err := staticFiles.Open("static/index.html")
		if err != nil {
			log.Printf("无法打开index.html: %v", err)
			writeError(w, http.StatusInternalServerError, "无法打开首页")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, file)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/filesuploader/") {
		originalPath := r.URL.Path
		relativePath := strings.TrimPrefix(r.URL.Path, "/filesuploader")
		targetPath := ""
		if relativePath == "/index.html" {
			targetPath = "static/index.html"
		} else if strings.HasPrefix(relativePath, "/static/") {
			targetPath = strings.TrimPrefix(relativePath, "/")
		} else if strings.HasPrefix(relativePath, "/css/") || strings.HasPrefix(relativePath, "/js/") {
			targetPath = "static" + relativePath
		}
		log.Printf("FilesUploader静态文件请求: %s -> %s", originalPath, targetPath)
		if targetPath != "" {
			file, err := staticFiles.Open(targetPath)
			if err != nil {
				log.Printf("无法打开静态文件 %s: %v", targetPath, err)
				http.NotFound(w, r)
				return
			}
			defer file.Close()
			w.Header().Set("Content-Type", getContentType(targetPath))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = io.Copy(w, file)
			return
		}
	}
	http.NotFound(w, r)
}

func main() {
	mux := http.NewServeMux()

	// 根及 API 路由
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/directory/list/", handleDirectoryList)
	mux.HandleFunc("/api/directory/list", handleDirectoryList)
	mux.HandleFunc("/api/directory/tree", handleDirectoryTree)
	mux.HandleFunc("/api/file/upload", handleFileUpload)
	mux.HandleFunc("/api/directory/create", handleCreateDirectory)
	mux.HandleFunc("/api/directory/symlink", handleCreateSymlink)
	mux.HandleFunc("/api/file/rename", handleRenameFile)
	mux.HandleFunc("/api/file/delete/", handleDeleteFile)

	// /filesuploader 前缀兼容路由
	mux.HandleFunc("/filesuploader", handleFilesUploaderIndex)
	mux.HandleFunc("/filesuploader/", handleFilesUploaderIndex)
	mux.HandleFunc("/filesuploader/api/directory/list/", handleDirectoryList)
	mux.HandleFunc("/filesuploader/api/directory/list", handleDirectoryList)
	mux.HandleFunc("/filesuploader/api/directory/tree", handleDirectoryTree)
	mux.HandleFunc("/filesuploader/api/file/upload", handleFileUpload)
	mux.HandleFunc("/filesuploader/api/directory/create", handleCreateDirectory)
	mux.HandleFunc("/filesuploader/api/directory/symlink", handleCreateSymlink)
	mux.HandleFunc("/filesuploader/api/file/rename", handleRenameFile)
	mux.HandleFunc("/filesuploader/api/file/delete/", handleDeleteFile)

	// 使用带超时的 http.Server 增强健壮性
	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("文件上传服务启动 - 地址: %s, 上传目录: %s, 最大大小: %d MB",
		listenAddr, rootDir, maxUploadSize/(1024*1024))

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
