package main

import (
	"archive/zip"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const VERSION = "2.0.0"
const DEFAULT_MAX_UPLOAD_MB = 500
const multipartOverheadBytes int64 = 1 << 20

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>中转站后台</title>
<style>
body{margin:0;background:#f3f5f8;color:#1e293b;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Arial,sans-serif}
.wrap{width:min(860px,calc(100% - 28px));margin:32px auto}
h1{font-size:24px;margin:0 0 18px}
.card{background:#fff;border:1px solid #e2e8f0;border-radius:10px;padding:20px;margin-bottom:16px}
.row{display:grid;grid-template-columns:150px 1fr;gap:12px;align-items:center;margin:12px 0}
label{color:#64748b;font-size:14px}
input,select{width:100%;box-sizing:border-box;border:1px solid #dbe3ee;border-radius:8px;padding:10px 12px;font-size:14px}
button{border:0;border-radius:8px;background:#4f6ef7;color:#fff;font-weight:700;padding:10px 16px;cursor:pointer}
button.secondary{background:#eef2ff;color:#3151d3}
.actions{display:flex;gap:10px;justify-content:flex-end;margin-top:16px}
.msg{min-height:22px;color:#64748b;font-size:13px}
.err{color:#ef4444}.ok{color:#16a34a}
.hide{display:none}
@media(max-width:640px){.row{grid-template-columns:1fr}.actions{justify-content:stretch;flex-direction:column}}
</style>
</head>
<body>
<div class="wrap">
  <h1>文件中转站后台</h1>
  <div class="card" id="loginCard">
    <h2>登录</h2>
    <div class="row"><label>后台密码</label><input id="loginPassword" type="password" autocomplete="current-password" placeholder="默认 password123"></div>
    <div class="actions"><button onclick="login()">登录</button></div>
    <div class="msg" id="loginMsg"></div>
  </div>
  <div id="panel" class="hide">
    <div class="card">
      <h2>密码修改</h2>
      <div class="row"><label>旧密码</label><input id="oldPassword" type="password"></div>
      <div class="row"><label>新密码</label><input id="newPassword" type="password" placeholder="至少 6 位"></div>
      <div class="actions"><button onclick="changePassword()">保存密码</button></div>
      <div class="msg" id="pwdMsg"></div>
    </div>
    <div class="card">
      <h2>存储挂载</h2>
      <div class="row"><label>存储类型</label><select id="storageType" onchange="toggleMinio()"><option value="local">本地存储</option><option value="minio">MinIO</option></select></div>
      <div id="minioFields">
        <div class="row"><label>Endpoint</label><input id="endpoint" placeholder="127.0.0.1:9000 或 minio.example.com"></div>
        <div class="row"><label>Access Key</label><input id="accessKey"></div>
        <div class="row"><label>Secret Key</label><input id="secretKey" type="password" placeholder="留空表示不修改"></div>
        <div class="row"><label>Bucket</label><input id="bucket"></div>
        <div class="row"><label>Region</label><input id="region" placeholder="默认 us-east-1"></div>
        <div class="row"><label>对象前缀</label><input id="prefix" placeholder="例如 zfile"></div>
        <div class="row"><label>使用 HTTPS</label><select id="useSSL"><option value="false">否</option><option value="true">是</option></select></div>
      </div>
      <div class="actions"><button class="secondary" onclick="testStorage()">测试连接</button><button onclick="saveStorage()">保存存储</button></div>
      <div class="msg" id="storageMsg"></div>
    </div>
  </div>
</div>
<script>
async function api(url, data, method='POST') {
  const res = await fetch(url, {method, credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:data?JSON.stringify(data):undefined});
  const json = await res.json().catch(()=>({ok:false,error:'响应解析失败'}));
  if (!res.ok || json.ok === false) throw new Error(json.error || '请求失败');
  return json;
}
function msg(id, text, ok){const el=document.getElementById(id);el.textContent=text;el.className='msg '+(ok?'ok':'err')}
async function login(){try{await api('/admin/api/login',{password:loginPassword.value});loginCard.classList.add('hide');panel.classList.remove('hide');await loadConfig()}catch(e){msg('loginMsg',e.message,false)}}
async function loadConfig(){const r=await api('/admin/api/config',null,'GET');const c=r.config||{};const s=c.storage||{};const m=s.minio||{};storageType.value=s.type||'local';endpoint.value=m.endpoint||'';accessKey.value=m.accessKey||'';secretKey.value='';bucket.value=m.bucket||'';region.value=m.region||'';prefix.value=m.prefix||'';useSSL.value=String(!!m.useSSL);toggleMinio()}
function toggleMinio(){minioFields.style.display=storageType.value==='minio'?'block':'none'}
async function changePassword(){try{await api('/admin/api/password',{oldPassword:oldPassword.value,newPassword:newPassword.value});oldPassword.value='';newPassword.value='';msg('pwdMsg','密码已修改',true)}catch(e){msg('pwdMsg',e.message,false)}}
function storagePayload(){return {storage:{type:storageType.value,minio:{endpoint:endpoint.value.trim(),accessKey:accessKey.value.trim(),secretKey:secretKey.value,bucket:bucket.value.trim(),region:region.value.trim(),prefix:prefix.value.trim(),useSSL:useSSL.value==='true'}}}}
async function saveStorage(){try{await api('/admin/api/config',storagePayload());msg('storageMsg','存储配置已保存',true);secretKey.value=''}catch(e){msg('storageMsg',e.message,false)}}
async function testStorage(){try{await saveStorage();const r=await api('/admin/api/storage-test',{});msg('storageMsg',r.message||'测试通过',true)}catch(e){msg('storageMsg',e.message,false)}}
</script>
</body>
</html>`

type FileMeta struct {
	Code          string
	FilePath      string
	StorageType   string
	ObjectKey     string
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

type MinioConfig struct {
	Endpoint  string `json:"endpoint"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	UseSSL    bool   `json:"useSSL"`
	Prefix    string `json:"prefix"`
}

type StorageConfig struct {
	Type  string      `json:"type"`
	Minio MinioConfig `json:"minio"`
}

type AppConfig struct {
	AdminPasswordHash string        `json:"adminPasswordHash"`
	Storage           StorageConfig `json:"storage"`
}

var (
	fileStore      sync.Map
	tempDir        string
	webRoot        string
	configPath     string
	metadataPath   string
	expireHours    int
	maxDownloads   int
	maxUploadMB    int
	maxUploadBytes int64
	appConfig      AppConfig
	configMu       sync.RWMutex
	metadataMu     sync.Mutex
	adminSession   string
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

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashPassword(password string) string {
	salt := randomToken(16)
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return salt + ":" + hex.EncodeToString(sum[:])
}

func verifyPassword(hashValue, password string) bool {
	parts := strings.Split(hashValue, ":")
	if len(parts) != 2 {
		return false
	}
	sum := sha256.Sum256([]byte(parts[0] + ":" + password))
	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(sum[:], expected) == 1
}

func defaultConfig() AppConfig {
	return AppConfig{
		AdminPasswordHash: hashPassword("password123"),
		Storage: StorageConfig{
			Type: "local",
		},
	}
}

func loadConfig() error {
	configMu.Lock()
	defer configMu.Unlock()

	appConfig = defaultConfig()
	if configPath == "" {
		return nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return saveConfigLocked()
		}
		return err
	}
	if err := json.Unmarshal(data, &appConfig); err != nil {
		return err
	}
	if appConfig.Storage.Type == "" {
		appConfig.Storage.Type = "local"
	}
	if appConfig.AdminPasswordHash == "" {
		appConfig.AdminPasswordHash = hashPassword("password123")
		return saveConfigLocked()
	}
	return nil
}

func saveConfigLocked() error {
	if configPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0600)
}

func currentConfig() AppConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return appConfig
}

func loadMetadata() {
	if metadataPath == "" {
		return
	}
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return
	}
	var records []*FileMeta
	if err := json.Unmarshal(data, &records); err != nil {
		log.Printf("metadata load failed: %v", err)
		return
	}
	now := time.Now()
	for _, meta := range records {
		if meta == nil || meta.Code == "" || now.After(meta.ExpireAt) {
			continue
		}
		fileStore.Store(meta.Code, meta)
	}
}

func saveMetadata() {
	if metadataPath == "" {
		return
	}
	metadataMu.Lock()
	defer metadataMu.Unlock()
	var records []*FileMeta
	fileStore.Range(func(_, value interface{}) bool {
		meta := value.(*FileMeta)
		clone := *meta
		records = append(records, &clone)
		return true
	})
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0755); err != nil {
		log.Printf("metadata mkdir failed: %v", err)
		return
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		log.Printf("metadata marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(metadataPath, data, 0600); err != nil {
		log.Printf("metadata save failed: %v", err)
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

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func minioObjectURL(mc MinioConfig, objectKey string) (string, error) {
	scheme := "http"
	if mc.UseSSL {
		scheme = "https"
	}
	endpoint := strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(mc.Endpoint, "https://"), "http://"), "/")
	if endpoint == "" || mc.Bucket == "" {
		return "", errors.New("MinIO endpoint and bucket are required")
	}
	segments := []string{url.PathEscape(mc.Bucket)}
	for _, part := range strings.Split(objectKey, "/") {
		if part != "" {
			segments = append(segments, url.PathEscape(part))
		}
	}
	return scheme + "://" + endpoint + "/" + strings.Join(segments, "/"), nil
}

func minioRegion(mc MinioConfig) string {
	if mc.Region != "" {
		return mc.Region
	}
	return "us-east-1"
}

func signMinioRequest(req *http.Request, mc MinioConfig, payloadHash string, t time.Time) {
	amzDate := t.UTC().Format("20060102T150405Z")
	dateStamp := t.UTC().Format("20060102")
	region := minioRegion(mc)

	req.Header.Set("Host", req.Host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)

	canonicalHeaders := "host:" + req.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := req.Method + "\n" +
		req.URL.EscapedPath() + "\n" +
		req.URL.RawQuery + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		payloadHash

	credentialScope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" +
		amzDate + "\n" +
		credentialScope + "\n" +
		sha256Hex([]byte(canonicalRequest))

	kDate := hmacSHA256([]byte("AWS4"+mc.SecretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+mc.AccessKey+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func minioPutObject(mc MinioConfig, objectKey string, body io.Reader, size int64) error {
	target, err := minioObjectURL(mc, objectKey)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, target, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	signMinioRequest(req, mc, "UNSIGNED-PAYLOAD", time.Now())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("minio put failed: %s %s", resp.Status, string(msg))
	}
	return nil
}

func minioGetObject(mc MinioConfig, objectKey string, rangeHeader string) (*http.Response, error) {
	target, err := minioObjectURL(mc, objectKey)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	signMinioRequest(req, mc, "UNSIGNED-PAYLOAD", time.Now())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("minio get failed: %s %s", resp.Status, string(msg))
	}
	return resp, nil
}

func minioDeleteObject(mc MinioConfig, objectKey string) error {
	target, err := minioObjectURL(mc, objectKey)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	signMinioRequest(req, mc, "UNSIGNED-PAYLOAD", time.Now())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("minio delete failed: %s %s", resp.Status, string(msg))
	}
	return nil
}

func storageObjectKey(code, fileName string) string {
	cfg := currentConfig()
	prefix := strings.Trim(cfg.Storage.Minio.Prefix, "/")
	name := code + "/" + filepath.Base(fileName)
	if prefix != "" {
		return prefix + "/" + name
	}
	return name
}

func removeStoredFile(meta *FileMeta) {
	if meta == nil {
		return
	}
	if meta.StorageType == "minio" && meta.ObjectKey != "" {
		cfg := currentConfig()
		if err := minioDeleteObject(cfg.Storage.Minio, meta.ObjectKey); err != nil {
			log.Printf("[minio delete failed] code=%s key=%s err=%v", meta.Code, meta.ObjectKey, err)
		}
		return
	}
	if meta.FilePath != "" {
		if err := os.Remove(meta.FilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("[local delete failed] code=%s path=%s err=%v", meta.Code, meta.FilePath, err)
		}
	}
}

func isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie("zfile_admin")
	if err != nil || cookie.Value == "" {
		return false
	}
	configMu.RLock()
	token := adminSession
	configMu.RUnlock()
	return token != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if isAdmin(r) {
		return true
	}
	respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "未登录"})
	return false
}

func respondJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func publicConfig(cfg AppConfig) AppConfig {
	cfg.AdminPasswordHash = ""
	cfg.Storage.Minio.SecretKey = ""
	return cfg
}

func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, adminHTML)
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "仅支持 POST"})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	cfg := currentConfig()
	if !verifyPassword(cfg.AdminPasswordHash, req.Password) {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "密码错误"})
		return
	}
	token := randomToken(32)
	configMu.Lock()
	adminSession = token
	configMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "zfile_admin",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	configMu.Lock()
	adminSession = ""
	configMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "zfile_admin", Value: "", Path: "/", MaxAge: -1})
	respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "config": publicConfig(currentConfig())})
	case http.MethodPost:
		var req struct {
			Storage StorageConfig `json:"storage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		if req.Storage.Type != "minio" {
			req.Storage.Type = "local"
		}
		configMu.Lock()
		if req.Storage.Type == "minio" && req.Storage.Minio.SecretKey == "" {
			req.Storage.Minio.SecretKey = appConfig.Storage.Minio.SecretKey
		}
		appConfig.Storage = req.Storage
		err := saveConfigLocked()
		configMu.Unlock()
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		respondJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "方法不支持"})
	}
}

func handleAdminPassword(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "仅支持 POST"})
		return
	}
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if len(req.NewPassword) < 6 {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "新密码至少 6 位"})
		return
	}
	configMu.Lock()
	if !verifyPassword(appConfig.AdminPasswordHash, req.OldPassword) {
		configMu.Unlock()
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "旧密码错误"})
		return
	}
	appConfig.AdminPasswordHash = hashPassword(req.NewPassword)
	err := saveConfigLocked()
	configMu.Unlock()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func handleAdminStorageTest(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "仅支持 POST"})
		return
	}
	cfg := currentConfig()
	if cfg.Storage.Type != "minio" {
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "当前为本地存储"})
		return
	}
	key := storageObjectKey("admin-test", "probe.txt")
	if err := minioPutObject(cfg.Storage.Minio, key, bytes.NewReader([]byte("ok")), 2); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	minioDeleteObject(cfg.Storage.Minio, key)
	respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "MinIO 连接正常"})
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

type uploadedFile struct {
	data     io.Reader
	filename string
	size     int64
}

func saveUploadedFiles(code string, files []uploadedFile) (*FileMeta, error) {
	cfg := currentConfig()
	var savedPath, downloadName, objectKey string
	var totalSize int64
	var isMulti bool

	if len(files) == 1 {
		uf := files[0]
		downloadName = uf.filename
		if cfg.Storage.Type == "minio" {
			objectKey = storageObjectKey(code, downloadName)
			if err := minioPutObject(cfg.Storage.Minio, objectKey, uf.data, uf.size); err != nil {
				return nil, err
			}
			totalSize = uf.size
		} else {
			ext := filepath.Ext(uf.filename)
			savedPath = filepath.Join(tempDir, code+ext)
			dst, err := os.Create(savedPath)
			if err != nil {
				return nil, err
			}
			written, err := io.Copy(dst, uf.data)
			dst.Close()
			if err != nil {
				os.Remove(savedPath)
				return nil, err
			}
			totalSize = written
		}
	} else {
		isMulti = true
		downloadName = code + ".zip"
		tmpZip := filepath.Join(tempDir, downloadName)
		zf, err := os.Create(tmpZip)
		if err != nil {
			return nil, err
		}
		zw := zip.NewWriter(zf)
		for _, uf := range files {
			w, err := zw.Create(uf.filename)
			if err != nil {
				continue
			}
			io.Copy(w, uf.data)
		}
		zw.Close()
		zf.Close()
		fi, _ := os.Stat(tmpZip)
		totalSize = fi.Size()

		if cfg.Storage.Type == "minio" {
			f, err := os.Open(tmpZip)
			if err != nil {
				os.Remove(tmpZip)
				return nil, err
			}
			objectKey = storageObjectKey(code, downloadName)
			err = minioPutObject(cfg.Storage.Minio, objectKey, f, totalSize)
			f.Close()
			os.Remove(tmpZip)
			if err != nil {
				return nil, err
			}
		} else {
			savedPath = tmpZip
		}
	}

	storageType := "local"
	if cfg.Storage.Type == "minio" {
		storageType = "minio"
	}
	return &FileMeta{
		Code:          code,
		FilePath:      savedPath,
		StorageType:   storageType,
		ObjectKey:     objectKey,
		FileName:      downloadName,
		FileSize:      totalSize,
		IsMultiFile:   isMulti,
		FileCount:     len(files),
		CreatedAt:     time.Now(),
		ExpireAt:      time.Now().Add(time.Duration(expireHours) * time.Hour),
		MaxDownloads:  maxDownloads,
		DownloadCount: 0,
	}, nil
}

func handleUploadLegacy(w http.ResponseWriter, r *http.Request) {
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
				files = append(files, uploadedFile{data: f, filename: header.Filename, size: header.Size})
			}
		}
	}
	if len(files) == 0 {
		respondText(w, http.StatusBadRequest, "未收到任何文件")
		return
	}

	code := randomCode()
	meta, err := saveUploadedFiles(code, files)
	for _, uf := range files {
		if closer, ok := uf.data.(io.Closer); ok {
			closer.Close()
		}
	}
	if err != nil {
		respondText(w, http.StatusInternalServerError, "保存文件失败: "+err.Error())
		return
	}
	fileStore.Store(code, meta)
	saveMetadata()
	log.Printf("[上传] code=%s name=%s size=%d files=%d storage=%s", code, meta.FileName, meta.FileSize, len(files), meta.StorageType)
	respondText(w, http.StatusOK, code)
}

func serveMinioDownload(w http.ResponseWriter, r *http.Request, code string, meta *FileMeta) {
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-File-Name", meta.FileName)
	w.Header().Set("X-File-Count", fmt.Sprintf("%d", meta.FileCount))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.FileName))
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.FileSize))
		w.WriteHeader(http.StatusOK)
		return
	}
	cfg := currentConfig()
	resp, err := minioGetObject(cfg.Storage.Minio, meta.ObjectKey, r.Header.Get("Range"))
	if err != nil {
		respondText(w, http.StatusNotFound, "文件不存在")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPartialContent {
		w.Header().Set("Content-Range", resp.Header.Get("Content-Range"))
		w.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.FileSize))
		w.WriteHeader(http.StatusOK)
	}
	written, copyErr := io.Copy(w, resp.Body)
	if copyErr == nil && (resp.StatusCode == http.StatusPartialContent || meta.FileSize <= 0 || written == meta.FileSize) {
		if meta.MaxDownloads > 0 {
			meta.mu.Lock()
			meta.DownloadCount++
			dc := meta.DownloadCount
			meta.mu.Unlock()
			saveMetadata()
			if dc >= meta.MaxDownloads {
				go func() {
					time.Sleep(2 * time.Second)
					fileStore.Delete(code)
					removeStoredFile(meta)
					saveMetadata()
				}()
			}
		}
	}
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
		removeStoredFile(meta)
		saveMetadata()
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

	if meta.StorageType == "minio" {
		serveMinioDownload(w, r, code, meta)
		return
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
				saveMetadata()
				log.Printf("[下载] code=%s name=%s count=%d/%d", code, meta.FileName, dc, meta.MaxDownloads)
				if dc >= meta.MaxDownloads {
					go func() {
						time.Sleep(2 * time.Second)
						fileStore.Delete(code)
						removeStoredFile(meta)
						saveMetadata()
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

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		respondText(w, http.StatusMethodNotAllowed, "仅支持 GET / HEAD")
		return
	}
	if webRoot == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
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
					removeStoredFile(meta)
					saveMetadata()
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
	web := flag.String("web", "", "web index.html directory (optional)")
	cfg := flag.String("config", "/opt/zfile-relay/config.json", "config file path")
	metaFile := flag.String("metadata", "/opt/zfile-relay/metadata.json", "metadata file path")
	exp := flag.Int("expire", 24, "文件过期时间（小时）")
	maxDl := flag.Int("max-dl", 0, "最大下载次数（0=不限）")
	maxUp := flag.Int("max-upload-mb", DEFAULT_MAX_UPLOAD_MB, "max upload size in MB (0=unlimited)")
	flag.Parse()

	tempDir = *dir
	webRoot = *web
	configPath = *cfg
	metadataPath = *metaFile
	expireHours = *exp
	maxDownloads = *maxDl
	maxUploadMB = *maxUp
	if maxUploadMB > 0 {
		maxUploadBytes = int64(maxUploadMB) * 1024 * 1024
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Fatalf("创建临时目录失败: %v", err)
	}
	if err := loadConfig(); err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	loadMetadata()
	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("service start failed: %v", err)
	}

	startCleaner()

	http.HandleFunc("/upload", corsMiddleware(handleUpload))
	http.HandleFunc("/download/", corsMiddleware(handleDownload))
	http.HandleFunc("/health", corsMiddleware(handleHealth))
	http.HandleFunc("/version", corsMiddleware(handleVersion))
	http.HandleFunc("/admin", handleAdminPage)
	http.HandleFunc("/admin/", handleAdminPage)
	http.HandleFunc("/admin/api/login", handleAdminLogin)
	http.HandleFunc("/admin/api/logout", handleAdminLogout)
	http.HandleFunc("/admin/api/config", handleAdminConfig)
	http.HandleFunc("/admin/api/password", handleAdminPassword)
	http.HandleFunc("/admin/api/storage-test", handleAdminStorageTest)
	http.HandleFunc("/", handleIndex)

	log.Printf("  max upload size: %d MB (0=unlimited)", maxUploadMB)
	log.Printf("============================================")
	log.Printf("  relay-server v%s", VERSION)
	log.Printf("  监听端口: %d", *port)
	if webRoot != "" {
		log.Printf("  web目录: %s", webRoot)
	}
	log.Printf("  取件码: 5位数字")
	log.Printf("  过期时间: %d 小时", expireHours)
	log.Printf("  最大下载次数: %d (0=不限)", maxDownloads)
	log.Printf("============================================")

	if err := http.Serve(ln, nil); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
