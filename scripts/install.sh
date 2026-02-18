#!/bin/bash
# AI Company Panel — One-line installer
# Usage: curl -sSL https://raw.githubusercontent.com/sunhuihui6688-star/ai-panel/main/scripts/install.sh | bash
set -e

REPO="sunhuihui6688-star/ai-panel"
INSTALL_DIR="/opt/ai-panel"
SERVICE_NAME="ai-panel"
PORT=8080

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo ""
echo "🚀 Installing AI Company Panel..."
echo "   OS: $OS / $ARCH"
echo ""

# Get latest release
LATEST=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": "\(.*\)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "❌ Could not fetch latest release. Check your internet connection."
  exit 1
fi

BINARY_URL="https://github.com/$REPO/releases/download/$LATEST/aipanel-${OS}-${ARCH}"

# Create install directory
sudo mkdir -p "$INSTALL_DIR"

# Download binary
echo "⬇️  Downloading aipanel $LATEST..."
sudo curl -sSL "$BINARY_URL" -o "$INSTALL_DIR/aipanel"
sudo chmod +x "$INSTALL_DIR/aipanel"

# Generate default config if not exists
if [ ! -f "$INSTALL_DIR/aipanel.json" ]; then
  ADMIN_TOKEN=$(openssl rand -hex 16 2>/dev/null || cat /dev/urandom | tr -dc 'a-f0-9' | head -c 32)
  sudo tee "$INSTALL_DIR/aipanel.json" > /dev/null << CONF
{
  "gateway": { "port": $PORT, "bind": "lan" },
  "agents":  { "dir": "$INSTALL_DIR/agents" },
  "models":  { "primary": "anthropic/claude-sonnet-4-6" },
  "auth":    { "mode": "token", "token": "$ADMIN_TOKEN" }
}
CONF
  echo "🔑 Admin token: $ADMIN_TOKEN  (saved in $INSTALL_DIR/aipanel.json)"
fi

# Create systemd service (Linux only)
if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
  sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null << UNIT
[Unit]
Description=AI Company Panel
After=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/aipanel
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
  sudo systemctl daemon-reload
  sudo systemctl enable "$SERVICE_NAME"
  sudo systemctl start "$SERVICE_NAME"
fi

# Get IPs and print access info
LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || ifconfig | grep 'inet ' | grep -v 127 | awk '{print $2}' | head -1)
PUBLIC_IP=$(curl -s --max-time 3 https://api.ipify.org 2>/dev/null || echo "")

echo ""
echo "✅ AI Company Panel 安装成功！"
echo ""
echo "  本地访问：  http://localhost:$PORT"
[ -n "$LOCAL_IP"  ] && echo "  内网访问：  http://$LOCAL_IP:$PORT"
[ -n "$PUBLIC_IP" ] && echo "  公网访问：  http://$PUBLIC_IP:$PORT"
echo ""
echo "  配置文件：  $INSTALL_DIR/aipanel.json"
echo ""
