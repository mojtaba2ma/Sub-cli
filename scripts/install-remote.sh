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

# GitHub names the archive root "<repo>-<branch>" (e.g. Sub-cli-main), so accept any
# single top-level directory instead of assuming the project name.
DIR="$(find "$TMP" -mindepth 1 -maxdepth 1 -type d | head -1)"
[ -n "$DIR" ] && [ -f "$DIR/install.sh" ] || { echo "unexpected archive layout" >&2; exit 1; }

cd "$DIR"

# `curl | bash` runs this script from the pipe, so install.sh would inherit a
# non-interactive stdin and could not ask anything. Re-attach the terminal.
# `-r /dev/tty` only checks file permissions; a machine without a controlling
# terminal fails to *open* it, which used to kill the run before install.sh even
# started (and made curl exit 23 on the closed pipe). Try the open for real.
if [ -t 0 ]; then
  bash install.sh "$@"
elif { true </dev/tty; } 2>/dev/null; then
  bash install.sh "$@" </dev/tty
else
  echo "note: no terminal found - running unattended, so --domain is required" >&2
  bash install.sh "$@"
fi
