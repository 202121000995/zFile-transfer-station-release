# 文件中转站部署说明

## 服务端形态

服务端本身只提供标准 HTTP 服务，形式为：

```text
http://服务器IP:端口
```

默认端口为 `8080`。如果需要域名和 SSL 证书，推荐使用：

- 1Panel 反向代理
- 宝塔反向代理
- Nginx
- Caddy

反代目标：

```text
http://127.0.0.1:8080
```

## Linux 一键部署

一键部署：

```bash
curl -fsSL https://raw.githubusercontent.com/202121000995/zFile-transfer-station/main/server/install-linux.sh | sudo bash -s -- --port 8080 --expire 24 --max-dl 0 --max-upload-mb 500
```

自定义示例：

```bash
curl -fsSL https://raw.githubusercontent.com/202121000995/zFile-transfer-station/main/server/install-linux.sh | sudo bash -s -- \
  --port 9000 \
  --expire 12 \
  --max-dl 1 \
  --max-upload-mb 500 \
  --dir /opt/zfile-relay/temp_files \
  --web-root /var/www/zfile-transfer
```

## 快捷命令

部署后会生成：

```bash
zfile-relay start
zfile-relay stop
zfile-relay restart
zfile-relay status
zfile-relay logs
zfile-relay config
zfile-relay set --port 8080 --expire 24 --max-dl 0 --max-upload-mb 500 --dir /opt/zfile-relay/temp_files
```

参数含义：

- `--port`：服务监听端口，例如 `8080`
- `--expire`：文件过期时间，单位小时
- `--max-dl`：最大下载次数，`0` 表示不限
- `--dir`：临时文件保存目录

## 客户端服务端地址

客户端不再强制写死服务器地址。

在客户端右下角点击：

```text
设置中心
```

可设置服务端地址，例如：

```text
http://121.196.195.45:8080
https://zz.31pk.top
```

如果用户只输入：

```text
121.196.195.45:8080
```

客户端会自动补成：

```text
http://121.196.195.45:8080
```
