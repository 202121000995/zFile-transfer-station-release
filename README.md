# zFile transfer station

跨平台文件中转站：服务端中转 + 网页端 + Windows 客户端。

## 结构

- `server/`：Go 服务端，负责上传、取件码、下载、Range 分片、24 小时过期。
- `web/`：网页端，适合 Mac/手机/浏览器使用。
- `client/tauri/`：Rust + Tauri Windows 图形客户端。
- `client/qt/`：Qt Widgets Win7 专版客户端，离线、无 WebView2 依赖。
- `client/cli/`：Go 命令行客户端。
- `server/install-linux.sh`：公开版 Linux 一键部署脚本，不包含私人服务器信息。
- `docs/deployment.md`：部署、快捷命令、客户端服务端地址配置说明。

## 当前服务端

- 地址：`https://zz.31pk.top`
- 取件码：5 位数字
- 文件过期：24 小时
- 下载次数：不限
- 服务本身支持 `ip:端口` 形式，例如 `http://服务器IP:8080`
- 如需 SSL，推荐用 1Panel/宝塔/Nginx/Caddy 反向代理到服务端口。

## 服务端部署

```bash
curl -fsSL https://raw.githubusercontent.com/202121000995/zFile-transfer-station/main/server/install-linux.sh | sudo bash -s -- --port 8080 --expire 24 --max-dl 0 --max-upload-mb 500
```

部署后会生成快捷命令：

```bash
zfile-relay start
zfile-relay stop
zfile-relay restart
zfile-relay status
zfile-relay logs
zfile-relay config
zfile-relay set --port 8080 --expire 24 --max-dl 0 --max-upload-mb 500
```

## 客户端设置

图形客户端右下角 `设置中心` 可设置服务端地址，例如：

```text
http://服务器IP:8080
https://你的域名
```

## Tauri 客户端功能

- 真实上传进度
- 原生下载到系统 `Downloads`
- 下载进度显示
- 下载完成后自动打开文件夹
- `.zip` 自动解压
- 开机自动启动

## 构建提示

Tauri 客户端当前使用 Rust 1.75 + Tauri v1 `windows7-compat` 路线，以尽量兼容 Windows 7。

构建前需要：

- Node.js
- Rust 1.75 GNU 工具链
- MinGW / w64devkit
- WebView2 Runtime

Windows 客户端源码目录：

```powershell
cd client\tauri
npm install
npm run tauri -- build
```

最终产物通常在：

```text
client\tauri\src-tauri\target\release\bundle\nsis\
```
