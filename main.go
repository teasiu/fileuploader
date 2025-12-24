package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 配置
var (
	rootDir          = "/var/www/html/files"
	listenAddr       = "0.0.0.0:6012"
	maxUploadSize    = int64(8 * 1024 * 1024 * 1024) // 8GB (可根据sda实际容量调整)
	memoryBufferSize = int64(16 * 1024 * 1024)       // 16MB内存缓冲区
	// 程序根目录
	appRootDir = "/opt/fileuploader"
)

// 并发控制
var (
	uploadSemaphore = make(chan struct{}, 2)          // 最大并发上传数
	uploadTimeoutMu sync.Mutex
	activeUploads   = make(map[string]*time.Timer)
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

// 确保路径在根目录内
func ensurePathInRoot(path string) (string, error) {
	// 规范化路径
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	// 处理相对路径
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(rootDir, path)
	}

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	// 检查路径是否在根目录内
	if !strings.HasPrefix(absPath, absRoot) {
		return "", fmt.Errorf("路径不在允许的目录范围内")
	}

	return absPath, nil
}

// 列出目录内容
func listDirectory(path string) ([]FileInfo, error) {
	// 确保路径在根目录内
	fullPath, err := ensurePathInRoot(path)
	if err != nil {
		return nil, err
	}

	// 检查路径是否存在
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	// 检查是否是目录
	if !info.IsDir() {
		return nil, fmt.Errorf("指定的路径不是目录")
	}

	// 读取目录内容
	dirEntries, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	// 构建文件信息列表
	var fileInfos []FileInfo
	for _, info := range dirEntries {
		// 构建完整路径
		entryPath := filepath.Join(fullPath, info.Name())

		// 获取文件信息
		fileInfo := FileInfo{
			Name:    info.Name(),
			Path:    strings.TrimPrefix(entryPath, rootDir+"/"),
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Unix(),
		}

		// 检查是否是软链接
		if info.Mode()&os.ModeSymlink != 0 {
			fileInfo.IsSymlink = true
			// 获取软链接目标
			if target, err := os.Readlink(entryPath); err == nil {
				fileInfo.SymlinkTarget = target
			}
		}

		fileInfos = append(fileInfos, fileInfo)
	}

	return fileInfos, nil
}

// 获取目录树
func getDirectoryTree() ([]FileInfo, error) {
	var allFiles []FileInfo

	// 递归遍历目录
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录本身
		if path == rootDir {
			return nil
		}

		// 过滤掉以_h5ai开头的文件和目录
		if strings.HasPrefix(strings.ToLower(info.Name()), "_h5ai") {
			// 如果是目录，跳过其所有子内容
			if info.IsDir() {
				return filepath.SkipDir
			}
			// 如果是文件，直接跳过
			return nil
		}

		// 构建文件信息
		fileInfo := FileInfo{
			Name:    info.Name(),
			Path:    strings.TrimPrefix(path, rootDir+"/"),
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Unix(),
		}

		// 检查是否是软链接
		if info.Mode()&os.ModeSymlink != 0 {
			fileInfo.IsSymlink = true
			// 获取软链接目标
			if target, err := os.Readlink(path); err == nil {
				fileInfo.SymlinkTarget = target
			}
		}

		allFiles = append(allFiles, fileInfo)
		return nil
	})

	return allFiles, err
}

// 处理目录列表请求
func handleDirectoryList(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数，支持两种路径格式
	var path string
	if strings.HasPrefix(r.URL.Path, "/filesuploader/api/directory/list/") {
		// 处理带/filesuploader前缀的路径
		path = strings.TrimPrefix(r.URL.Path, "/filesuploader/api/directory/list/")
	} else {
		// 处理不带前缀的路径
		path = strings.TrimPrefix(r.URL.Path, "/api/directory/list/")
	}
	
	if path == "" {
		path = "."
	}

	// 列出目录内容
	files, err := listDirectory(path)
	if err != nil {
		log.Printf("错误: 列出目录失败 - %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 返回JSON响应
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"path":  path,
		"files": files,
	}
	json.NewEncoder(w).Encode(response)
}

// 处理目录树请求
func handleDirectoryTree(w http.ResponseWriter, r *http.Request) {
	// 获取目录树
	files, err := getDirectoryTree()
	if err != nil {
		log.Printf("错误: 获取目录树失败 - %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回JSON响应
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"files": files,
	}
	json.NewEncoder(w).Encode(response)
}

// 处理文件上传
func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 记录请求信息用于调试
	log.Printf("收到文件上传请求: 方法=%s, 内容类型=%s", r.Method, r.Header.Get("Content-Type"))

	// 解析请求
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		log.Printf("解析multipart表单失败: %v", err)
		http.Error(w, fmt.Sprintf("无法解析请求: %v", err), http.StatusBadRequest)
		return
	}

	// 获取路径参数
	path := r.FormValue("path")
	if path == "" {
		path = "."
	}
	log.Printf("上传路径: %s", path)

	// 确保路径在根目录内
	fullPath, err := ensurePathInRoot(path)
	if err != nil {
		log.Printf("路径验证失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 获取上传的文件
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		log.Printf("未找到上传的文件")
		http.Error(w, "没有找到上传的文件", http.StatusBadRequest)
		return
	}

	log.Printf("找到 %d 个文件待上传", len(files))

	var errors []string

	// 处理每个文件
	for _, fileHeader := range files {
		log.Printf("处理文件: %s (大小: %d 字节)", fileHeader.Filename, fileHeader.Size)

		// 检查文件大小
		if fileHeader.Size > maxUploadSize {
			errMsg := fmt.Sprintf("文件 %s 太大", fileHeader.Filename)
			log.Printf(errMsg)
			errors = append(errors, errMsg)
			continue
		}

		// 打开上传的文件
		src, err := fileHeader.Open()
		if err != nil {
			errMsg := fmt.Sprintf("无法打开文件 %s: %v", fileHeader.Filename, err)
			log.Printf(errMsg)
			errors = append(errors, errMsg)
			continue
		}

		// 创建目标文件
		dstPath := filepath.Join(fullPath, fileHeader.Filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			errMsg := fmt.Sprintf("无法创建文件 %s: %v", fileHeader.Filename, err)
			log.Printf(errMsg)
			errors = append(errors, errMsg)
			continue
		}

		// 复制文件内容
		if _, err = io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			errMsg := fmt.Sprintf("无法保存文件 %s: %v", fileHeader.Filename, err)
			log.Printf(errMsg)
			errors = append(errors, errMsg)
			continue
		}

		// 关闭文件
		src.Close()
		dst.Close()

		log.Printf("文件上传成功: %s -> %s", fileHeader.Filename, dstPath)
	}

	// 返回响应
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": len(errors) == 0,
		"message": "文件上传完成",
	}

	if len(errors) > 0 {
		response["errors"] = errors
		log.Printf("上传完成，但有错误: %v", errors)
	}

	json.NewEncoder(w).Encode(response)
}

// 处理创建目录请求
func handleCreateDirectory(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求
	if err := r.ParseForm(); err != nil {
		http.Error(w, "无法解析请求", http.StatusBadRequest)
		return
	}

	// 获取参数
	parentPath := r.FormValue("parentPath")
	name := r.FormValue("name")

	if name == "" {
		http.Error(w, "目录名称不能为空", http.StatusBadRequest)
		return
	}

	// 构建完整路径
	fullPath := filepath.Join(parentPath, name)

	// 确保路径在根目录内
	absPath, err := ensurePathInRoot(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 创建目录
	if err := os.Mkdir(absPath, 0755); err != nil {
		http.Error(w, fmt.Sprintf("无法创建目录: %v", err), http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "目录创建成功",
	})
}

// 处理创建软链接请求
func handleCreateSymlink(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求
	if err := r.ParseForm(); err != nil {
		http.Error(w, "无法解析请求", http.StatusBadRequest)
		return
	}

	// 获取参数
	parentPath := r.FormValue("parentPath")
	name := r.FormValue("name")
	target := r.FormValue("target")

	if name == "" || target == "" {
		http.Error(w, "链接名称和目标路径不能为空", http.StatusBadRequest)
		return
	}

	// 限制软链接目标路径必须以/mnt开头
	if !strings.HasPrefix(target, "/mnt") {
		http.Error(w, "禁止创建/mnt以外的软链接，为了保护系统文件安全", http.StatusForbidden)
		return
	}

	// 构建完整路径
	fullPath := filepath.Join(parentPath, name)

	// 确保路径在根目录内
	absPath, err := ensurePathInRoot(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 创建软链接
	if err := os.Symlink(target, absPath); err != nil {
		http.Error(w, fmt.Sprintf("无法创建软链接: %v", err), http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "软链接创建成功",
	})
}

// 处理文件重命名请求
func handleRenameFile(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求
	if err := r.ParseForm(); err != nil {
		http.Error(w, "无法解析请求", http.StatusBadRequest)
		return
	}

	// 获取参数
	oldPath := r.FormValue("oldPath")
	newName := r.FormValue("newName")

	if oldPath == "" || newName == "" {
		http.Error(w, "原路径和新名称不能为空", http.StatusBadRequest)
		return
	}

	// 确保原路径在根目录内
	absOldPath, err := ensurePathInRoot(oldPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 构建新路径
	dir := filepath.Dir(absOldPath)
	absNewPath := filepath.Join(dir, newName)

	// 检查新路径是否在根目录内
	if !strings.HasPrefix(absNewPath, rootDir) {
		http.Error(w, "新路径不在允许的目录范围内", http.StatusBadRequest)
		return
	}

	// 重命名文件
	if err := os.Rename(absOldPath, absNewPath); err != nil {
		http.Error(w, fmt.Sprintf("无法重命名文件: %v", err), http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "文件重命名成功",
	})
}

// 处理文件删除请求
func handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取路径参数，支持两种路径格式
	var path string
	if strings.HasPrefix(r.URL.Path, "/filesuploader/api/file/delete/") {
		// 处理带/filesuploader前缀的路径
		path = strings.TrimPrefix(r.URL.Path, "/filesuploader/api/file/delete/")
	} else {
		// 处理不带前缀的路径
		path = strings.TrimPrefix(r.URL.Path, "/api/file/delete/")
	}
	
	if path == "" {
		http.Error(w, "路径不能为空", http.StatusBadRequest)
		return
	}

	// 确保路径在根目录内
	absPath, err := ensurePathInRoot(path)
	if err != nil {
		log.Printf("删除路径检查失败: 请求路径=%s, 错误=%v", path, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 检查路径是否存在
	info, err := os.Stat(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("路径不存在: %v", err), http.StatusNotFound)
		return
	}

	// 删除文件或目录
	if info.IsDir() {
		// 删除目录
		if err := os.RemoveAll(absPath); err != nil {
			http.Error(w, fmt.Sprintf("无法删除目录: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// 删除文件
		if err := os.Remove(absPath); err != nil {
			http.Error(w, fmt.Sprintf("无法删除文件: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "删除成功",
	})
}

// 获取内容类型的辅助函数
func getContentType(filePath string) string {
	if strings.HasSuffix(filePath, ".html") {
		return "text/html; charset=utf-8"
	} else if strings.HasSuffix(filePath, ".css") {
		return "text/css"
	} else if strings.HasSuffix(filePath, ".js") {
		return "application/javascript"
	} else if strings.HasSuffix(filePath, ".json") {
		return "application/json"
	} else if strings.HasSuffix(filePath, ".png") {
		return "image/png"
	} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
		return "image/jpeg"
	} else if strings.HasSuffix(filePath, ".gif") {
		return "image/gif"
	} else if strings.HasSuffix(filePath, ".svg") {
		return "image/svg+xml"
	}
	return "application/octet-stream"
}

// 处理首页请求（根路径）
func handleIndex(w http.ResponseWriter, r *http.Request) {
	// 记录首页请求
	log.Printf("首页请求: %s", r.URL.Path)
	
	// 如果是根路径，直接返回index.html内容
	if r.URL.Path == "/" {
		// 打开并返回index.html文件
		file, err := staticFiles.Open("static/index.html")
		if err != nil {
			log.Printf("无法打开index.html: %v", err)
			http.Error(w, "无法打开首页", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		
		// 设置内容类型
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		
		// 复制文件内容到响应
		io.Copy(w, file)
		return
	}
	
	// 直接处理静态文件请求
	if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/css/") || strings.HasPrefix(r.URL.Path, "/js/") || r.URL.Path == "/index.html" {
		originalPath := r.URL.Path
		targetPath := originalPath
		
		// 路径转换逻辑
		if r.URL.Path == "/index.html" {
			targetPath = "static/index.html"
		} else if strings.HasPrefix(r.URL.Path, "/static/") {
			// 已经是正确的路径格式
			targetPath = strings.TrimPrefix(r.URL.Path, "/")
		} else if strings.HasPrefix(r.URL.Path, "/css/") || strings.HasPrefix(r.URL.Path, "/js/") {
			// 需要添加static前缀
			targetPath = "static" + r.URL.Path
		}
		
		log.Printf("静态文件请求: %s -> %s", originalPath, targetPath)
		
		// 直接从嵌入文件系统读取文件
		file, err := staticFiles.Open(targetPath)
		if err != nil {
			log.Printf("无法打开静态文件 %s: %v", targetPath, err)
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		
		// 设置正确的内容类型
		contentType := getContentType(targetPath)
		w.Header().Set("Content-Type", contentType)
		
		// 复制文件内容到响应
		io.Copy(w, file)
		return
	}
	
	// 其他路径返回404
	http.NotFound(w, r)
}

// 处理/filesuploader路径的首页请求
func handleFilesUploaderIndex(w http.ResponseWriter, r *http.Request) {
	// 记录首页请求
	log.Printf("FilesUploader首页请求: %s", r.URL.Path)
	
	// 如果是/filesuploader根路径，直接返回index.html内容
	if r.URL.Path == "/filesuploader" || r.URL.Path == "/filesuploader/" {
		// 打开并返回index.html文件
		file, err := staticFiles.Open("static/index.html")
		if err != nil {
			log.Printf("无法打开index.html: %v", err)
			http.Error(w, "无法打开首页", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		
		// 设置内容类型
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		
		// 复制文件内容到响应
		io.Copy(w, file)
		return
	}
	
	// 处理/filesuploader下的静态文件请求
	if strings.HasPrefix(r.URL.Path, "/filesuploader/") {
		originalPath := r.URL.Path
		targetPath := ""
		
		// 移除/filesuploader前缀
		relativePath := strings.TrimPrefix(r.URL.Path, "/filesuploader")
		
		// 路径转换逻辑
		if relativePath == "/index.html" {
			targetPath = "static/index.html"
		} else if strings.HasPrefix(relativePath, "/static/") {
			targetPath = strings.TrimPrefix(relativePath, "/")
		} else if strings.HasPrefix(relativePath, "/css/") || strings.HasPrefix(relativePath, "/js/") {
			targetPath = "static" + relativePath
		}
		
		log.Printf("FilesUploader静态文件请求: %s -> %s", originalPath, targetPath)
		
		if targetPath != "" {
			// 直接从嵌入文件系统读取文件
			file, err := staticFiles.Open(targetPath)
			if err != nil {
				log.Printf("无法打开静态文件 %s: %v", targetPath, err)
				http.NotFound(w, r)
				return
			}
			defer file.Close()
			
			// 设置正确的内容类型
			contentType := getContentType(targetPath)
			w.Header().Set("Content-Type", contentType)
			
			// 复制文件内容到响应
			io.Copy(w, file)
			return
		}
	}
	
	// 其他路径返回404
	http.NotFound(w, r)
}

func main() {
	// 创建主路由mux
	mux := http.NewServeMux()
	
	// 注册根路径的路由
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/directory/list/", handleDirectoryList)
	mux.HandleFunc("/api/directory/list", handleDirectoryList) // 支持不带斜杠的路径
	mux.HandleFunc("/api/directory/tree", handleDirectoryTree)
	mux.HandleFunc("/api/file/upload", handleFileUpload)
	mux.HandleFunc("/api/directory/create", handleCreateDirectory)
	mux.HandleFunc("/api/directory/symlink", handleCreateSymlink)
	mux.HandleFunc("/api/file/rename", handleRenameFile)
	mux.HandleFunc("/api/file/delete/", handleDeleteFile)
	
	// 注册/filesuploader路径的路由（Nginx保留前缀时使用）
	mux.HandleFunc("/filesuploader", handleFilesUploaderIndex)
	mux.HandleFunc("/filesuploader/", handleFilesUploaderIndex)
	mux.HandleFunc("/filesuploader/api/directory/list/", handleDirectoryList)
	mux.HandleFunc("/filesuploader/api/directory/list", handleDirectoryList) // 支持不带斜杠的路径
	mux.HandleFunc("/filesuploader/api/directory/tree", handleDirectoryTree)
	mux.HandleFunc("/filesuploader/api/file/upload", handleFileUpload)
	mux.HandleFunc("/filesuploader/api/directory/create", handleCreateDirectory)
	mux.HandleFunc("/filesuploader/api/directory/symlink", handleCreateSymlink)
	mux.HandleFunc("/filesuploader/api/file/rename", handleRenameFile)
	mux.HandleFunc("/filesuploader/api/file/delete/", handleDeleteFile) // 修复：使用正确的处理函数
	
	// 注册静态文件处理（现在通过handleIndex和handleFilesUploaderIndex处理）
	// 不再需要单独的静态文件路由
	
	// 主路由mux已在上方创建，这里不需要重复声明
	
	// 启动服务器
	log.Printf("文件上传服务启动 - 地址: %s, 上传目录: %s, 最大大小: %d MB", 
		listenAddr, rootDir, maxUploadSize/(1024*1024))
	
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}