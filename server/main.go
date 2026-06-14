package main

import (
	"archive/zip"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const VERSION = "2.0.0"
const DEFAULT_MAX_UPLOAD_MB = 500
const multipartOverheadBytes int64 = 1 << 20

type FileMeta struct {
	Code          string
	FilePath      string
	FileName      string
	FileSize      int64
	IsMultiFile   bool
	FileCount     int
	CreatedAt     time.Time
	ExpireAt      time.Time
	MaxDownloads  int
	DownloadCount int
	mu            sync.Mutex
}

var (
	fileStore      sync.Map
	tempDir        string
	expireHours    int
	maxDownloads   int
	maxUploadMB    int
	maxUploadBytes int64
)

func randomCode() string {
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(90000))
		if err != nil {
			continue
		}
		code := fmt.Sprintf("%05d", n.Int64()+10000)
		if _, exists := fileStore.Load(code); !exists {
			return code
		}
	}
}

func respondText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

func uploadLimitText() string {
	return fmt.Sprintf("文件超过上传限制，最大允许 %dMB", maxUploadMB)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, X-File-Name, X-File-Count, Content-Disposition")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondText(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	if maxUploadBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+multipartOverheadBytes)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondText(w, http.StatusRequestEntityTooLarge, uploadLimitText())
			return
		}
		respondText(w, http.StatusBadRequest, "解析文件失败: "+err.Error())
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	type uploadedFile struct {
		data     io.Reader
		filename string
	}
	var files []uploadedFile
	var uploadSize int64

	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, headers := range r.MultipartForm.File {
			for _, header := range headers {
				uploadSize += header.Size
				if maxUploadBytes > 0 && uploadSize > maxUploadBytes {
					respondText(w, http.StatusRequestEntityTooLarge, uploadLimitText())
					return
				}
				f, err := header.Open()
				if err != nil {
					continue
				}
				files = append(files, uploadedFile{f, header.Filename})
			}
		}
	}
	if len(files) == 0 {
		respondText(w, http.StatusBadRequest, "未收到任何文件")
		return
	}

	code := randomCode()
	var savedPath, downloadName string
	var totalSize int64
	var isMulti bool

	if len(files) == 1 {
		uf := files[0]
		ext := filepath.Ext(uf.filename)
		savedPath = filepath.Join(tempDir, code+ext)
		downloadName = uf.filename
		dst, err := os.Create(savedPath)
		if err != nil {
			respondText(w, http.StatusInternalServerError, "创建文件失败")
			return
		}
		written, err := io.Copy(dst, uf.data)
		dst.Close()
		if closer, ok := uf.data.(io.Closer); ok {
			closer.Close()
		}
		if err != nil {
			os.Remove(savedPath)
			respondText(w, http.StatusInternalServerError, "写入文件失败")
			return
		}
		totalSize = written
	} else {
		isMulti = true
		downloadName = code + ".zip"
		savedPath = filepath.Join(tempDir, downloadName)
		zf, err := os.Create(savedPath)
		if err != nil {
			respondText(w, http.StatusInternalServerError, "创建 ZIP 失败")
			return
		}
		zw := zip.NewWriter(zf)
		for _, uf := range files {
			w, err := zw.Create(uf.filename)
			if err != nil {
				continue
			}
			io.Copy(w, uf.data)
			if closer, ok := uf.data.(io.Closer); ok {
				closer.Close()
			}
		}
		zw.Close()
		zf.Close()
		fi, _ := os.Stat(savedPath)
		totalSize = fi.Size()
	}

	meta := &FileMeta{
		Code:          code,
		FilePath:      savedPath,
		FileName:      downloadName,
		FileSize:      totalSize,
		IsMultiFile:   isMulti,
		FileCount:     len(files),
		CreatedAt:     time.Now(),
		ExpireAt:      time.Now().Add(time.Duration(expireHours) * time.Hour),
		MaxDownloads:  maxDownloads,
		DownloadCount: 0,
	}
	fileStore.Store(code, meta)
	log.Printf("[上传] code=%s name=%s size=%d files=%d zip=%v", code, downloadName, totalSize, len(files), isMulti)
	respondText(w, http.StatusOK, code)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		respondText(w, http.StatusMethodNotAllowed, "仅支持 GET / HEAD")
		return
	}
	code := strings.TrimPrefix(r.URL.Path, "/download/")
	code = strings.TrimSpace(code)
	if code == "" || len(code) != 5 {
		respondText(w, http.StatusBadRequest, "取件码格式错误，应为5位数字")
		return
	}

	val, ok := fileStore.Load(code)
	if !ok {
		respondText(w, http.StatusNotFound, "取件码不存在")
		return
	}
	meta := val.(*FileMeta)

	if time.Now().After(meta.ExpireAt) {
		fileStore.Delete(code)
		os.Remove(meta.FilePath)
		respondText(w, http.StatusGone, "文件已过期")
		return
	}

	if meta.MaxDownloads > 0 {
		meta.mu.Lock()
		if meta.DownloadCount >= meta.MaxDownloads {
			meta.mu.Unlock()
			respondText(w, http.StatusGone, "文件下载次数已用完")
			return
		}
		meta.mu.Unlock()
	}

	f, err := os.Open(meta.FilePath)
	if err != nil {
		respondText(w, http.StatusNotFound, "文件不存在")
		return
	}
	defer f.Close()

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-File-Name", meta.FileName)
	w.Header().Set("X-File-Count", fmt.Sprintf("%d", meta.FileCount))
	w.Header().Set("Content-Type", "application/octet-stream")

	rangeHeader := r.Header.Get("Range")

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.FileSize))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.FileName))
		w.WriteHeader(http.StatusOK)
		return
	}

	if rangeHeader == "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.FileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.FileSize))
		w.WriteHeader(http.StatusOK)

		written, copyErr := io.Copy(w, f)

		if copyErr == nil && written == meta.FileSize {
			if meta.MaxDownloads > 0 {
				meta.mu.Lock()
				meta.DownloadCount++
				dc := meta.DownloadCount
				meta.mu.Unlock()
				log.Printf("[下载] code=%s name=%s count=%d/%d", code, meta.FileName, dc, meta.MaxDownloads)
				if dc >= meta.MaxDownloads {
					go func() {
						time.Sleep(2 * time.Second)
						fileStore.Delete(code)
						os.Remove(meta.FilePath)
						log.Printf("[销毁] code=%s name=%s", code, meta.FileName)
					}()
				}
			} else {
				log.Printf("[下载] code=%s name=%s (无限次)", code, meta.FileName)
			}
		} else {
			log.Printf("[中断] code=%s name=%s 传输未完成: %v", code, meta.FileName, copyErr)
		}
		return
	}

	// Range
	var start, end int64
	_, err = fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
	if err != nil {
		_, err2 := fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
		if err2 != nil {
			respondText(w, http.StatusRequestedRangeNotSatisfiable, "Range 格式错误")
			return
		}
		end = meta.FileSize - 1
	}
	if start < 0 || start >= meta.FileSize || end < start || end >= meta.FileSize {
		respondText(w, http.StatusRequestedRangeNotSatisfiable, "Range 超出范围")
		return
	}
	cl := end - start + 1
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, meta.FileSize))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", cl))
	w.WriteHeader(http.StatusPartialContent)
	f.Seek(start, io.SeekStart)
	io.CopyN(w, f, cl)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	respondText(w, http.StatusOK, "ok")
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(fmt.Sprintf(`{"version":"%s","codeLen":5,"expireHours":%d,"maxDownloads":%d,"maxUploadMB":%d,"maxUploadBytes":%d}`, VERSION, expireHours, maxDownloads, maxUploadMB, maxUploadBytes)))
}

func startCleaner() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			fileStore.Range(func(key, value interface{}) bool {
				meta := value.(*FileMeta)
				if now.After(meta.ExpireAt) {
					fileStore.Delete(key)
					os.Remove(meta.FilePath)
					log.Printf("[清理] code=%s name=%s (过期)", meta.Code, meta.FileName)
				}
				return true
			})
		}
	}()
}

func cleanupOnStart() {
	entries, _ := os.ReadDir(tempDir)
	for _, entry := range entries {
		os.Remove(filepath.Join(tempDir, entry.Name()))
	}
	log.Printf("[启动] 已清空临时目录: %s", tempDir)
}

func main() {
	port := flag.Int("port", 8080, "HTTP 监听端口")
	dir := flag.String("dir", "./temp_files", "临时文件目录")
	exp := flag.Int("expire", 24, "文件过期时间（小时）")
	maxDl := flag.Int("max-dl", 0, "最大下载次数（0=不限）")
	maxUp := flag.Int("max-upload-mb", DEFAULT_MAX_UPLOAD_MB, "max upload size in MB (0=unlimited)")
	flag.Parse()

	tempDir = *dir
	expireHours = *exp
	maxDownloads = *maxDl
	maxUploadMB = *maxUp
	if maxUploadMB > 0 {
		maxUploadBytes = int64(maxUploadMB) * 1024 * 1024
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Fatalf("创建临时目录失败: %v", err)
	}
	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("service start failed: %v", err)
	}

	cleanupOnStart()
	startCleaner()

	http.HandleFunc("/upload", corsMiddleware(handleUpload))
	http.HandleFunc("/download/", corsMiddleware(handleDownload))
	http.HandleFunc("/health", corsMiddleware(handleHealth))
	http.HandleFunc("/version", corsMiddleware(handleVersion))

	log.Printf("  max upload size: %d MB (0=unlimited)", maxUploadMB)
	log.Printf("============================================")
	log.Printf("  relay-server v%s", VERSION)
	log.Printf("  监听端口: %d", *port)
	log.Printf("  取件码: 5位数字")
	log.Printf("  过期时间: %d 小时", expireHours)
	log.Printf("  最大下载次数: %d (0=不限)", maxDownloads)
	log.Printf("============================================")

	if err := http.Serve(ln, nil); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
