package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type bufferedReadCloser struct {
	bufReader *bufio.Reader
	rawCloser io.ReadCloser
}

func (brc *bufferedReadCloser) Read(p []byte) (int, error) {
	return brc.bufReader.Read(p)
}

func (brc *bufferedReadCloser) Close() error {
	return brc.rawCloser.Close()
}

var (
	rootDir       = "/var/www/html/files"
	listenAddr    = "0.0.0.0:6012"
	maxUploadSize = int64(8 * 1024 * 1024 * 1024)
	appRootDir    = "/opt/fileuploader"
)

//go:embed static/* static/css/* static/js/* static/fonts/*
var staticFiles embed.FS

type FileInfo struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	IsDir         bool   `json:"isDir"`
	IsSymlink     bool   `json:"isSymlink"`
	ModTime       int64  `json:"modTime"`
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func init() {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		log.Fatalf("无法创建上传目录: %v", err)
	}

	if err := os.MkdirAll(appRootDir, 0755); err != nil {
		log.Fatalf("无法创建应用目录: %v", err)
	}
}

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
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("路径不在允许的目录范围内")
	}
	return absPath, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

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

func getDirectoryTree() ([]FileInfo, error) {
	var allFiles []FileInfo
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == rootDir {
			return nil
		}
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

func handleDirectoryList(w http.ResponseWriter, r *http.Request) {
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

func handleDirectoryTree(w http.ResponseWriter, r *http.Request) {
	files, err := getDirectoryTree()
	if err != nil {
		log.Printf("获取目录树失败: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"files": files,
	})
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	bufReader := bufio.NewReaderSize(r.Body, 1024*1024)
	r.Body = &bufferedReadCloser{
		bufReader: bufReader,
		rawCloser: r.Body,
	}

	log.Printf("收到文件上传请求: 方法=%s, 内容类型=%s", r.Method, r.Header.Get("Content-Type"))

	reader, err := r.MultipartReader()
	if err != nil {
		log.Printf("解析multipart读取器失败: %v", err)
		writeError(w, http.StatusBadRequest, fmt.Sprintf("无法解析请求: %v", err))
		return
	}

	var pathParam string
	type bufferedFile struct {
		tempPath string
		fileName string
		fileSize int64
	}
	var bufferedFiles []bufferedFile
	var errorsList []string
	var hasFiles bool

	for {
		part, err := reader.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("读取表单部分失败: %v", err)
			if strings.Contains(err.Error(), "timeout") {
				writeError(w, http.StatusRequestTimeout, "读取请求参数超时，请重试")
				return
			}
			writeError(w, http.StatusBadRequest, "读取请求数据失败")
			return
		}

		if part.FileName() == "" {
			fieldName := part.FormName()
			if fieldName == "path" {
				pathBytes, err := io.ReadAll(part)
				if err != nil {
					log.Printf("读取path参数失败: %v", err)
					writeError(w, http.StatusBadRequest, "获取上传路径失败")
					return
				}
				pathParam = string(pathBytes)
			}
			_ = part.Close()
			continue
		}

		hasFiles = true
		tempPath, written, err := savePartToTempFile(part, maxUploadSize)
		if err != nil {
			errMsg := fmt.Sprintf("无法保存文件 %s: %v", filepath.Base(part.FileName()), err)
			log.Printf(errMsg)
			errorsList = append(errorsList, errMsg)
			continue
		}
		bufferedFiles = append(bufferedFiles, bufferedFile{
			tempPath: tempPath,
			fileName: filepath.Base(part.FileName()),
			fileSize: written,
		})
	}

	if !hasFiles {
		log.Printf("未找到上传的文件")
		writeError(w, http.StatusBadRequest, "没有找到上传的文件")
		return
	}

	if pathParam == "" {
		pathParam = "."
	}
	log.Printf("上传路径: %s", pathParam)

	fullPath, err := ensurePathInRoot(pathParam)
	if err != nil {
		log.Printf("路径验证失败: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		log.Printf("无法创建目标目录 %s: %v", fullPath, err)
		writeError(w, http.StatusInternalServerError, "无法创建目标目录")
		return
	}

	for _, file := range bufferedFiles {
		dstPath := filepath.Join(fullPath, file.fileName)
		if err := moveTempFile(file.tempPath, dstPath); err != nil {
			errMsg := fmt.Sprintf("无法移动文件 %s: %v", file.fileName, err)
			log.Printf(errMsg)
			errorsList = append(errorsList, errMsg)
			continue
		}
		if err := os.Chmod(dstPath, 0644); err != nil {
			log.Printf("设置权限失败 %s: %v", file.fileName, err)
		}
		log.Printf("文件上传成功: %s -> %s（大小: %d 字节）", file.fileName, dstPath, file.fileSize)
	}

	resp := map[string]interface{}{
		"success": len(errorsList) == 0,
		"message": "文件上传完成（部分文件可能失败）",
	}
	if len(errorsList) > 0 {
		resp["errors"] = errorsList
		log.Printf("上传完成，但有错误: %v", errorsList)
	}
	writeJSON(w, http.StatusOK, resp)
}

func savePartToTempFile(part *multipart.Part, maxSize int64) (string, int64, error) {
	defer part.Close()
	cleanName := filepath.Base(part.FileName())
	tempFile, err := os.CreateTemp("", "fileuploader-*")
	if err != nil {
		return "", 0, err
	}

	written, err := io.Copy(tempFile, io.LimitReader(part, maxSize+1))
	if err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", 0, err
	}

	if written > maxSize {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", 0, fmt.Errorf("文件 %s 太大（超过 %d GB）", cleanName, maxSize/(1024*1024*1024))
	}

	if err := tempFile.Sync(); err != nil {
		log.Printf("临时文件同步失败 %s: %v", cleanName, err)
	}
	if err := tempFile.Close(); err != nil {
		log.Printf("关闭临时文件失败 %s: %v", cleanName, err)
	}
	return tempFile.Name(), written, nil
}

func moveTempFile(srcPath, dstPath string) error {
	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	if err := dstFile.Sync(); err != nil {
		log.Printf("目标文件同步失败 %s: %v", dstPath, err)
	}
	if err := os.Remove(srcPath); err != nil {
		log.Printf("删除临时文件失败 %s: %v", srcPath, err)
	}
	return nil
}

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

func getContentType(filePath string) string {
	if ext := filepath.Ext(filePath); ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
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

	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/directory/list/", handleDirectoryList)
	mux.HandleFunc("/api/directory/list", handleDirectoryList)
	mux.HandleFunc("/api/directory/tree", handleDirectoryTree)
	mux.HandleFunc("/api/file/upload", handleFileUpload)
	mux.HandleFunc("/api/directory/create", handleCreateDirectory)
	mux.HandleFunc("/api/directory/symlink", handleCreateSymlink)
	mux.HandleFunc("/api/file/rename", handleRenameFile)
	mux.HandleFunc("/api/file/delete/", handleDeleteFile)

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

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadTimeout:       1800 * time.Second,
		WriteTimeout:      1800 * time.Second,
		IdleTimeout:       300 * time.Second,
		ReadHeaderTimeout: 60 * time.Second,
	}

	log.Printf("文件上传服务启动 - 地址: %s, 上传目录: %s, 最大大小: %d GB",
		listenAddr, rootDir, maxUploadSize/(1024*1024*1024))

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
