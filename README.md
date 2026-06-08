# 文件中转站

文件中转站是一套轻量级公网文件临时中转工具，适合在不同电脑、手机、虚拟机之间快速发送和接收文件。

它采用“服务端中转站 + 网页端 + Windows 客户端”的结构：文件上传到自己的服务器后生成 5 位取件码，接收方输入取件码即可下载。文件不会长期保存，可按部署参数设置自动过期时间和下载次数。

## 核心特点

- 服务端支持 Linux VPS 一键部署。
- 网页端无需安装，浏览器打开即可上传和下载。
- Windows Qt 客户端支持 Windows 7 / Windows 10 / Windows 11。
- Qt 客户端不依赖 WebView2 和 .NET，适合老电脑与离线环境。
- 支持 5 位数字取件码、文件自动过期、HTTP Range 下载。
- 默认使用 HTTP + 端口访问；需要 HTTPS 时可用 1Panel、宝塔、Nginx、Caddy 反向代理。

## 一键部署服务端

```bash
curl -fsSL https://raw.githubusercontent.com/202121000995/zFile-transfer-station-release/main/server/install-linux.sh | sudo bash -s -- --port 8080 --expire 24 --max-dl 0
```

## 常用命令

```bash
zfile-relay start
zfile-relay stop
zfile-relay restart
zfile-relay status
zfile-relay logs
zfile-relay config
zfile-relay set --port 8080 --expire 24 --max-dl 0
```

## 客户端使用

首次打开 Windows Qt 客户端时，需要填写服务端地址，例如：

```text
http://服务器IP:8080
https://你的域名
```

点击“测试连接”，返回正常后保存生效。之后客户端会读取本地配置，不再需要每次重复填写。

## 文件说明

- `server/install-linux.sh`：Linux 一键部署脚本。
- `server/bin/relay-server-linux-amd64`：Linux 服务端二进制文件。
- `web/index.html`：网页端文件。
- Releases：Windows 离线客户端发布包。
