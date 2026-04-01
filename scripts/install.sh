#!/bin/bash
# ZyHive (引巢) — 一键安装脚本（通用版，自动识别平台）
# ─────────────────────────────────────────────────────────────────────────
# Linux / macOS:
#   curl -sSL https://install.zyling.ai/install | bash
#
# Windows (PowerShell):
#   irm https://install.zyling.ai/install | iex
#
# Windows (Git Bash / MSYS2 / Cygwin): 与 Linux/macOS 命令相同，脚本会
#   自动检测并调用系统 PowerShell 完成安装。
# ─────────────────────────────────────────────────────────────────────────
set -e

INSTALL_BASE="https://install.zyling.ai"

# ══════════════════════════════════════════════════════════════════════════
# 依赖检查：自动安装 curl（若缺失则尝试 apt/yum/apk/brew）
# ══════════════════════════════════════════════════════════════════════════
_ensure_curl() {
  if command -v curl &>/dev/null; then return 0; fi
  echo "  ⚙  未检测到 curl，尝试自动安装..."
  if command -v apt-get &>/dev/null; then
    sudo apt-get install -y -q curl
  elif command -v apt &>/dev/null; then
    sudo apt install -y -q curl
  elif command -v yum &>/dev/null; then
    sudo yum install -y -q curl
  elif command -v dnf &>/dev/null; then
    sudo dnf install -y -q curl
  elif command -v apk &>/dev/null; then
    sudo apk add --no-cache -q curl
  elif command -v brew &>/dev/null; then
    brew install curl
  else
    echo "❌ 未找到 curl 且无法自动安装，请手动安装后重试："
    echo "   Ubuntu/Debian:  sudo apt-get install -y curl"
    echo "   CentOS/RHEL:    sudo yum install -y curl"
    echo "   Alpine:         sudo apk add curl"
    exit 1
  fi
  command -v curl &>/dev/null || { echo "❌ curl 安装失败，请手动安装"; exit 1; }
  echo "  ✅ curl 安装完成"
}
_ensure_curl

# 下载辅助函数：优先 curl，回退 wget
_dl() {
  local url="$1" dest="$2" max="${3:-120}"
  if command -v curl &>/dev/null; then
    curl -fsSL --max-time "$max" "$url" ${dest:+-o "$dest"}
  elif command -v wget &>/dev/null; then
    if [ -n "$dest" ]; then
      wget -q --timeout="$max" -O "$dest" "$url"
    else
      wget -qO- --timeout="$max" "$url"
    fi
  else
    echo "❌ 未找到 curl 或 wget"; exit 1
  fi
}

# 仅支持 macOS 和 Linux，不支持 Windows

SERVICE_NAME="zyhive"
BINARY_NAME="zyhive"
PORT=8080
DOMAIN=""
NO_ROOT=false   # --no-root 强制用户目录安装

# ── 向导参数（可通过 flag 跳过交互）────────────────────────────────────────
SKIP_SETUP=false          # --skip-setup 跳过向导
YES_MODE=false            # --yes 全部默认
WIZARD_PROVIDER=""        # --provider anthropic|openai|deepseek|kimi|minimax|gemini|zhipu|custom
WIZARD_API_KEY=""         # --api-key sk-xxx  (任意提供商)
WIZARD_BASE_URL=""        # --base-url https://xxx (自定义接口地址)
WIZARD_MODEL=""           # --model claude-sonnet-4-6
WIZARD_TG_TOKEN=""        # --telegram-token 123:ABC
WIZARD_TG_ALLOWED=""      # --telegram-allowed "111,222,333"

# ── 解析参数 ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)            DOMAIN="$2";           shift 2 ;;
    --port)              PORT="$2";             shift 2 ;;
    --no-root)           NO_ROOT=true;          shift   ;;
    --skip-setup)        SKIP_SETUP=true;       shift   ;;
    --yes|-y)            YES_MODE=true;         shift   ;;
    --provider)          WIZARD_PROVIDER="$2";  shift 2 ;;
    --api-key)           WIZARD_API_KEY="$2";   shift 2 ;;
    --anthropic-key)     WIZARD_API_KEY="$2"; WIZARD_PROVIDER="anthropic"; shift 2 ;;
    --openai-key)        WIZARD_API_KEY="$2"; WIZARD_PROVIDER="openai";    shift 2 ;;
    --deepseek-key)      WIZARD_API_KEY="$2"; WIZARD_PROVIDER="deepseek";  shift 2 ;;
    --base-url)          WIZARD_BASE_URL="$2";  shift 2 ;;
    --model)             WIZARD_MODEL="$2";     shift 2 ;;
    --telegram-token)    WIZARD_TG_TOKEN="$2";  shift 2 ;;
    --telegram-allowed)  WIZARD_TG_ALLOWED="$2"; shift 2 ;;
    *) shift ;;
  esac
done

# ── 颜色输出 ───────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'
info()    { echo -e "${BLUE}ℹ${NC}  $*"; }
success() { echo -e "${GREEN}✅${NC} $*"; }
warning() { echo -e "${YELLOW}⚠${NC}  $*"; }
error()   { echo -e "${RED}❌${NC} $*"; exit 1; }

# ── 检测架构 ───────────────────────────────────────────────────────────────
RAW_ARCH=$(uname -m)
case "$RAW_ARCH" in
  x86_64)         ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  armv7l|armv6l)  ARCH="arm"   ;;
  *) error "不支持的架构: $RAW_ARCH" ;;
esac

RAW_OS=$(uname -s)
case "$RAW_OS" in
  Linux)  OS="linux"  ;;
  Darwin) OS="darwin" ;;
  *) error "不支持的操作系统: $RAW_OS" ;;
esac

# ── 自动获取 root 权限 ─────────────────────────────────────────────────────
# 优先级：已是 root > sudo > 用户目录（--no-root 跳过前两步）
SUDO=""
USE_SYSTEM_PATH=false

if $NO_ROOT; then
  USE_SYSTEM_PATH=false
elif [ "$(id -u)" = "0" ]; then
  USE_SYSTEM_PATH=true
elif command -v sudo &>/dev/null; then
  echo ""
  echo -e "${BOLD}ZyHive 需要管理员权限来安装系统服务。${NC}"
  echo -e "  将安装到系统目录，无需 root 也可用 ${YELLOW}--no-root${NC} 参数跳过。"
  echo ""
  if sudo -v 2>/dev/null; then
    SUDO="sudo"
    USE_SYSTEM_PATH=true
    # 保活 sudo 票据（后台每 60s 刷新一次，避免长下载后超时）
    (while true; do sudo -v; sleep 60; done) &
    SUDO_KEEPALIVE_PID=$!
    trap "kill $SUDO_KEEPALIVE_PID 2>/dev/null; true" EXIT
  else
    warning "sudo 认证失败，降级到用户目录安装"
    USE_SYSTEM_PATH=false
  fi
else
  warning "未找到 sudo，降级到用户目录安装"
  USE_SYSTEM_PATH=false
fi

# ── 确定安装路径 ───────────────────────────────────────────────────────────
if $USE_SYSTEM_PATH; then
  INSTALL_BIN="/usr/local/bin/$BINARY_NAME"
  if [ "$OS" = "darwin" ]; then
    CONFIG_DIR="/usr/local/etc/$SERVICE_NAME"
  else
    CONFIG_DIR="/etc/$SERVICE_NAME"
  fi
  AGENTS_DIR="/var/lib/$SERVICE_NAME/agents"
else
  INSTALL_BIN="$HOME/.local/bin/$BINARY_NAME"
  CONFIG_DIR="$HOME/.config/$SERVICE_NAME"
  AGENTS_DIR="$HOME/.local/share/$SERVICE_NAME/agents"
fi

CONFIG_FILE="$CONFIG_DIR/$SERVICE_NAME.json"

# ── 获取最新版本号 ─────────────────────────────────────────────────────────
info "查询最新版本…"
LATEST=$(_dl "$INSTALL_BASE/latest" "" 8 2>/dev/null \
  | grep -o '"version":"[^"]*"' | sed 's/"version":"//;s/"//g')
if [ -z "$LATEST" ]; then
  info "CF 镜像不可用，回退到 GitHub API…"
  LATEST=$(_dl "https://api.github.com/repos/Zyling-ai/zyhive/releases/latest" "" 10 \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
fi
[ -z "$LATEST" ] && error "无法获取最新版本，请检查网络连接。"
info "最新版本：$LATEST"

BINARY_FILENAME="zyhive-${OS}-${ARCH}"
BINARY_URL="$INSTALL_BASE/dl/$LATEST/$BINARY_FILENAME"
BINARY_URL_FALLBACK="https://install.zyling.ai/dl/$LATEST/$BINARY_FILENAME"

# ── 检测是否已安装（更新流程）─────────────────────────────────────────────
if [ -f "$INSTALL_BIN" ]; then
  CURRENT=$("$INSTALL_BIN" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  [ -z "$CURRENT" ] && CURRENT="（未知版本）"

  echo ""
  echo -e "  ${YELLOW}检测到已安装的 ZyHive：${BOLD}$CURRENT${NC}"
  echo -e "  ${BLUE}最新版本：${BOLD}$LATEST${NC}"
  echo ""

  if [ "$CURRENT" = "$LATEST" ]; then
    echo -e "  ${GREEN}✅ 已是最新版本，无需更新。${NC}"
    echo ""
    exit 0
  fi

  printf "  是否更新 %s → %s？[Y/n] " "$CURRENT" "$LATEST"
  read -r CONFIRM </dev/tty
  CONFIRM="${CONFIRM:-Y}"
  if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo ""
    info "已取消，当前版本 $CURRENT 保持不变。"
    exit 0
  fi

  echo ""
  info "开始更新 $CURRENT → $LATEST…"

  # 停止服务
  if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
    $SUDO systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    info "服务已停止"
  elif [ "$OS" = "darwin" ]; then
    _LABEL="com.zyhive.$SERVICE_NAME"
    if $USE_SYSTEM_PATH; then
      $SUDO launchctl stop "$_LABEL" 2>/dev/null || true
    else
      launchctl stop "$_LABEL" 2>/dev/null || true
    fi
    info "服务已停止"
  fi

  # 下载新版本
  info "下载 $BINARY_NAME $LATEST ($OS/$ARCH)…"
  TMP_BIN=$(mktemp)
  if ! _dl "$BINARY_URL" "$TMP_BIN" 120 2>/dev/null; then
    info "CF 镜像下载失败，回退到 GitHub…"
    _dl "$BINARY_URL_FALLBACK" "$TMP_BIN" 120 \
      || { rm -f "$TMP_BIN"; error "下载失败。\n  CF: $BINARY_URL\n  GitHub: $BINARY_URL_FALLBACK"; }
  fi

  # 替换二进制（先 rm 避免 Text file busy）
  $SUDO rm -f "$INSTALL_BIN"
  $SUDO install -m 755 "$TMP_BIN" "$INSTALL_BIN"
  rm -f "$TMP_BIN"
  success "二进制已更新至 $INSTALL_BIN"

  # 重启服务
  if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
    $SUDO systemctl start "$SERVICE_NAME"
    success "服务已重启"
    info "查看日志：sudo journalctl -u $SERVICE_NAME -f"
  elif [ "$OS" = "darwin" ]; then
    _LABEL="com.zyhive.$SERVICE_NAME"
    if $USE_SYSTEM_PATH; then
      $SUDO launchctl start "$_LABEL" 2>/dev/null || true
    else
      launchctl start "$_LABEL" 2>/dev/null || true
    fi
    success "服务已重启"
  fi

  # 停止 sudo 保活进程
  [ -n "$SUDO_KEEPALIVE_PID" ] && kill "$SUDO_KEEPALIVE_PID" 2>/dev/null || true

  echo ""
  echo -e "${GREEN}╔══════════════════════════════════════════════╗${NC}"
  printf  "${GREEN}║  ✅  ZyHive 更新成功！%-22s║${NC}\n" "$CURRENT → $LATEST"
  echo -e "${GREEN}╚══════════════════════════════════════════════╝${NC}"
  echo ""
  exit 0
fi

# ── 以下为全新安装流程 ─────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}${BOLD}🚀 正在安装 ZyHive (引巢 · AI 团队操作系统)…${NC}"
echo ""
info "操作系统：$RAW_OS / $RAW_ARCH → 下载 $OS-$ARCH"
info "安装路径：$INSTALL_BIN"
info "配置目录：$CONFIG_DIR"
if $USE_SYSTEM_PATH; then
  info "权限模式：系统安装（root）"
else
  info "权限模式：用户目录安装"
fi
[ -n "$DOMAIN" ] && info "域名：$DOMAIN（将自动配置 NGINX + HTTPS）"
echo ""

# ── 下载二进制 ─────────────────────────────────────────────────────────────
info "下载 $BINARY_NAME $LATEST ($OS/$ARCH)…"
TMP_BIN=$(mktemp)
if ! _dl "$BINARY_URL" "$TMP_BIN" 120 2>/dev/null; then
  info "CF 镜像下载失败，回退到 GitHub…"
  _dl "$BINARY_URL_FALLBACK" "$TMP_BIN" 120 \
    || { rm -f "$TMP_BIN"; error "下载失败。\n  CF: $BINARY_URL\n  GitHub: $BINARY_URL_FALLBACK"; }
fi

# 创建目录并安装二进制
$SUDO mkdir -p "$(dirname "$INSTALL_BIN")" "$CONFIG_DIR" "$AGENTS_DIR"
$SUDO install -m 755 "$TMP_BIN" "$INSTALL_BIN"
rm -f "$TMP_BIN"
success "二进制已安装至 $INSTALL_BIN"

# ── 确保 PATH 包含安装目录 ────────────────────────────────────────────────
if ! echo "$PATH" | grep -q "$(dirname "$INSTALL_BIN")"; then
  for RC in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
    [ -f "$RC" ] || continue
    if ! grep -q "$(dirname "$INSTALL_BIN")" "$RC" 2>/dev/null; then
      echo "export PATH=\"$(dirname "$INSTALL_BIN"):\$PATH\"" >> "$RC"
      info "已将 $(dirname "$INSTALL_BIN") 加入 PATH（$RC）"
    fi
  done
fi

# ══════════════════════════════════════════════════════════════════════════
# 首次配置向导
# ══════════════════════════════════════════════════════════════════════════

# 提供商预设：provider_id|display_name|default_model|model_provider|base_url
_PROVIDERS=(
  "anthropic|Anthropic (Claude)|claude-sonnet-4-6|anthropic|"
  "openai|OpenAI (GPT-4o)|gpt-4o|openai|"
  "deepseek|DeepSeek|deepseek-chat|deepseek|https://api.deepseek.com"
  "moonshot|月之暗面 Kimi|moonshot-v1-8k|moonshot|https://api.moonshot.cn/v1"
  "minimax|MiniMax|abab5.5s-chat|minimax|https://api.minimax.chat/v1"
  "google|Google Gemini|gemini-2.0-flash|google|"
  "zhipu|智谱 AI (GLM)|glm-4-flash|zhipu|https://open.bigmodel.cn/api/paas/v4"
  "custom|自定义 (OpenAI 兼容)|gpt-4o|custom|"
)

# 转义 JSON 字符串
_json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

# 生成 providers JSON 块
_make_providers_json() {
  local p_id="$1" p_name="$2" api_key="$3" base_url="$4"
  local key_escaped; key_escaped=$(_json_escape "$api_key")
  local base_url_json=""
  [ -n "$base_url" ] && base_url_json=", \"baseUrl\": \"$(_json_escape "$base_url")\""
  printf '[{"id":"%s","name":"%s","provider":"%s","apiKey":"%s","status":"untested"%s}]' \
    "$p_id" "$p_name" "$p_id" "$key_escaped" "$base_url_json"
}

# 生成 models JSON 块
_make_models_json() {
  local p_id="$1" model_id="$2" model_provider="$3"
  printf '[{"id":"default","name":"%s / %s","provider":"%s","model":"%s","providerId":"%s","isDefault":true,"status":"untested"}]' \
    "$model_provider" "$model_id" "$model_provider" "$model_id" "$p_id"
}

# 生成 channels JSON 块（Telegram）
_make_channels_json() {
  local tg_token="$1" tg_allowed="$2"
  if [ -z "$tg_token" ]; then
    printf '[]'
    return
  fi
  local allowed_json="[]"
  if [ -n "$tg_allowed" ]; then
    allowed_json="[$(echo "$tg_allowed" | tr ',' '\n' | awk '{printf "%s%s", (NR>1?",":""), $0}')]"
  fi
  printf '[{"id":"telegram","name":"Telegram","type":"telegram","config":{"botToken":"%s"},"enabled":true,"status":"untested","allowedFrom":%s}]' \
    "$(_json_escape "$tg_token")" "$allowed_json"
}

# 向导主函数
_wizard_setup() {
  local cfg_file="$1"
  local bind_mode="$2"

  # ── 已有 flag 时直接使用，跳过交互 ──────────────────────────────────────
  local sel_provider="$WIZARD_PROVIDER"
  local sel_api_key="$WIZARD_API_KEY"
  local sel_base_url="$WIZARD_BASE_URL"
  local sel_model="$WIZARD_MODEL"
  local sel_tg_token="$WIZARD_TG_TOKEN"
  local sel_tg_allowed="$WIZARD_TG_ALLOWED"

  # 是否运行在 TTY（可交互）
  local interactive=false
  [ -t 0 ] && [ "$SKIP_SETUP" = "false" ] && [ "$YES_MODE" = "false" ] && interactive=true

  # ── 交互向导 ───────────────────────────────────────────────────────────
  if $interactive; then
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║   🚀  ZyHive 快速配置向导                           ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════╝${NC}"
    echo -e "  ${YELLOW}按 Enter 跳过任一步骤，稍后在面板中配置${NC}"
    echo ""

    # ── Step 1: 选择提供商 ──────────────────────────────────────────────
    if [ -z "$sel_provider" ]; then
      echo -e "  ${BOLD}Step 1/3  —  选择 AI 模型提供商${NC}"
      echo ""
      local i=1
      for entry in "${_PROVIDERS[@]}"; do
        local pname; pname=$(echo "$entry" | cut -d'|' -f2)
        printf "    %d) %s\n" "$i" "$pname"
        i=$((i+1))
      done
      echo "    0) 跳过（稍后手动配置）"
      echo ""
      printf "  请选择 [0-%d]: " "$((${#_PROVIDERS[@]}))"
      local choice; read -r choice </dev/tty
      echo ""

      if [ -n "$choice" ] && [ "$choice" -ge 1 ] && [ "$choice" -le "${#_PROVIDERS[@]}" ] 2>/dev/null; then
        local entry="${_PROVIDERS[$((choice-1))]}"
        sel_provider=$(echo "$entry" | cut -d'|' -f1)
        local default_model; default_model=$(echo "$entry" | cut -d'|' -f3)
        local default_base; default_base=$(echo "$entry" | cut -d'|' -f5)
        [ -z "$sel_model"    ] && sel_model="$default_model"
        [ -z "$sel_base_url" ] && sel_base_url="$default_base"
      fi
    fi

    # ── Step 2: 输入 API Key ────────────────────────────────────────────
    if [ -n "$sel_provider" ] && [ "$sel_provider" != "0" ] && [ -z "$sel_api_key" ]; then
      local p_display="$sel_provider"
      for entry in "${_PROVIDERS[@]}"; do
        if [ "$(echo "$entry"|cut -d'|' -f1)" = "$sel_provider" ]; then
          p_display=$(echo "$entry"|cut -d'|' -f2)
          break
        fi
      done
      echo -e "  ${BOLD}Step 2/3  —  输入 API Key${NC}"
      echo -e "  ${YELLOW}提供商：${NC}${p_display}"

      if [ "$sel_provider" = "custom" ]; then
        printf "  Base URL (如 https://api.example.com/v1): "
        read -r sel_base_url </dev/tty
      fi

      printf "  API Key: "
      # 读取时不回显（隐藏 key）
      if [ -t 0 ]; then
        stty -echo 2>/dev/null || true
        read -r sel_api_key </dev/tty
        stty echo 2>/dev/null || true
        echo ""
      else
        read -r sel_api_key </dev/tty
      fi
      echo ""

      if [ "$sel_provider" = "custom" ] && [ -z "$sel_model" ]; then
        printf "  模型 ID (如 gpt-4o): "
        read -r sel_model </dev/tty
        echo ""
      fi
    fi

    # ── Step 3: Telegram 配置 ───────────────────────────────────────────
    if [ -z "$sel_tg_token" ]; then
      echo -e "  ${BOLD}Step 3/3  —  Telegram Bot 配置（可选）${NC}"
      echo -e "  ${YELLOW}通过 @BotFather 创建 Bot 并获取 Token${NC}"
      printf "  Bot Token (直接 Enter 跳过): "
      read -r sel_tg_token </dev/tty
      echo ""

      if [ -n "$sel_tg_token" ]; then
        printf "  允许的用户 ID（逗号分隔，直接 Enter 跳过鉴权）: "
        read -r sel_tg_allowed </dev/tty
        echo ""
      fi
    fi
  fi

  # ── 生成配置文件 ────────────────────────────────────────────────────────
  local admin_token
  admin_token=$(openssl rand -hex 16 2>/dev/null \
    || tr -dc 'a-f0-9' < /dev/urandom | head -c 32)

  # 构建各 JSON 块
  local providers_json='[]'
  local models_json='[]'
  local channels_json='[]'

  if [ -n "$sel_provider" ] && [ "$sel_provider" != "0" ] && [ -n "$sel_api_key" ]; then
    local p_display="$sel_provider"
    local p_default_model="$sel_model"
    local p_model_provider="$sel_provider"
    for entry in "${_PROVIDERS[@]}"; do
      if [ "$(echo "$entry"|cut -d'|' -f1)" = "$sel_provider" ]; then
        p_display=$(echo "$entry"|cut -d'|' -f2)
        [ -z "$p_default_model" ] && p_default_model=$(echo "$entry"|cut -d'|' -f3)
        p_model_provider=$(echo "$entry"|cut -d'|' -f4)
        break
      fi
    done
    providers_json=$(_make_providers_json "$sel_provider" "$p_display" "$sel_api_key" "$sel_base_url")
    models_json=$(_make_models_json "$sel_provider" "${p_default_model:-gpt-4o}" "$p_model_provider")
  fi

  [ -n "$sel_tg_token" ] && \
    channels_json=$(_make_channels_json "$sel_tg_token" "${sel_tg_allowed:-$WIZARD_TG_ALLOWED}")

  # 写配置
  cat > "$cfg_file" << CFGEOF
{
  "configVersion": 3,
  "gateway":  { "port": ${PORT}, "bind": "${bind_mode}" },
  "agents":   { "dir": "${AGENTS_DIR}" },
  "providers": ${providers_json},
  "models":    ${models_json},
  "channels":  ${channels_json},
  "tools":     [],
  "skills":    [],
  "auth":      { "mode": "token", "token": "${admin_token}" }
}
CFGEOF

  echo ""
  echo -e "  ${YELLOW}🔑 管理员 Token：${NC}${GREEN}${BOLD}${admin_token}${NC}"
  echo -e "     已保存至 ${cfg_file}"

  [ -n "$sel_provider" ] && [ -n "$sel_api_key" ] && \
    echo -e "  🤖 已配置提供商：${GREEN}${sel_provider}${NC}  模型：${GREEN}${sel_model:-默认}${NC}"
  [ -n "$sel_tg_token" ] && \
    echo -e "  💬 Telegram Bot 已配置"

  SHOW_TOKEN="$admin_token"
  SHOW_PROVIDER="$sel_provider"
  SHOW_MODEL="$sel_model"
}

# ── 生成默认配置（若不存在）────────────────────────────────────────────────
SHOW_TOKEN=""
SHOW_PROVIDER=""
SHOW_MODEL=""

if [ ! -f "$CONFIG_FILE" ]; then
  BIND_MODE="lan"
  [ -n "$DOMAIN" ] && BIND_MODE="localhost"
  _wizard_setup "$CONFIG_FILE" "$BIND_MODE"
fi

# ═══════════════════════════════════════════════════════════════════════════
# 服务安装
# ═══════════════════════════════════════════════════════════════════════════

# ── Linux systemd ──────────────────────────────────────────────────────────
install_systemd() {
  local UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

  $SUDO tee "$UNIT_FILE" > /dev/null << UNIT
[Unit]
Description=ZyHive — AI 团队操作系统
Documentation=https://github.com/Zyling-ai/zyhive
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_BIN --config $CONFIG_FILE
WorkingDirectory=$CONFIG_DIR
Restart=always
RestartSec=2
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
UNIT

  $SUDO systemctl daemon-reload
  $SUDO systemctl enable "$SERVICE_NAME"
  $SUDO systemctl start  "$SERVICE_NAME"
  success "systemd 服务已启动"
  info   "查看状态：sudo systemctl status $SERVICE_NAME"
  info   "查看日志：sudo journalctl -u $SERVICE_NAME -f"
}

# ── macOS launchd ──────────────────────────────────────────────────────────
install_launchd() {
  local LABEL="com.zyhive.$SERVICE_NAME"

  if $USE_SYSTEM_PATH; then
    # root 安装 → LaunchDaemons（全局，随系统启动）
    local PLIST_FILE="/Library/LaunchDaemons/${LABEL}.plist"
    local LOG_DIR="/var/log/$SERVICE_NAME"
    $SUDO mkdir -p "$LOG_DIR"
    $SUDO tee "$PLIST_FILE" > /dev/null << PLIST
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
    $SUDO launchctl load -w "$PLIST_FILE"
    success "LaunchDaemon 已加载（随系统启动）：$LABEL"
    info   "日志目录：$LOG_DIR"
    info   "停止服务：sudo launchctl stop $LABEL"
    info   "查看日志：sudo tail -f $LOG_DIR/stdout.log"
  else
    # 用户级 → LaunchAgents（登录后启动）
    local PLIST_DIR="$HOME/Library/LaunchAgents"
    local PLIST_FILE="$PLIST_DIR/${LABEL}.plist"
    local LOG_DIR="$HOME/Library/Logs/$SERVICE_NAME"
    mkdir -p "$PLIST_DIR" "$LOG_DIR"
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
    success "LaunchAgent 已加载（用户级，登录后自启）：$LABEL"
    info   "日志目录：$LOG_DIR"
    info   "停止服务：launchctl stop $LABEL"
    info   "查看日志：tail -f $LOG_DIR/stdout.log"
  fi
}

# ── NGINX + HTTPS（Linux，--domain 参数） ─────────────────────────────────
install_nginx_https() {
  local domain="$1"

  if command -v apt-get &>/dev/null; then
    info "安装 nginx + certbot（apt）…"
    $SUDO apt-get update -q
    $SUDO apt-get install -y -q nginx certbot python3-certbot-nginx
    CERTBOT_PLUGIN="--nginx"
  elif command -v yum &>/dev/null; then
    info "安装 nginx + certbot（yum）…"
    $SUDO yum install -y epel-release &>/dev/null || true
    $SUDO yum install -y nginx certbot &>/dev/null
    # CentOS 7 用 webroot 模式（certbot-nginx 插件不可靠）
    CERTBOT_PLUGIN="--webroot -w /var/www/certbot"
  else
    warning "无法自动安装 nginx，请手动配置反向代理至 http://localhost:$PORT"
    return
  fi

  # 写 nginx 配置
  if [ -d /etc/nginx/sites-available ]; then
    NGINX_CONF="/etc/nginx/sites-available/$SERVICE_NAME"
    NGINX_LINK="/etc/nginx/sites-enabled/$SERVICE_NAME"
  else
    NGINX_CONF="/etc/nginx/conf.d/$SERVICE_NAME.conf"
    NGINX_LINK=""
  fi

  $SUDO tee "$NGINX_CONF" > /dev/null << NGINX
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

  [ -n "$NGINX_LINK" ] && $SUDO ln -sf "$NGINX_CONF" "$NGINX_LINK"
  $SUDO mkdir -p /var/www/certbot
  $SUDO systemctl enable nginx
  $SUDO systemctl restart nginx
  success "NGINX 已启动 → http://$domain"

  info "申请 Let's Encrypt 证书…"
  if $SUDO certbot $CERTBOT_PLUGIN -d "$domain" \
     --non-interactive --agree-tos --email "admin@$domain" --redirect 2>&1; then
    success "HTTPS 证书申请成功！"
    (crontab -l 2>/dev/null; echo "0 3 * * * certbot renew --quiet && systemctl reload nginx") \
      | $SUDO crontab -
    success "已设置证书自动续期（每天 3:00）"
  else
    warning "证书申请失败，请确认域名已解析到此 IP，80 端口可访问"
    warning "手动申请：sudo certbot $CERTBOT_PLUGIN -d $domain"
  fi
}

# ── 服务安装入口 ───────────────────────────────────────────────────────────
echo ""
if [ "$OS" = "linux" ]; then
  if command -v systemctl &>/dev/null; then
    info "配置 systemd 服务…"
    install_systemd
    [ -n "$DOMAIN" ] && $USE_SYSTEM_PATH && install_nginx_https "$DOMAIN"
  else
    warning "systemd 不可用，请手动启动：$INSTALL_BIN --config $CONFIG_FILE"
  fi
elif [ "$OS" = "darwin" ]; then
  info "配置 launchd 服务…"
  install_launchd
fi

# ── 获取访问地址 ───────────────────────────────────────────────────────────
if [ "$OS" = "linux" ]; then
  LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
else
  LOCAL_IP=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)
fi
PUBLIC_IP=$(_dl "https://api.ipify.org" "" 5 2>/dev/null || true)

# 停止 sudo 保活进程
[ -n "$SUDO_KEEPALIVE_PID" ] && kill "$SUDO_KEEPALIVE_PID" 2>/dev/null || true

# ── 完成 ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════╗${NC}"
printf  "${GREEN}║  ✅  ZyHive 安装成功！版本: %-17s║${NC}\n" "$LATEST"
echo -e "${GREEN}╚══════════════════════════════════════════════╝${NC}"
echo ""
if [ -n "$DOMAIN" ]; then
  echo -e "  🌐 访问地址：  ${BLUE}https://$DOMAIN${NC}"
else
  echo -e "  📍 本地访问：  ${BLUE}http://localhost:$PORT${NC}"
  [ -n "$LOCAL_IP"  ] && echo -e "  🏠 内网访问：  ${BLUE}http://$LOCAL_IP:$PORT${NC}"
  [ -n "$PUBLIC_IP" ] && echo -e "  🌐 公网访问：  ${BLUE}http://$PUBLIC_IP:$PORT${NC}"
fi
[ -n "$SHOW_TOKEN"    ] && echo -e "\n  🔑 管理员 Token：  ${GREEN}${BOLD}$SHOW_TOKEN${NC}"
[ -n "$SHOW_PROVIDER" ] && echo -e "  🤖 AI 提供商：     ${GREEN}$SHOW_PROVIDER${NC}${SHOW_MODEL:+ / $SHOW_MODEL}"
[ -z "$SHOW_PROVIDER" ] && echo -e "  ${YELLOW}⚠  未配置 AI Key，请登录面板 → 设置 → 提供商 添加${NC}"
echo ""
echo -e "  📄 配置文件：  $CONFIG_FILE"
echo -e "  🗂  成员目录：  $AGENTS_DIR"
echo -e "  📦 二进制：    $INSTALL_BIN"
echo ""
echo -e "  ${YELLOW}常用命令：${NC}"
if [ "$OS" = "linux" ]; then
  echo "    停止服务：  sudo systemctl stop $SERVICE_NAME"
  echo "    查看日志：  sudo journalctl -u $SERVICE_NAME -f"
  echo "    重启服务：  sudo systemctl restart $SERVICE_NAME"
  echo "    CLI  管理：  sudo $BINARY_NAME"
elif [ "$OS" = "darwin" ] && $USE_SYSTEM_PATH; then
  echo "    停止服务：  sudo launchctl stop com.zyhive.$SERVICE_NAME"
  echo "    查看日志：  sudo tail -f /var/log/$SERVICE_NAME/stdout.log"
  echo "    CLI  管理：  sudo $BINARY_NAME"
else
  echo "    停止服务：  launchctl stop com.zyhive.$SERVICE_NAME"
  echo "    查看日志：  tail -f ~/Library/Logs/$SERVICE_NAME/stdout.log"
  echo "    CLI  管理：  $BINARY_NAME"
fi
echo ""
