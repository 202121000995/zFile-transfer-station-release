#!/usr/bin/env bash
set -euo pipefail

PORT="8080"
DATA_DIR="/opt/zfile-relay/temp_files"
EXPIRE_HOURS="24"
MAX_DOWNLOADS="0"
INSTALL_DIR="/opt/zfile-relay"
WEB_ROOT="/var/www/zfile-transfer"
SERVICE_NAME="zfile-relay"
RELAY_BIN="./relay-server-linux-amd64"
WEB_FILE="./index.html"
REPO_RAW_BASE="https://raw.githubusercontent.com/202121000995/zFile-transfer-station-release/main"
RELAY_URL="${REPO_RAW_BASE}/server/bin/relay-server-linux-amd64"
WEB_URL="${REPO_RAW_BASE}/web/index.html"

usage() {
  cat <<EOF
文件中转站 Linux 一键部署脚本

用法：
  sudo bash install-linux.sh [参数]

参数：
  --port 8080                 服务监听端口，默认 8080
  --dir /opt/.../temp_files   临时文件目录
  --expire 24                 文件过期时间，单位小时
  --max-dl 0                  最大下载次数，0 表示不限
  --install-dir /opt/...      服务端安装目录
  --web-root /var/www/...     网页部署目录
  --relay-bin ./xxx           relay-server Linux 二进制路径
  --web-file ./index.html     网页 index.html 路径
  --relay-url https://...     relay-server 下载地址
  --web-url https://...       网页 index.html 下载地址
  -h, --help                  查看帮助

示例：
  sudo bash install-linux.sh --port 8080 --expire 24 --max-dl 0
  sudo bash install-linux.sh --port 9000 --expire 12 --max-dl 1

说明：
  服务本身只提供 ip:端口，例如 http://服务器IP:8080
  如需 HTTPS/域名证书，建议用 1Panel、宝塔、Nginx、Caddy 等反向代理到 127.0.0.1:端口。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --dir) DATA_DIR="$2"; shift 2 ;;
    --expire) EXPIRE_HOURS="$2"; shift 2 ;;
    --max-dl) MAX_DOWNLOADS="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --web-root) WEB_ROOT="$2"; shift 2 ;;
    --relay-bin) RELAY_BIN="$2"; shift 2 ;;
    --web-file) WEB_FILE="$2"; shift 2 ;;
    --relay-url) RELAY_URL="$2"; shift 2 ;;
    --web-url) WEB_URL="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数：$1"; usage; exit 1 ;;
  esac
done

if [[ "${EUID}" -ne 0 ]]; then
  echo "请使用 root 运行：sudo bash install-linux.sh"
  exit 1
fi

download_file() {
  local url="$1"
  local output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 20 -o "${output}" "${url}"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -O "${output}" "${url}"
    return
  fi
  echo "需要 curl 或 wget 才能下载：${url}"
  exit 1
}

mkdir -p "${INSTALL_DIR}" "${DATA_DIR}" "${WEB_ROOT}"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

if [[ ! -f "${RELAY_BIN}" ]]; then
  echo "未找到本地服务端二进制，正在下载：${RELAY_URL}"
  RELAY_BIN="${WORK_DIR}/relay-server-linux-amd64"
  download_file "${RELAY_URL}" "${RELAY_BIN}"
fi

install -m 755 "${RELAY_BIN}" "${INSTALL_DIR}/relay-server"

if [[ ! -f "${WEB_FILE}" ]]; then
  echo "未找到本地网页文件，正在下载：${WEB_URL}"
  WEB_FILE="${WORK_DIR}/index.html"
  download_file "${WEB_URL}" "${WEB_FILE}"
fi
install -m 644 "${WEB_FILE}" "${WEB_ROOT}/index.html"

cat >"/etc/${SERVICE_NAME}.conf" <<EOF
PORT=${PORT}
DATA_DIR=${DATA_DIR}
EXPIRE_HOURS=${EXPIRE_HOURS}
MAX_DOWNLOADS=${MAX_DOWNLOADS}
EOF

cat >"/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=zFile Transfer Relay Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/${SERVICE_NAME}.conf
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/relay-server -port \${PORT} -dir \${DATA_DIR} -expire \${EXPIRE_HOURS} -max-dl \${MAX_DOWNLOADS}
Restart=always
RestartSec=3
LimitNOFILE=65535
StandardOutput=append:${INSTALL_DIR}/server.log
StandardError=append:${INSTALL_DIR}/server.log

[Install]
WantedBy=multi-user.target
EOF

cat >"/usr/local/bin/${SERVICE_NAME}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

CONF="/etc/zfile-relay.conf"
SERVICE="zfile-relay.service"

usage() {
  cat <<HELP
文件中转站快捷命令

用法：
  zfile-relay start                 启动服务
  zfile-relay stop                  停止服务
  zfile-relay restart               重启服务
  zfile-relay status                查看状态
  zfile-relay logs                  查看日志
  zfile-relay config                查看当前参数
  zfile-relay set --port 8080 --expire 24 --max-dl 0 --dir /opt/zfile-relay/temp_files

参数说明：
  --port      服务端口，例如 8080，对外形式为 http://服务器IP:8080
  --expire    文件有效期，单位小时
  --max-dl    最大下载次数，0 表示不限
  --dir       临时文件存放目录

HTTPS 说明：
  本服务只负责 HTTP 端口。
  如需 SSL，使用 1Panel/宝塔/Nginx/Caddy 反向代理到 127.0.0.1:端口。
HELP
}

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "该操作需要 root 权限"
    exit 1
  fi
}

case "${1:-}" in
  start|stop|restart)
    require_root
    systemctl "$1" "${SERVICE}"
    ;;
  status)
    systemctl status "${SERVICE}" --no-pager -l
    ;;
  logs)
    journalctl -u "${SERVICE}" -f
    ;;
  config)
    cat "${CONF}"
    ;;
  set)
    require_root
    shift
    source "${CONF}"
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --port) PORT="$2"; shift 2 ;;
        --expire) EXPIRE_HOURS="$2"; shift 2 ;;
        --max-dl) MAX_DOWNLOADS="$2"; shift 2 ;;
        --dir) DATA_DIR="$2"; shift 2 ;;
        *) echo "未知参数：$1"; usage; exit 1 ;;
      esac
    done
    cat >"${CONF}" <<CONFIG
PORT=${PORT}
DATA_DIR=${DATA_DIR}
EXPIRE_HOURS=${EXPIRE_HOURS}
MAX_DOWNLOADS=${MAX_DOWNLOADS}
CONFIG
    mkdir -p "${DATA_DIR}"
    systemctl daemon-reload
    systemctl restart "${SERVICE}"
    systemctl status "${SERVICE}" --no-pager -l
    ;;
  -h|--help|help|"")
    usage
    ;;
  *)
    echo "未知命令：$1"
    usage
    exit 1
    ;;
esac
EOF

chmod +x "/usr/local/bin/${SERVICE_NAME}"
systemctl daemon-reload
systemctl enable --now "${SERVICE_NAME}.service"

HOST_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
if [[ -z "${HOST_IP}" ]]; then
  HOST_IP="服务器IP"
fi

cat <<EOF

部署完成。

服务地址：
  http://${HOST_IP}:${PORT}

健康检查：
  curl http://127.0.0.1:${PORT}/health

网页目录：
  ${WEB_ROOT}

快捷命令：
  zfile-relay start
  zfile-relay stop
  zfile-relay restart
  zfile-relay status
  zfile-relay logs
  zfile-relay config
  zfile-relay set --port ${PORT} --expire ${EXPIRE_HOURS} --max-dl ${MAX_DOWNLOADS} --dir ${DATA_DIR}

参数含义：
  --port      服务监听端口
  --expire    文件过期小时数
  --max-dl    最大下载次数，0 表示不限
  --dir       临时文件目录

SSL/域名：
  服务本身是 ip+端口形式。
  如需 HTTPS，请用 1Panel/宝塔/Nginx/Caddy 反向代理到 127.0.0.1:${PORT}。

EOF

