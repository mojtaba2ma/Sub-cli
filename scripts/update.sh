#!/usr/bin/env bash
# =============================================================================
#  update an existing installation from the GitHub repository
#  usage:  sudo bash /opt/sb-ech-gateway/update.sh    (or:  sb-ech update)
# =============================================================================
set -euo pipefail

REPO="${SB_ECH_REPO:-mojtaba2ma/Sub-cli}"
BRANCH="${SB_ECH_BRANCH:-main}"
PREFIX="/opt/sb-ech-gateway"
SERVICE="sb-ech-gateway"

echo "updating sb-ech-gateway from ${REPO}@${BRANCH} ..."
for t in curl tar; do command -v "$t" >/dev/null 2>&1 || { echo "$t is required" >&2; exit 1; }; done

BACKUP=""
[ -f "$PREFIX/config.json" ] && BACKUP="$PREFIX/config.json.bak.$(date +%s)"
[ -n "$BACKUP" ] && cp "$PREFIX/config.json" "$BACKUP" && echo "backup of config: $BACKUP"

TMP="$(mktemp -d /tmp/sb-ech-update.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "https://github.com/${REPO}/archive/refs/heads/${BRANCH}.tar.gz" -o "$TMP/src.tgz"
tar xzf "$TMP/src.tgz" -C "$TMP"
SRC="$(find "$TMP" -mindepth 1 -maxdepth 1 -type d | head -1)"
[ -n "$SRC" ] && [ -f "$SRC/main.go" ] || { echo "unexpected archive layout" >&2; exit 1; }

GO=""
command -v go >/dev/null 2>&1 && GO="$(command -v go)"
[ -z "$GO" ] && [ -x /usr/local/go/bin/go ] && GO=/usr/local/go/bin/go

if [ -n "$GO" ] && [ -f "$SRC/main.go" ]; then
  ( cd "$SRC" && GOCACHE="/tmp/gocache-$$" GOPATH="/tmp/gopath-$$" "$GO" build -trimpath -ldflags "-s -w" -o "$PREFIX/sb-ech-gateway" . )
  echo "rebuilt the gateway from source"
else
  ARCH="$(uname -m)"
  case "$ARCH" in x86_64|amd64) A=amd64 ;; aarch64|arm64) A=arm64 ;; *) echo "unsupported arch $ARCH" >&2; exit 1 ;; esac
  curl -fsSL "https://github.com/${REPO}/releases/latest/download/sb-ech-gateway-linux-${A}" -o "$PREFIX/sb-ech-gateway"
  chmod +x "$PREFIX/sb-ech-gateway"
  echo "downloaded the prebuilt binary"
fi

# refresh the helper script too (it may contain the repo url)
if [ -f "$SRC/install.sh" ]; then
  grep -n "sb-ech" /usr/local/bin/sb-ech >/dev/null 2>&1 || true
fi

systemctl restart "$SERVICE" 2>/dev/null || echo "note: service not restarted (installed with --no-systemd?)"
sleep 1
if systemctl is-active --quiet "$SERVICE" 2>/dev/null; then echo "service is running"; fi
echo "update finished."
