#!/bin/bash
# ZyHive (引巢) — 一键安装脚本
# 用法:
#   curl -sSL https://raw.githubusercontent.com/Zyling-ai/zyhive/main/scripts/install.sh | bash
#   curl -sSL ... | bash -s -- --domain hive.example.com --port 8080
set -e

REPO="Zyling-ai/zyhive"
SERVICE_NAME="zyhive"
BINARY_NAME="zyhive"
PORT=8080
DOMAIN=""

# ── 解析参数 ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)  DOMAIN="$2"; shift 2 ;;
    --port)    PORT="$2";   shift 2 ;;
    *) shift ;;
  esac
done

# ── 颜色输出 ───────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}ℹ${NC}  $*"; }
success() { echo -e "${GREEN}✅${NC} $*"; }
warning() { echo -e "${YELLOW}⚠${NC}  $*"; }
error()   { echo -e "${RED}❌${NC} $*"; exit 1; }

# ── 检测架构 ───────────────────────────────────────────────────────────────
RAW_ARCH=$(uname -m)
case "$RAW_ARCH" in
  x86_64)          ARCH="amd64" ;;
  aarch64|arm64)   ARCH="arm64" ;;
  armv7l|armv6l)   ARCH="arm"   ;;
  *) error "不支持的架构: $RAW_ARCH" ;;
esac

# ── 检测操作系统 ───────────────────────────────────────────────────────────
RAW_OS=$(uname -s)
case "$RAW_OS" in
  Linux)  OS="linux"  ;;
  Darwin) OS="darwin" ;;
  *) error "不支持的操作系统: $RAW_OS" ;;
esac

# ── 检测 root / sudo 权限 ──────────────────────────────────────────────────
HAVE_ROOT=false
if [ "$(id -u)" = "0" ]; then
  HAVE_ROOT=true
elif sudo -n true 2>/dev/null; then
  HAVE_ROOT=true
fi

# ── 确定安装路径 ───────────────────────────────────────────────────────────
if $HAVE_ROOT; then
  INSTALL_BIN="/usr/local/bin/$BINARY_NAME"
  if [ "$OS" = "linux" ]; then
    CONFIG_DIR="/etc/$SERVICE_NAME"
  else
    CONFIG_DIR="/usr/local/etc/$SERVICE_NAME"
  fi
  AGENTS_DIR="/var/lib/$SERVICE_NAME/agents"
  RUN_AS_ROOT=true
else
  warning "无 root 权限，将安装到用户目录"
  INSTALL_BIN="$HOME/.local/bin/$BINARY_NAME"
  CONFIG_DIR="$HOME/.config/$SERVICE_NAME"
  AGENTS_DIR="$HOME/.local/share/$SERVICE_NAME/agents"
  RUN_AS_ROOT=false
fi

CONFIG_FILE="$CONFIG_DIR/$SERVICE_NAME.json"

echo ""
echo -e "${BLUE}🚀 正在安装 ZyHive (引巢 · AI 团队操作系统)…${NC}"
echo ""
info "操作系统：$RAW_OS / $RAW_ARCH → 下载 $OS-$ARCH"
info "安装路径：$INSTALL_BIN"
info "配置目录：$CONFIG_DIR"
[ -n "$DOMAIN" ] && info "域名：$DOMAIN（将自动配置 NGINX + HTTPS）"
echo ""

# ── 获取最新版本号 ─────────────────────────────────────────────────────────
info "查询最新版本…"
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
if [ -z "$LATEST" ]; then
  error "无法获取最新版本，请检查网络连接。"
fi
info "最新版本：$LATEST"

# ── 构造下载 URL ───────────────────────────────────────────────────────────
BINARY_URL="https://github.com/$REPO/releases/download/$LATEST/aipanel-${OS}-${ARCH}"

# ── 创建目录 ───────────────────────────────────────────────────────────────
if $RUN_AS_ROOT; then
  sudo mkdir -p "$(dirname "$INSTALL_BIN")" "$CONFIG_DIR" "$AGENTS_DIR"
else
  mkdir -p "$(dirname "$INSTALL_BIN")" "$CONFIG_DIR" "$AGENTS_DIR"
fi

# ── 下载二进制 ─────────────────────────────────────────────────────────────
info "下载 $BINARY_NAME $LATEST ($OS/$ARCH)…"
TMP_BIN=$(mktemp)
if ! curl -fsSL --progress-bar "$BINARY_URL" -o "$TMP_BIN"; then
  rm -f "$TMP_BIN"
  error "下载失败，URL: $BINARY_URL"
fi

if $RUN_AS_ROOT; then
  sudo install -m 755 "$TMP_BIN" "$INSTALL_BIN"
else
  install -m 755 "$TMP_BIN" "$INSTALL_BIN"
fi
rm -f "$TMP_BIN"
success "二进制已安装至 $INSTALL_BIN"

# ── 生成默认配置（若不存在）────────────────────────────────────────────────
if [ ! -f "$CONFIG_FILE" ]; then
  ADMIN_TOKEN=$(openssl rand -hex 16 2>/dev/null \
    || tr -dc 'a-f0-9' < /dev/urandom | head -c 32)

  # 有域名时 bind 设为 localhost（前面挂 NGINX），否则 lan
  if [ -n "$DOMAIN" ]; then
    BIND_MODE="localhost"
  else
    BIND_MODE="lan"
  fi

  CONFIG_CONTENT="{
  \"gateway\": { \"port\": $PORT, \"bind\": \"$BIND_MODE\" },
  \"agents\":  { \"dir\": \"$AGENTS_DIR\" },
  \"models\":  { \"primary\": \"anthropic/claude-sonnet-4-6\" },
  \"auth\":    { \"mode\": \"token\", \"token\": \"$ADMIN_TOKEN\" }
}"

  if $RUN_AS_ROOT; then
    echo "$CONFIG_CONTENT" | sudo tee "$CONFIG_FILE" > /dev/null
  else
    echo "$CONFIG_CONTENT" > "$CONFIG_FILE"
  fi

  echo ""
  echo -e "${YELLOW}🔑 管理员 Token：${NC}${GREEN}$ADMIN_TOKEN${NC}"
  echo -e "   （已保存至 $CONFIG_FILE，请妥善保存）"
  SHOW_TOKEN="$ADMIN_TOKEN"
fi

# ── Linux: 安装 systemd 服务 ───────────────────────────────────────────────
install_systemd() {
  local SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
  sudo tee "$SERVICE_FILE" > /dev/null << UNIT
[Unit]
Description=ZyHive — AI 团队操作系统
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_BIN --config $CONFIG_FILE
WorkingDirectory=$CONFIG_DIR
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
UNIT

  sudo systemctl daemon-reload
  sudo systemctl enable "$SERVICE_NAME"
  sudo systemctl start  "$SERVICE_NAME"
  success "systemd 服务已启动：$SERVICE_NAME"
  info   "查看状态：sudo systemctl status $SERVICE_NAME"
}

# ── macOS: 安装 launchd 服务 ───────────────────────────────────────────────
install_launchd() {
  local LABEL="com.zyhive.$SERVICE_NAME"
  local LOG_DIR="$HOME/Library/Logs/$SERVICE_NAME"
  mkdir -p "$LOG_DIR"

  if $RUN_AS_ROOT; then
    local PLIST_DIR="/Library/LaunchDaemons"
    local PLIST_FILE="$PLIST_DIR/${LABEL}.plist"
    sudo tee "$PLIST_FILE" > /dev/null << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>              <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$INSTALL_BIN</string>
    <string>--config</string>
    <string>$CONFIG_FILE</string>
  </array>
  <key>WorkingDirectory</key>   <string>$CONFIG_DIR</string>
  <key>RunAtLoad</key>          <true/>
  <key>KeepAlive</key>          <true/>
  <key>StandardOutPath</key>    <string>$LOG_DIR/stdout.log</string>
  <key>StandardErrorPath</key>  <string>$LOG_DIR/stderr.log</string>
</dict>
</plist>
PLIST
    sudo launchctl load -w "$PLIST_FILE"
    success "LaunchDaemon 已加载：$LABEL"
  else
    local PLIST_DIR="$HOME/Library/LaunchAgents"
    local PLIST_FILE="$PLIST_DIR/${LABEL}.plist"
    mkdir -p "$PLIST_DIR"
    cat > "$PLIST_FILE" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>              <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$INSTALL_BIN</string>
    <string>--config</string>
    <string>$CONFIG_FILE</string>
  </array>
  <key>WorkingDirectory</key>   <string>$CONFIG_DIR</string>
  <key>RunAtLoad</key>          <true/>
  <key>KeepAlive</key>          <true/>
  <key>StandardOutPath</key>    <string>$LOG_DIR/stdout.log</string>
  <key>StandardErrorPath</key>  <string>$LOG_DIR/stderr.log</string>
</dict>
</plist>
PLIST
    launchctl load -w "$PLIST_FILE"
    success "LaunchAgent 已加载（用户级）：$LABEL"
  fi
  info "日志目录：$LOG_DIR"
}

# ── 配置 NGINX + HTTPS（需要 --domain 参数且有 root）──────────────────────
install_nginx_https() {
  local domain="$1"

  # ── 安装 nginx + certbot ───────────────────────────────────────────────
  if command -v apt-get &>/dev/null; then
    info "安装 nginx + certbot（apt）…"
    sudo apt-get update -q
    sudo apt-get install -y -q nginx certbot python3-certbot-nginx
    CERTBOT_CMD="certbot --nginx"
  elif command -v yum &>/dev/null; then
    info "安装 nginx + certbot（yum）…"
    # 确保 epel-release 已启用
    sudo yum install -y epel-release &>/dev/null || true
    sudo yum install -y nginx certbot &>/dev/null
    # CentOS 7 yum 仓库里 certbot-nginx 插件不可用，用 pip3 安装
    if ! certbot plugins 2>/dev/null | grep -q nginx; then
      if command -v pip3 &>/dev/null; then
        sudo pip3 install certbot-nginx -q 2>/dev/null || true
      elif command -v pip &>/dev/null; then
        sudo pip install certbot-nginx -q 2>/dev/null || true
      fi
    fi
    CERTBOT_CMD="certbot --nginx"
  else
    warning "无法自动安装 nginx，请手动安装后配置反向代理至 http://localhost:$PORT"
    return
  fi

  # ── 确定 NGINX 配置目录 ────────────────────────────────────────────────
  if [ -d /etc/nginx/sites-available ]; then
    # Debian/Ubuntu 风格
    NGINX_CONF="/etc/nginx/sites-available/$SERVICE_NAME"
    sudo tee "$NGINX_CONF" > /dev/null << NGINX
server {
    listen 80;
    listen [::]:80;
    server_name $domain;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / {
        proxy_pass         http://127.0.0.1:$PORT;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade \$http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host \$host;
        proxy_set_header   X-Real-IP \$remote_addr;
        proxy_set_header   X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering    off;
        proxy_cache        off;
    }
}
NGINX
    sudo ln -sf "$NGINX_CONF" "/etc/nginx/sites-enabled/$SERVICE_NAME"
  else
    # CentOS/RHEL 风格：直接写 conf.d
    NGINX_CONF="/etc/nginx/conf.d/$SERVICE_NAME.conf"
    sudo tee "$NGINX_CONF" > /dev/null << NGINX
server {
    listen 80;
    listen [::]:80;
    server_name $domain;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / {
        proxy_pass         http://127.0.0.1:$PORT;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade \$http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host \$host;
        proxy_set_header   X-Real-IP \$remote_addr;
        proxy_set_header   X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering    off;
        proxy_cache        off;
    }
}
NGINX
  fi

  sudo mkdir -p /var/www/certbot
  sudo systemctl enable nginx
  sudo systemctl restart nginx
  success "NGINX 已启动，代理 $domain → localhost:$PORT"

  # ── 申请 Let's Encrypt 证书 ────────────────────────────────────────────
  info "申请 HTTPS 证书（Let's Encrypt）…"
  if sudo $CERTBOT_CMD -d "$domain" --non-interactive --agree-tos \
     --email "admin@$domain" --redirect 2>&1; then
    success "HTTPS 证书申请成功！"
    (crontab -l 2>/dev/null; echo "0 3 * * * certbot renew --quiet && systemctl reload nginx") | sudo crontab -
    success "已设置证书自动续期（每天 3:00 检查）"
  else
    warning "HTTPS 证书申请失败，请手动运行：sudo certbot --nginx -d $domain"
    warning "可能原因：域名未解析到此 IP，或 80/443 端口被防火墙拦截"
  fi
}

# ── 服务安装入口 ───────────────────────────────────────────────────────────
if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
  if $RUN_AS_ROOT; then
    info "配置 systemd 服务…"
    install_systemd
    # 若指定域名，额外安装 NGINX + HTTPS
    if [ -n "$DOMAIN" ]; then
      install_nginx_https "$DOMAIN"
    fi
  else
    warning "无 root 权限，跳过 systemd 注册。手动启动：$INSTALL_BIN --config $CONFIG_FILE"
    SHELL_RC="$HOME/.bashrc"
    echo "$SHELL" | grep -q zsh && SHELL_RC="$HOME/.zshrc"
    if ! grep -q "$HOME/.local/bin" "$SHELL_RC" 2>/dev/null; then
      echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$SHELL_RC"
      info "已将 ~/.local/bin 加入 PATH（$SHELL_RC）"
    fi
  fi
elif [ "$OS" = "darwin" ] && command -v launchctl &>/dev/null; then
  info "配置 launchd 服务…"
  install_launchd
else
  warning "无法自动配置服务，手动启动：$INSTALL_BIN --config $CONFIG_FILE"
fi

# ── 获取访问地址 ───────────────────────────────────────────────────────────
LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
PUBLIC_IP=$(curl -fsSL --max-time 4 https://api.ipify.org 2>/dev/null || true)

# ── 安装完成 ───────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  ✅  ZyHive 安装成功！版本: $LATEST          ${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════╝${NC}"
echo ""
if [ -n "$DOMAIN" ]; then
  echo -e "  🌐 访问地址：  ${BLUE}https://$DOMAIN${NC}"
else
  echo -e "  📍 本地访问：  ${BLUE}http://localhost:$PORT${NC}"
  [ -n "$LOCAL_IP"  ] && echo -e "  🏠 内网访问：  ${BLUE}http://$LOCAL_IP:$PORT${NC}"
  [ -n "$PUBLIC_IP" ] && echo -e "  🌐 公网访问：  ${BLUE}http://$PUBLIC_IP:$PORT${NC}"
fi
echo ""
[ -n "$SHOW_TOKEN" ] && echo -e "  🔑 管理员 Token：${GREEN}$SHOW_TOKEN${NC}"
echo ""
echo -e "  📄 配置文件：  $CONFIG_FILE"
echo -e "  🗂  成员目录：  $AGENTS_DIR"
echo -e "  📦 二进制：    $INSTALL_BIN"
echo ""
echo -e "  ${YELLOW}常用命令：${NC}"
if [ "$OS" = "linux" ] && $RUN_AS_ROOT; then
  echo "    查看状态：  sudo systemctl status $SERVICE_NAME"
  echo "    查看日志：  sudo journalctl -u $SERVICE_NAME -f"
  echo "    重启服务：  sudo systemctl restart $SERVICE_NAME"
elif [ "$OS" = "darwin" ]; then
  echo "    停止服务：  launchctl stop com.zyhive.$SERVICE_NAME"
  echo "    查看日志：  tail -f ~/Library/Logs/$SERVICE_NAME/stdout.log"
else
  echo "    手动启动：  $INSTALL_BIN --config $CONFIG_FILE"
fi
echo ""
