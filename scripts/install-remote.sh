#!/usr/bin/env bash
# =============================================================================
#  one-line remote installer
#
#  usage (on the server):
#     curl -fsSL https://raw.githubusercontent.com/mojtaba2ma/Sub-cli/main/scripts/install-remote.sh | sudo bash
#
#  pass installer flags after `-s --`, for example:
#     curl -fsSL .../install-remote.sh | sudo bash -s -- --domain panel.example.com --yes
# =============================================================================
set -euo pipefail

REPO="${SB_ECH_REPO:-mojtaba2ma/Sub-cli}"
BRANCH="${SB_ECH_BRANCH:-main}"

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  echo "please run as root (sudo bash)" >&2
  exit 1
fi
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }

TMP="$(mktemp -d /tmp/sb-ech-install.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

echo "downloading ${REPO}@${BRANCH} ..."
curl -fsSL "https://github.com/${REPO}/archive/refs/heads/${BRANCH}.tar.gz" -o "$TMP/src.tgz"
tar xzf "$TMP/src.tgz" -C "$TMP"

DIR="$(find "$TMP" -maxdepth 1 -type d -name 'sb-ech-gateway-*' | head -1)"
[ -n "$DIR" ] || { echo "unexpected archive layout" >&2; exit 1; }

cd "$DIR"
bash install.sh "$@"
