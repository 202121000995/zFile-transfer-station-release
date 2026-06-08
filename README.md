# zFile Transfer Station Release

这个仓库只存放公开一键部署文件：

- `server/install-linux.sh`
- `server/bin/relay-server-linux-amd64`
- `web/index.html`

主项目源码仓库保持私有。

## 一键部署

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
