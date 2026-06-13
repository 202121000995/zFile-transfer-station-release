package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 配置 & 工具
// ============================================================

var serverURL string

const maxUploadBytes int64 = 500 * 1024 * 1024
const maxUploadLabel = "500MB"

func loadConfig() string {
	data, err := os.ReadFile("config.txt")
	if err == nil {
		url := strings.TrimSpace(string(data))
		if url != "" {
			return url
		}
	}
	return "http://localhost:8080"
}

func saveConfig(url string) {
	os.WriteFile("config.txt", []byte(url), 0644)
}

// ============================================================
// 上传
// ============================================================

func uploadFile(filePath string) {
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("❌ 打开文件失败: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		fmt.Printf("❌ 获取文件信息失败: %v\n", err)
		os.Exit(1)
	}
	fileSize := fi.Size()
	fileName := filepath.Base(filePath)
	if fileSize > maxUploadBytes {
		fmt.Printf("❌ 文件超过上传限制，最大允许 %s\n", maxUploadLabel)
		os.Exit(1)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		fmt.Printf("❌ 构造请求失败: %v\n", err)
		os.Exit(1)
	}
	io.Copy(part, f)
	writer.Close()

	req, err := http.NewRequest("POST", serverURL+"/upload", body)
	if err != nil {
		fmt.Printf("❌ 创建请求失败: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 0}
	fmt.Printf("📤 上传中: %s (%.2f MB)...\n", fileName, float64(fileSize)/(1<<20))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ 上传失败: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 上传失败 (%d): %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	code, _ := io.ReadAll(resp.Body)
	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("  ✅ 上传成功！取件码: %s\n", string(code))
	fmt.Println("  请将此取件码发送给接收方")
	fmt.Println("========================================")
}

// ============================================================
// 下载（多线程 + 断点续传）
// ============================================================

type chunkTask struct {
	index     int
	code      string
	startByte int64
	endByte   int64
	fileName  string
	outDir    string
}

func downloadFile(code string, threads int, outDir string) {
	fmt.Printf("🔍 查询取件码: %s ...\n", code)

	client := &http.Client{Timeout: 30 * time.Second}
	headResp, err := client.Head(serverURL + "/download/" + code)
	if err != nil {
		fmt.Printf("❌ 连接服务器失败: %v\n", err)
		os.Exit(1)
	}
	headResp.Body.Close()

	switch headResp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		fmt.Println("❌ 取件码不存在")
		os.Exit(1)
	case http.StatusGone:
		fmt.Println("❌ 文件已过期或已被下载（阅后即焚）")
		os.Exit(1)
	default:
		fmt.Printf("❌ 服务器错误 (%d)\n", headResp.StatusCode)
		os.Exit(1)
	}

	fileSize, _ := strconv.ParseInt(headResp.Header.Get("Content-Length"), 10, 64)
	fileName := headResp.Header.Get("X-File-Name")
	if fileName == "" {
		fileName = code
	}

	outPath := filepath.Join(outDir, fileName)
	fmt.Printf("📥 文件: %s (%.2f MB)\n", fileName, float64(fileSize)/(1<<20))
	fmt.Printf("🧵 线程数: %d\n\n", threads)

	if fileSize == 0 {
		os.WriteFile(outPath, nil, 0644)
		fmt.Println("✅ 空文件，下载完成")
		return
	}

	chunkSize := fileSize / int64(threads)
	if chunkSize < 1 {
		chunkSize = 1
	}

	tasks := make([]chunkTask, 0, threads)
	for i := 0; i < threads; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == threads-1 {
			end = fileSize - 1
		}
		if start > end {
			continue
		}
		tasks = append(tasks, chunkTask{
			index:     i,
			code:      code,
			startByte: start,
			endByte:   end,
			fileName:  fileName,
			outDir:    outDir,
		})
	}

	var downloadedBytes int64
	var wg sync.WaitGroup
	errCh := make(chan error, len(tasks))

	for _, t := range tasks {
		wg.Add(1)
		go func(task chunkTask) {
			defer wg.Done()
			if err := downloadChunk(task, &downloadedBytes); err != nil {
				errCh <- fmt.Errorf("分片 %d 失败: %v", task.index, err)
			}
		}(t)
	}

	// 进度条
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				dl := atomic.LoadInt64(&downloadedBytes)
				pct := float64(dl) / float64(fileSize) * 100
				if pct > 100 {
					pct = 100
				}
				bar := buildBar(int(pct), 30)
				fmt.Printf("\r  %s %.1f%% (%s / %s)", bar, pct, sizeStr(dl), sizeStr(fileSize))
			case <-done:
				return
			}
		}
	}()

	wg.Wait()
	close(done)
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		fmt.Println()
		for _, e := range errs {
			fmt.Printf("❌ %v\n", e)
		}
		os.Exit(1)
	}

	fmt.Printf("\r  %s 100.0%% (%s / %s)\n", buildBar(100, 30), sizeStr(fileSize), sizeStr(fileSize))

	fmt.Println("🔗 正在合并分片...")
	if err := mergeChunks(tasks, outPath); err != nil {
		fmt.Printf("❌ 合并失败: %v\n", err)
		os.Exit(1)
	}

	for _, t := range tasks {
		os.Remove(filepath.Join(t.outDir, fmt.Sprintf("%s.part.%d", t.fileName, t.index)))
	}
	pattern := filepath.Join(outDir, fileName+".part.*")
	matches, _ := filepath.Glob(pattern)
	for _, m := range matches {
		os.Remove(m)
	}

	fmt.Printf("\n✅ 下载完成: %s\n", outPath)
	runtime.GC()
}

func downloadChunk(t chunkTask, downloaded *int64) error {
	partName := filepath.Join(t.outDir, fmt.Sprintf("%s.part.%d", t.fileName, t.index))

	// 断点续传：检查已有 part 文件
	existingSize := int64(0)
	if info, err := os.Stat(partName); err == nil {
		existingSize = info.Size()
	}

	rangeStart := t.startByte + existingSize
	rangeEnd := t.endByte

	if rangeStart > rangeEnd {
		atomic.AddInt64(downloaded, existingSize)
		return nil
	}

	client := &http.Client{Timeout: 0}
	req, err := http.NewRequest("GET", serverURL+"/download/"+t.code, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	flag := os.O_CREATE | os.O_WRONLY
	if existingSize > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(partName, flag, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			atomic.AddInt64(downloaded, int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

func mergeChunks(tasks []chunkTask, outPath string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	for _, t := range tasks {
		partName := filepath.Join(t.outDir, fmt.Sprintf("%s.part.%d", t.fileName, t.index))
		f, err := os.Open(partName)
		if err != nil {
			return fmt.Errorf("打开分片 %s 失败: %v", partName, err)
		}
		io.CopyBuffer(out, f, buf)
		f.Close()
	}
	return nil
}

// ============================================================
// 工具函数
// ============================================================

func buildBar(pct int, width int) string {
	filled := width * pct / 100
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func sizeStr(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func printUsage() {
	fmt.Println("文件中转站 Windows 客户端")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  transfer.exe upload <文件路径>              上传文件，返回取件码")
	fmt.Println("  transfer.exe download <取件码>               下载文件（默认 8 线程）")
	fmt.Println("  transfer.exe download <取件码> -t 16         指定 16 线程下载")
	fmt.Println("  transfer.exe download <取件码> -o D:\\out     指定输出目录")
	fmt.Println()
	fmt.Println("参数:")
	fmt.Println("  -s <URL>    服务器地址（默认读取 config.txt，否则 http://localhost:8080）")
	fmt.Println("  -t <N>      下载线程数（默认 8）")
	fmt.Println("  -o <DIR>    输出目录（默认当前目录）")
}

// ============================================================
// main
// ============================================================

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	args := os.Args[1:]
	cmd := args[0]

	// 解析 -s 全局参数
	serverURL = loadConfig()
	for i := 0; i < len(args); i++ {
		if args[i] == "-s" && i+1 < len(args) {
			serverURL = args[i+1]
			saveConfig(serverURL)
		}
	}

	// 提取位置参数（去掉 flag 和 flag 值）
	var positional []string
	skipNext := false
	for i, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if a == "-s" || a == "-t" || a == "-o" {
			if i+1 < len(args) {
				skipNext = true
			}
			continue
		}
		positional = append(positional, a)
	}

	switch cmd {
	case "upload":
		if len(positional) < 2 {
			fmt.Println("❌ 用法: transfer.exe upload <文件路径>")
			os.Exit(1)
		}
		uploadFile(positional[1])

	case "download":
		if len(positional) < 2 {
			fmt.Println("❌ 用法: transfer.exe download <取件码>")
			os.Exit(1)
		}
		code := positional[1]

		threads := 8
		outDir := "."
		for i := 0; i < len(args); i++ {
			if args[i] == "-t" && i+1 < len(args) {
				if t, err := strconv.Atoi(args[i+1]); err == nil && t > 0 && t <= 64 {
					threads = t
				}
			}
			if args[i] == "-o" && i+1 < len(args) {
				outDir = args[i+1]
			}
		}

		if err := os.MkdirAll(outDir, 0755); err != nil {
			fmt.Printf("❌ 创建输出目录失败: %v\n", err)
			os.Exit(1)
		}

		downloadFile(code, threads, outDir)

	default:
		fmt.Printf("❌ 未知命令: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}
