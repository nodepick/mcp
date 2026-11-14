#!/bin/sh
set -eu

INSTALL_PATH="/usr/local/sbin/mcpd"
SERVICE_PATH="/etc/systemd/system/mcpd.service"
BASE_URL="https://github.com/nodepick/mcp/releases/latest/download"

log() {
    echo "[mcpd-install] $*"
}

err() {
    echo "[mcpd-install] ERROR: $*" >&2
}

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

log "mcpd installed and started successfully."
systemctl status mcpd.service --no-pager || true
