#!/bin/sh
set -eu

INSTALL_PATH="/usr/local/sbin/mcpd"
SERVICE_PATH="/etc/systemd/system/mcpd.service"
CADDYFILE_PATH="/etc/caddy/Caddyfile"
BASE_URL="https://github.com/nodepick/mcp/releases/latest/download"
PUBLIC_IP=0

log() {
    echo "[mcpd-install] $*"
}

err() {
    echo "[mcpd-install] ERROR: $*" >&2
}

usage() {
    cat <<'EOF'
Usage: install.sh [--public-ip] [-h|--help]

Options:
  --public-ip   Configure Caddy with on-demand TLS for nodes with a public IP.
                By default, Caddy is configured for plain HTTP on port 8000.
  -h, --help    Show this help message and exit.
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --public-ip)
            PUBLIC_IP=1
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            err "Unknown option: $1"
            usage >&2
            exit 1
            ;;
    esac
    shift
done

if [ "$(id -u)" -ne 0 ]; then
    err "This script must be run as root (e.g., via sudo)."
    exit 1
fi

OS="$(uname -s)"
if [ "$OS" != "Linux" ]; then
    err "Unsupported operating system: $OS (Linux is required)."
    exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
    err "systemctl not found. systemd is required to configure and manage mcpd."
    exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
    err "apt-get not found. Debian/Ubuntu is required to install Caddy."
    exit 1
fi

RAW_ARCH="$(uname -m)"
case "$RAW_ARCH" in
    x86_64|amd64)
        ARCH="x86_64"
        ;;
    aarch64|arm64)
        ARCH="aarch64"
        ;;
    *)
        err "Unsupported architecture: $RAW_ARCH (supported: x86_64, aarch64)"
        exit 1
        ;;
esac

DOWNLOAD_URL="${BASE_URL}/mcpd.${ARCH}"
TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

log "Downloading mcpd (${ARCH}) from ${DOWNLOAD_URL}..."
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP_FILE" "$DOWNLOAD_URL"
else
    err "Neither curl nor wget is available to download mcpd."
    exit 1
fi

if [ ! -s "$TMP_FILE" ]; then
    err "Downloaded binary is empty."
    exit 1
fi

log "Installing mcpd to ${INSTALL_PATH}..."
install -d -m 0755 "$(dirname "$INSTALL_PATH")"
install -m 0755 "$TMP_FILE" "$INSTALL_PATH"

log "Configuring systemd service at ${SERVICE_PATH}..."
cat <<'EOF' > "$SERVICE_PATH"
[Unit]
Description=nodepick.ai MCP Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/sbin/mcpd --foreground --transport http
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$SERVICE_PATH"

log "Reloading systemd and starting mcpd service..."
systemctl daemon-reload
systemctl enable mcpd.service
systemctl restart mcpd.service

log "Installing Caddy from the official APT repository..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl gnupg
install -d -m 0755 /usr/share/keyrings /etc/apt/sources.list.d
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --batch --yes --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' > /etc/apt/sources.list.d/caddy-stable.list
chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg /etc/apt/sources.list.d/caddy-stable.list
apt-get update -qq
apt-get install -y -qq caddy

log "Configuring Caddy at ${CADDYFILE_PATH}..."
install -d -m 0755 "$(dirname "$CADDYFILE_PATH")"
install -d -o caddy -g caddy -m 0755 /var/log/caddy

if [ "$PUBLIC_IP" -eq 1 ]; then
    log "Configuring Caddyfile with on-demand TLS (--public-ip)..."
    cat <<'EOF' > "$CADDYFILE_PATH"
{
    log {
        output file /var/log/caddy/access.log
    }
    auto_https disable_redirects
    on_demand_tls {
        ask https://api.nodepick.ai/api/v1/validate-domain
    }
}

https://:8000 {
    tls {
        on_demand
    }

    reverse_proxy localhost:8444 {
        header_up Host {upstream_hostport}
    }
}
EOF
else
    log "Configuring Caddyfile for plain HTTP (private IP default)..."
    cat <<'EOF' > "$CADDYFILE_PATH"
{
    log {
        output file /var/log/caddy/access.log
    }
}

http://:8000 {
    reverse_proxy localhost:8444 {
        header_up Host {upstream_hostport}
    }
}
EOF
fi
chmod 0644 "$CADDYFILE_PATH"

caddy validate --config "$CADDYFILE_PATH"
chown -R caddy:caddy /var/log/caddy

log "Enabling and starting caddy service..."
systemctl enable caddy.service
systemctl restart caddy.service

log "mcpd and Caddy installed and started successfully."
systemctl status mcpd.service --no-pager || true
systemctl status caddy.service --no-pager || true
