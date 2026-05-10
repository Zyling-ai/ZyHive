#!/usr/bin/env bash
# scripts/deploy-aws.sh — Hot-deploy ZyHive to the AWS staging server.
#
# Target instance:
#   region:     ap-east-1
#   instance:   i-04405815de67eda10
#   public IP:  18.162.161.138
#   user:       ubuntu (passwordless sudo)
#   arch:       arm64 (t4g.small Graviton)
#   AMI:        Ubuntu 22.04 LTS
#
# Usage:
#   ./scripts/deploy-aws.sh [version-string]
#
# Required env (set in GitHub Secrets or local shell):
#   AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY  — IAM creds
#
# Optional env:
#   AWS_REGION       (default ap-east-1)
#   AWS_INSTANCE_ID  (default i-04405815de67eda10)
#   AWS_SSH_USER     (default ubuntu)
#   AWS_HOST         (default 18.162.161.138)
#   SSH_KEY_PATH     (default $HOME/.ssh/zyhive_aws)
#   ZYHIVE_FLAGS_ENV (optional file with ZYHIVE_EXPERIMENTAL_* settings)
#
# What it does:
#   1. vite-builds the UI and copies ui/dist → cmd/aipanel/ui_dist.
#   2. CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build with version ldflag.
#   3. Generates an ed25519 keypair if none exists locally and pushes the
#      public key via AWS EC2 Instance Connect (60-second TTL on instance).
#   4. SCP-uploads the binary to /tmp/zyhive-deploy on the host.
#   5. Backs up the running binary, swaps it, restarts systemd unit,
#      runs a basic smoke probe (/api/version), and tails the unit status.
#
# Idempotent. Safe to re-run. Fails loud on any step.

set -euo pipefail

# ---------- inputs --------------------------------------------------------
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

VERSION="${1:-$(git rev-parse --short HEAD)-aws-staging}"
AWS_REGION="${AWS_REGION:-ap-east-1}"
AWS_INSTANCE_ID="${AWS_INSTANCE_ID:-i-04405815de67eda10}"
AWS_SSH_USER="${AWS_SSH_USER:-ubuntu}"
AWS_HOST="${AWS_HOST:-18.162.161.138}"
SSH_KEY_PATH="${SSH_KEY_PATH:-$HOME/.ssh/zyhive_aws}"

# Make AWS CLI available even when running on a dev box that pip-installed
# it to ~/.local/bin (where ec2-instance-connect lives).
export PATH="$HOME/.local/bin:$PATH"

if ! command -v aws >/dev/null 2>&1; then
    echo "❌ aws CLI not found in PATH — install via 'pip install awscli'" >&2
    exit 1
fi
if [[ -z "${AWS_ACCESS_KEY_ID:-}" || -z "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
    echo "❌ AWS credentials missing — export AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY" >&2
    exit 1
fi
export AWS_DEFAULT_REGION="$AWS_REGION"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "▶ AWS staging deploy"
echo "   version:   $VERSION"
echo "   region:    $AWS_REGION"
echo "   instance:  $AWS_INSTANCE_ID  ($AWS_HOST)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ---------- 1. build UI ---------------------------------------------------
echo "🧱 [1/5] vite build (ui/)..."
pushd ui >/dev/null
if [[ ! -d node_modules ]]; then npm install --silent; fi
npx vite build 2>&1 | tail -5
popd >/dev/null

echo "🔄 [2/5] sync ui/dist → cmd/aipanel/ui_dist (go:embed source)..."
rm -rf cmd/aipanel/ui_dist
cp -r ui/dist cmd/aipanel/ui_dist
echo "   ui_dist: $(find cmd/aipanel/ui_dist/assets -type f 2>/dev/null | wc -l | tr -d ' ') asset files"

# ---------- 3. cross-compile for ARM64 ------------------------------------
echo "🔨 [3/5] CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build..."
BIN_PATH="/tmp/zyhive-deploy-aws-$$"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags "-X main.Version=${VERSION}" \
    -o "$BIN_PATH" \
    ./cmd/aipanel/
ls -lh "$BIN_PATH"
echo "   built: $(file "$BIN_PATH" 2>/dev/null || echo "$BIN_PATH")"

# ---------- 4. SSH access via EC2 Instance Connect ------------------------
echo "🔑 [4/5] ensure SSH key + push to instance via EC2 Instance Connect..."
mkdir -p "$(dirname "$SSH_KEY_PATH")"
if [[ ! -f "$SSH_KEY_PATH" ]]; then
    ssh-keygen -t ed25519 -N '' -f "$SSH_KEY_PATH" -C "zyhive-aws-deploy@$(hostname)" >/dev/null
    echo "   generated new keypair: $SSH_KEY_PATH"
fi
chmod 600 "$SSH_KEY_PATH"

aws ec2-instance-connect send-ssh-public-key \
    --instance-id "$AWS_INSTANCE_ID" \
    --instance-os-user "$AWS_SSH_USER" \
    --ssh-public-key "file://${SSH_KEY_PATH}.pub" \
    >/dev/null

SSH_OPTS=(-i "$SSH_KEY_PATH" \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -o ConnectTimeout=15)

# ---------- 5. upload + hot swap ------------------------------------------
echo "🚀 [5/5] upload + hot-swap..."
scp "${SSH_OPTS[@]}" "$BIN_PATH" "${AWS_SSH_USER}@${AWS_HOST}:/tmp/zyhive-deploy.new"

# Optional flags-env file: copied as /etc/zyhive/aiteam.env when present.
if [[ -n "${ZYHIVE_FLAGS_ENV:-}" && -f "${ZYHIVE_FLAGS_ENV}" ]]; then
    echo "   uploading flag env file: $ZYHIVE_FLAGS_ENV"
    scp "${SSH_OPTS[@]}" "$ZYHIVE_FLAGS_ENV" "${AWS_SSH_USER}@${AWS_HOST}:/tmp/aiteam.env.new"
fi

ssh "${SSH_OPTS[@]}" "${AWS_SSH_USER}@${AWS_HOST}" "VERSION='${VERSION}' bash -s" <<'REMOTE'
set -euo pipefail
TS=$(date +%Y%m%d-%H%M%S)

# back up current binary
if [[ -x /usr/local/bin/zyhive ]]; then
    sudo cp /usr/local/bin/zyhive "/usr/local/bin/zyhive.bak-${TS}"
fi

# swap in the new one
chmod +x /tmp/zyhive-deploy.new
sudo mv /tmp/zyhive-deploy.new /usr/local/bin/zyhive

# install optional flag env file if uploaded
if [[ -f /tmp/aiteam.env.new ]]; then
    sudo install -d -m 0755 /etc/zyhive
    sudo install -m 0640 /tmp/aiteam.env.new /etc/zyhive/aiteam.env
    rm -f /tmp/aiteam.env.new
fi

sudo systemctl restart zyhive
sleep 3
sudo systemctl is-active zyhive
echo "--- /api/version ---"
curl -sS http://localhost:8080/api/version
echo
echo "--- /api/aiteam/flags ---"
# unauth — depending on auth.token config the response is 401 or 200
curl -sS http://localhost:8080/api/aiteam/flags || true
echo
REMOTE

rm -f "$BIN_PATH"

echo ""
echo "✅ AWS staging deploy complete"
echo "   public:  http://${AWS_HOST}:8080"
echo "   version: ${VERSION}"
