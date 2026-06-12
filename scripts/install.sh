#!/usr/bin/env bash
set -euo pipefail

# ──────────────────────────────────────────────
# TailChat — One-shot server provisioning script
# Target: Ubuntu 24.04+ headless server
# ──────────────────────────────────────────────

TS_AUTH_KEY="${TS_AUTH_KEY:-}"
GIT_REPO="${GIT_REPO:-}"
TAILNET_DOMAIN="${TAILNET_DOMAIN:-}"
VERSION="1.0.0"

echo "=== TailChat Server Installer v${VERSION} ==="

# ── Step 1: Pre-flight checks ──────────────────
if [[ $EUID -ne 0 ]]; then
  echo "ERROR: Must run as root (use sudo)." >&2
  exit 1
fi

if [[ ! -f /etc/os-release ]]; then
  echo "ERROR: Cannot detect OS." >&2
  exit 1
fi
# shellcheck source=/dev/null
. /etc/os-release
ubuntu_ver="${VERSION_ID:-0}"
if [[ "${ID:-}" != "ubuntu" ]] || [[ "${ubuntu_ver%%.*}" -lt 24 ]]; then
  echo "ERROR: Requires Ubuntu 24.04+. Detected: ${ID:-unknown} ${ubuntu_ver}" >&2
  exit 1
fi

for cmd in curl git; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "Installing missing dependency: $cmd"
    apt update && apt install -y "$cmd"
  fi
done

# ── Step 2: Install system dependencies ────────
echo "=== Installing system packages ==="
apt update
apt install -y build-essential sqlite3 curl git ufw

# ── Step 3: Install Go ─────────────────────────
echo "=== Installing Go ==="
GO_MIN_MAJOR=1
GO_MIN_MINOR=23
if ! command -v go &>/dev/null; then
  need_go=true
else
  GO_CUR=$(go version | grep -oP 'go\d+\.\d+' | head -1)
  GO_MAJOR=$(echo "$GO_CUR" | cut -d. -f1 | tr -d 'go')
  GO_MINOR=$(echo "$GO_CUR" | cut -d. -f2)
  if [[ "$GO_MAJOR" -gt "$GO_MIN_MAJOR" ]] || { [[ "$GO_MAJOR" -eq "$GO_MIN_MAJOR" ]] && [[ "$GO_MINOR" -ge "$GO_MIN_MINOR" ]]; }; then
    need_go=false
  else
    need_go=true
  fi
fi
if [[ "${need_go:-true}" == "true" ]]; then
  # Use GO_VERSION env var or fetch latest Go 1.26.x dynamically
  if [[ -n "${GO_VERSION:-}" ]]; then
    GO_VER="$GO_VERSION"
  else
    GO_VER=$(curl -s https://go.dev/dl/?mode=json | grep -o '"version":"go[0-9.]*"' | head -1 | grep -o '[0-9.]*' | head -1)
    if [[ -z "$GO_VER" ]]; then
      GO_VER="1.26.4" # fallback
    fi
  fi
  echo "Downloading Go ${GO_VER}..."
  curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
  tar -C /usr/local -xzf /tmp/go.tar.gz
  rm -f /tmp/go.tar.gz
  cat > /etc/profile.d/go.sh <<'GOEOF'
export PATH=$PATH:/usr/local/go/bin
GOEOF
  chmod 755 /etc/profile.d/go.sh
  export PATH="$PATH:/usr/local/go/bin"
fi
echo "Go version: $(go version)"

# ── Step 4: Install Node.js 22 LTS ─────────────
echo "=== Installing Node.js 22 LTS ==="
if ! command -v node &>/dev/null || [[ "$(node --version)" != v22* ]]; then
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
  apt install -y nodejs
fi
echo "Node version: $(node --version)"
echo "npm version: $(npm --version)"

# ── Step 5: Install Tailscale ──────────────────
echo "=== Installing Tailscale ==="
if ! command -v tailscale &>/dev/null; then
  curl -fsSL https://tailscale.com/install.sh | sh
fi

if [[ -n "$TS_AUTH_KEY" ]]; then
  tailscale up --auth-key="$TS_AUTH_KEY" --hostname=tailchat-server
else
  echo "WARNING: No TS_AUTH_KEY set. Starting interactive login..."
  tailscale up --hostname=tailchat-server
fi

TAILNET_HOSTNAME="tailchat-server.${TAILNET_DOMAIN:-$(tailscale status --json 2>/dev/null | grep -o '"Domain":"[^"]*"' | cut -d'"' -f4)}"
tailscale serve --bg --https 443 localhost:3000

# ── Step 6: Configure firewall ─────────────────
echo "=== Configuring UFW ==="
ufw allow in on tailscale0
ufw default deny incoming
ufw --force enable

# ── Step 7: Create system user ─────────────────
echo "=== Creating tailchat system user ==="
id -u tailchat &>/dev/null || \
  useradd --system --no-create-home --shell /usr/sbin/nologin tailchat

# ── Step 8: Create directory structure ─────────
echo "=== Creating directory structure ==="
mkdir -p /opt/tailchat/{server,web,scripts,data/blobs}
chmod 700 /opt/tailchat/data/blobs

# ── Step 9: Clone or copy application files ────
echo "=== Acquiring application files ==="
if [[ -n "$GIT_REPO" ]]; then
  if [[ -d /opt/tailchat/server-src/.git ]]; then
    cd /opt/tailchat/server-src && git pull
  else
    git clone "$GIT_REPO" /opt/tailchat/server-src
  fi
  rsync -a /opt/tailchat/server-src/ /opt/tailchat/ --exclude=data --exclude=config.yaml
else
  echo "============================================"
  echo " GIT_REPO not set. Manually copy files:"
  echo "   scp -r ./tailchat/* root@<server>:/opt/tailchat/"
  echo " Then re-run this script."
  echo "============================================"
fi

chown -R tailchat:tailchat /opt/tailchat

# ── Step 10: Initial build ─────────────────────
echo "=== Building frontend ==="
if [[ -f /opt/tailchat/web/package.json ]]; then
  cd /opt/tailchat/web
  if [[ -f package-lock.json ]]; then
    npm ci
  else
    npm install
  fi
  npm run build
else
  echo "WARNING: web/ not found — skipping frontend build"
fi

echo "=== Building backend ==="
if [[ -f /opt/tailchat/server/go.mod ]]; then
  cd /opt/tailchat/server
  go build -o /opt/tailchat/tailchat-server .
  /opt/tailchat/tailchat-server --version
else
  echo "WARNING: server/ not found — skipping backend build"
fi

# ── Step 11: Install systemd service ───────────
echo "=== Installing systemd service ==="
if [[ -f /opt/tailchat/scripts/tailchat.service ]]; then
  cp /opt/tailchat/scripts/tailchat.service /etc/systemd/system/tailchat.service
else
  echo "ERROR: scripts/tailchat.service not found" >&2
  exit 1
fi

# Copy config if present
if [[ -f /opt/tailchat/config.yaml ]]; then
  echo "Config found at /opt/tailchat/config.yaml"
fi

# ── Step 12: Start service ─────────────────────
echo "=== Starting TailChat service ==="
systemctl daemon-reload
systemctl enable tailchat
systemctl start tailchat

echo "=== Health check ==="
sleep 2
if curl -s http://localhost:3000/api/health; then
  echo ""
  echo "Health check passed."
else
  echo "WARNING: Health check failed — check logs: journalctl -u tailchat -f" >&2
fi

# ── Step 13: Success summary ───────────────────
echo ""
echo "============================================"
echo " TailChat Server Installation Complete!"
echo "============================================"
echo " Tailnet hostname: ${TAILNET_HOSTNAME:-tailchat-server.<your-tailnet>.ts.net}"
echo " Service status:   systemctl status tailchat"
echo " Logs:             journalctl -u tailchat -f"
echo " Config:           /opt/tailchat/config.yaml"
echo " Database:         /opt/tailchat/data/tailchat.db"
echo "============================================"
