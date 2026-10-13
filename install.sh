#!/usr/bin/env bash
# =============================================================================
#  sb-ech-gateway installer
#  3x-ui (Xray) subscription  ->  sing-box / mihomo clients with working ECH
#
#  Run it with:   sudo bash install.sh
#  Non-interactive flags are listed with:  bash install.sh --help
# =============================================================================
set -uo pipefail

VERSION="2.0.0"
GITHUB_REPO="${SB_ECH_REPO:-mojtaba2ma/Sub-cli}"   # replaced with the real repo

PREFIX="/opt/sb-ech-gateway"
PORT="2199"
PANEL_BASE=""
SUB_PATH="/sub"
PANEL_UA="v2rayNG/1.9.44"
PUBLIC_DOMAIN=""
DO_NGINX=1
DO_SYSTEMD=1
BIN_SOURCE="auto"          # auto | build | release | local
ASSUME_YES=0
UNINSTALL=0
NGINX_SNIPPET=""
CREATED_SECRET=""

# ---- pretty output ----------------------------------------------------------
if [ -t 1 ] && [ "${NO_COLOR:-}" = "" ]; then
  C_R="$(printf '\033[0m')"; C_B="$(printf '\033[1;36m')"; C_G="$(printf '\033[1;32m')"
  C_Y="$(printf '\033[1;33m')"; C_E="$(printf '\033[1;31m')"; C_D="$(printf '\033[2m')"
else
  C_R=""; C_B=""; C_G=""; C_Y=""; C_E=""; C_D=""
fi
say()  { printf '%s\n' "${C_B}== $*${C_R}"; }
ok()   { printf '%s\n' "  ${C_G}OK${C_R}   $*"; }
warn() { printf '%s\n' "  ${C_Y}WARN${C_R} $*"; }
err()  { printf '%s\n' "  ${C_E}ERR${C_R}  $*" >&2; }
note() { printf '%s\n' "  ${C_D}$*${C_R}"; }
die()  { err "$*"; exit 1; }

banner() {
  printf '%s\n' "${C_B}"
  cat <<'EOF'
   ____  ____        _____ ____ _   __   _____
  / __ \| __ )      | ____/ ___| | | |  / ____|_      _____  __ _ _   _
  \__ \|  _ \ _____ |  _|| |   | |_| | | |  _| \ \ /\ / / _ \/ _` | | | |
  (__) | |_) |_____|| |__| |___|  _  | | |_| |  \ V  V /  __/ (_| | |_| |
  |___/|____/       |_____\____|_| |_|  \_____|   \_/\_/ \___|\__, |\__, |
                                                               |___/ |___/
EOF
  printf '%s' "${C_R}"
  printf '%s\n' "  ${C_D}sing-box / mihomo subscription gateway with live Cloudflare ECH - v${VERSION}${C_R}"
  echo
}

usage() {
  cat <<EOF
usage: sudo bash install.sh [options]

  --prefix DIR        install directory            (default: ${PREFIX})
  --port N            local listening port         (default: ${PORT})
  --panel URL         panel base url               (default: auto-detect, else ask)
  --sub-path PATH     panel subscription path      (default: ${SUB_PATH})
  --panel-ua UA       user-agent used to fetch the panel sub (default: ${PANEL_UA})
  --domain NAME       public domain that fronts the panel (for nginx + printed links)
  --no-nginx          do not touch nginx
  --no-systemd        do not install/start the systemd service
  --bin-source SRC    auto|build|release|local     (default: auto)
  --yes               accept defaults, no questions
  --uninstall         remove the service and installation
  -h, --help          this help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --panel) PANEL_BASE="$2"; shift 2 ;;
    --sub-path) SUB_PATH="$2"; shift 2 ;;
    --panel-ua) PANEL_UA="$2"; shift 2 ;;
    --domain) PUBLIC_DOMAIN="$2"; shift 2 ;;
    --no-nginx) DO_NGINX=0; shift ;;
    --no-systemd) DO_SYSTEMD=0; shift ;;
    --bin-source) BIN_SOURCE="$2"; shift 2 ;;
    --yes|-y) ASSUME_YES=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  die "please run as root:  sudo bash install.sh"
fi
if ! [ -t 0 ] && [ "$ASSUME_YES" -ne 1 ] && [ -z "$PANEL_BASE" ]; then
  warn "stdin is not a terminal; run with --panel URL --yes for unattended installs"
fi

ask() { # ask <var> <question> <default>
  local __var="$1" __q="$2" __def="$3" __ans=""
  if [ "$ASSUME_YES" -eq 1 ]; then printf -v "$__var" '%s' "$__def"; return; fi
  printf '%s' "  ${C_B}?${C_R} ${__q}"
  [ -n "$__def" ] && printf ' %s' "${C_D}[${__def}]${C_R}"
  printf ': '
  read -r __ans || true
  [ -z "$__ans" ] && __ans="$__def"
  printf -v "$__var" '%s' "$__ans"
}

ask_yesno() { # ask_yesno <question> <default: y|n>
  local __q="$1" __def="$2" __ans=""
  if [ "$ASSUME_YES" -eq 1 ]; then [ "$__def" = "y" ]; return; fi
  printf '%s' "  ${C_B}?${C_R} ${__q} [${__def}]: "
  read -r __ans || true
  [ -z "$__ans" ] && __ans="$__def"
  case "$__ans" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
  else
    od -An -N16 -tx1 /dev/urandom | tr -d ' \n'
  fi
}

# ---- prerequisites ----------------------------------------------------------
ensure_tools() {
  local missing=""
  for t in curl tar; do command -v "$t" >/dev/null 2>&1 || missing="$missing $t"; done
  if [ -n "$missing" ]; then
    say "installing prerequisites:${missing}"
    if command -v apt-get >/dev/null 2>&1; then
      DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $missing libcap2-bin
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y -q $missing libcap
    elif command -v yum >/dev/null 2>&1; then
      yum install -y -q $missing libcap
    elif command -v apk >/dev/null 2>&1; then
      apk add --no-cache $missing libcap
    else
      die "please install manually:$missing"
    fi
  fi
}

# ---- uninstall --------------------------------------------------------------
do_uninstall() {
  banner
  say "uninstalling"
  if [ "$DO_SYSTEMD" -eq 1 ]; then
    systemctl disable --now sb-ech-gateway 2>/dev/null && ok "service stopped" || warn "service was not installed"
    rm -f /etc/systemd/system/sb-ech-gateway.service
    systemctl daemon-reload 2>/dev/null || true
  fi
  if [ -n "$NGINX_SNIPPET" ] && [ -f "$NGINX_SNIPPET" ]; then
    rm -f "$NGINX_SNIPPET" && ok "removed $NGINX_SNIPPET"
    note "make sure to delete the include line from your nginx server block and run: nginx -t && systemctl reload nginx"
  fi
  rm -rf "$PREFIX" && ok "removed $PREFIX"
  rm -f /usr/local/bin/sb-ech
  say "done"
  exit 0
}

banner
[ "$UNINSTALL" -eq 1 ] && do_uninstall

# ---- 1. panel -----------------------------------------------------------------
say "step 1/6 - panel subscription"
PANEL_DEFAULT=""
for cand in "http://127.0.0.1:2096" "http://127.0.0.1:2053" "http://127.0.0.1:2096"; do
  if curl -fsS -m 3 -o /dev/null "$cand" 2>/dev/null; then PANEL_DEFAULT="$cand"; break; fi
done
if [ -z "$PANEL_BASE" ]; then
  ask PANEL_BASE "panel base URL (where your 3x-ui subscription lives)" "${PANEL_DEFAULT:-http://127.0.0.1:2096}"
fi

if [ "$ASSUME_YES" -ne 1 ]; then
  note "tip: in the panel, Settings -> Subscription shows the exact sub path (subPath)."
  ask SUB_PATH "panel subscription path" "$SUB_PATH"
fi
SUB_URL="${PANEL_BASE%/}${SUB_PATH%/}/{subid}"
echo
note "gateway will fetch:  curl -A '${PANEL_UA}' ${PANEL_BASE%/}${SUB_PATH%/}/<SUBID>"
TMP_SUBID="$(random_secret | cut -c1-12)"
if command -v curl >/dev/null 2>&1; then
  PROBE_FILE=/tmp/.sb_probe.$$
  if curl -fsS -m 8 -A "$PANEL_UA" "${PANEL_BASE%/}${SUB_PATH%/}/${TMP_SUBID}" -o "$PROBE_FILE" 2>/dev/null; then
    if grep -q "://" "$PROBE_FILE" 2>/dev/null; then
      ok "panel answered with node links (plain)"
    elif command -v base64 >/dev/null 2>&1 && base64 -d < "$PROBE_FILE" 2>/dev/null | grep -q "://"; then
      ok "panel answered with node links (base64 subscription)"
    else
      warn "panel answered but the body has no share links - check the sub path"
      note "the gateway reads the panel's plain/base64 subscription; a random test id may also be refused"
    fi
  else
    warn "could not fetch a test subscription from the panel (HTTP error)"
    note "this is normal for a random test id - or fix the URL/path and run install again"
  fi
  rm -f "$PROBE_FILE"
fi

# ---- 2. public address ---------------------------------------------------------
say "step 2/6 - public address"
if [ "$DO_NGINX" -eq 1 ] && [ "$ASSUME_YES" -ne 1 ]; then
  note "the gateway is published through nginx, next to your panel subscription."
  ask PUBLIC_DOMAIN "public domain of your panel (leave empty to skip nginx)" "$PUBLIC_DOMAIN"
  [ -z "$PUBLIC_DOMAIN" ] && DO_NGINX=0
fi

# ---- 3. secret -----------------------------------------------------------------
say "step 3/6 - secret"
CREATED_SECRET="$(random_secret)"
ok "generated a random secret (shown again at the end)"
note "the secret is part of every subscription URL - keep the URLs private"

# ---- 4. install the binary -----------------------------------------------------
say "step 4/6 - installing the gateway"
mkdir -p "$PREFIX"
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$PREFIX/sb-ech-gateway"

build_with_go() {
  local gobin="" cache=""
  if command -v go >/dev/null 2>&1; then gobin="$(command -v go)"; fi
  if [ -z "$gobin" ] && [ -x /usr/local/go/bin/go ]; then gobin=/usr/local/go/bin/go; fi
  if [ -z "$gobin" ]; then
    local arch goarch tmpdir
    arch="$(uname -m)"
    case "$arch" in x86_64|amd64) goarch=amd64 ;; aarch64|arm64) goarch=arm64 ;; *) return 1 ;; esac
    tmpdir="${TMPDIR:-/tmp}"
    say "  downloading the Go toolchain (one time, ~80 MB)"
    curl -fsSL "https://go.dev/dl/go1.24.5.linux-${goarch}.tar.gz" -o "$tmpdir/go.tgz" || return 1
    tar -C /usr/local -xzf "$tmpdir/go.tgz" && rm -f "$tmpdir/go.tgz" || return 1
    gobin=/usr/local/go/bin/go
  fi
  # keep the build cache out of a full /tmp: prefer the install prefix
  if [ -w "$PREFIX" ] && [ "$(df -Pk "$PREFIX" 2>/dev/null | awk 'NR==2{print $4}')" -gt 800000 ] 2>/dev/null; then
    cache="$PREFIX/.gocache"
  else
    cache="$(mktemp -d "${TMPDIR:-/tmp}/sb-ech-build.XXXXXX")"
  fi
  mkdir -p "$cache"
  ( cd "$SRC_DIR" && GOCACHE="$cache/cache" GOPATH="$cache/gopath" "$gobin" build -trimpath -ldflags "-s -w" -o "$BIN" . )
  local rc=$?
  case "$cache" in "$PREFIX/.gocache") : ;; *) rm -rf "$cache" ;; esac
  [ $rc -eq 0 ] || return 1
  ok "built from source with $("$gobin" version | awk '{print $3}')"
  return 0
}

download_release() {
  local arch asset url
  arch="$(uname -m)"
  case "$arch" in x86_64|amd64) asset=sb-ech-gateway-linux-amd64 ;; aarch64|arm64) asset=sb-ech-gateway-linux-arm64 ;; *) return 1 ;; esac
  url="https://github.com/${GITHUB_REPO}/releases/latest/download/${asset}"
  curl -fsSL "$url" -o "$BIN" || return 1
  chmod +x "$BIN"
  ok "downloaded the prebuilt binary"
  return 0
}

installed=0
case "$BIN_SOURCE" in
  local)
    [ -f "$SRC_DIR/sb-ech-gateway" ] || die "no prebuilt ./sb-ech-gateway in $SRC_DIR"
    install -m 0755 "$SRC_DIR/sb-ech-gateway" "$BIN"; installed=1; ok "installed local binary" ;;
  build)
    build_with_go || die "build failed"; installed=1 ;;
  release)
    download_release || die "release download failed"; installed=1 ;;
  auto)
    if [ -f "$SRC_DIR/main.go" ] && { command -v go >/dev/null 2>&1 || [ -x /usr/local/go/bin/go ]; }; then
      build_with_go && installed=1
    fi
    if [ "$installed" -eq 0 ] && [ -f "$SRC_DIR/main.go" ]; then
      build_with_go && installed=1
    fi
    if [ "$installed" -eq 0 ]; then
      download_release && installed=1
    fi
    if [ "$installed" -eq 0 ] && [ -f "$SRC_DIR/main.go" ]; then
      build_with_go && installed=1
    fi
    [ "$installed" -eq 1 ] || die "could not obtain the gateway binary"
    ;;
esac

rm -rf "$PREFIX/.gocache" 2>/dev/null || true

# ---- 5. config + service -------------------------------------------------------
say "step 5/6 - configuration"

cat > "$PREFIX/config.json" <<EOF
{
  "listen": "127.0.0.1",
  "port": ${PORT},
  "base_path": "/sb",
  "secret": "${CREATED_SECRET}",

  "panel": {
    "sub_url": "${SUB_URL}",
    "user_agent": "${PANEL_UA}",
    "timeout_seconds": 15,
    "insecure_tls": false,
    "host_header": ""
  },

  "ech": {
    "enabled": true,
    "domain": "cloudflare-ech.com",
    "resolvers": ["https://8.8.8.8/dns-query", "https://1.1.1.1/dns-query"],
    "mode": "all",
    "domains": [],
    "cache_seconds": 300,
    "static_endpoint": true
  },

  "profile": {
    "log_level": "warn",
    "dns_direct": ["8.8.8.8", "1.1.1.1"],
    "dns_ech": "1.1.1.1",
    "strategy": "prefer_ipv4",
    "fingerprint": "chrome",
    "include_tun": true,
    "mixed_port": 2080,
    "urltest_url": "http://cp.cloudflare.com/generate_204",
    "urltest_interval": "3m"
  },

  "response": {
    "profile_update_interval_hours": 6,
    "profile_title": ""
  }
}
EOF
chmod 600 "$PREFIX/config.json"
ok "wrote $PREFIX/config.json (mode 600)"

HARDEN="PrivateTmp=true
ProtectHome=true"
case "$PREFIX" in
  /tmp/*|/var/tmp/*|/home/*|/root/*)
    HARDEN="# installed under $PREFIX - namespace hardening disabled so systemd can see the files"
    warn "install prefix is inside a temporary/home path; private-tmp hardening disabled"
    ;;
esac

cat > /etc/systemd/system/sb-ech-gateway.service <<EOF
[Unit]
Description=3x-ui -> sing-box/mihomo subscription gateway with ECH
After=network-online.target x-ui.service
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${PREFIX}
ExecStart=${BIN} --config ${PREFIX}/config.json
Restart=always
RestartSec=3
NoNewPrivileges=true
ProtectSystem=full
${HARDEN}
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF
ok "wrote /etc/systemd/system/sb-ech-gateway.service"

# small management helper
cat > /usr/local/bin/sb-ech <<'HELPER'
#!/usr/bin/env bash
# sb-ech - manage the sb-ech-gateway service
PREFIX="__PREFIX__"
CONF="$PREFIX/config.json"
[ -f "$CONF" ] || { echo "not installed"; exit 1; }
case "${1:-url}" in
  url|urls)
    SECRET="$(grep -o '"secret"[^,]*' "$CONF" | cut -d'"' -f4)"
    PORT="$(grep -o '"port"[^,]*' "$CONF" | cut -d: -f2 | tr -d ' ,')"
    echo "local:  http://127.0.0.1:${PORT}/sb/${SECRET}/auto/<SUBID>"
    echo "secret: ${SECRET}"
    ;;
  secret) grep -o '"secret"[^,]*' "$CONF" | cut -d'"' -f4 ;;
  status) systemctl --no-pager -l status sb-ech-gateway | head -20 ;;
  logs) journalctl -u sb-ech-gateway -n 60 --no-pager ;;
  restart) systemctl restart sb-ech-gateway && echo restarted ;;
  health) curl -s "http://127.0.0.1:$(grep -o '"port"[^,]*' "$CONF" | cut -d: -f2 | tr -d ' ,')/sb/$(grep -o '"secret"[^,]*' "$CONF" | cut -d'"' -f4)/health" ;;
  edit) ${EDITOR:-nano} "$CONF" && systemctl restart sb-ech-gateway ;;
  update) bash -c 'curl -fsSL https://raw.githubusercontent.com/__REPO__/main/scripts/update.sh | bash' ;;
  *) echo "usage: sb-ech [url|secret|status|logs|restart|health|edit|update]"; ;;
esac
HELPER
sed -i "s|__REPO__|${GITHUB_REPO}|; s|__PREFIX__|${PREFIX}|" /usr/local/bin/sb-ech
chmod +x /usr/local/bin/sb-ech
ok "installed helper command: sb-ech"

if [ -f "$SRC_DIR/scripts/test-on-server.sh" ]; then
  install -m 0755 "$SRC_DIR/scripts/test-on-server.sh" "$PREFIX/test-on-server.sh"
  ok "installed the end-to-end ECH test: ${PREFIX}/test-on-server.sh"
fi

if [ "$DO_SYSTEMD" -eq 1 ]; then
  systemctl daemon-reload
  systemctl enable sb-ech-gateway >/dev/null 2>&1
  systemctl restart sb-ech-gateway >/dev/null 2>&1 || systemctl start sb-ech-gateway >/dev/null 2>&1
  sleep 1
  if systemctl is-active --quiet sb-ech-gateway; then
    ok "service is running"
  else
    err "service failed to start - recent logs:"
    journalctl -u sb-ech-gateway -n 15 --no-pager || true
    die "fix the config ($PREFIX/config.json) and run: systemctl restart sb-ech-gateway"
  fi
else
  warn "skipping systemd (--no-systemd); start manually: ${BIN} --config ${PREFIX}/config.json"
fi

HEALTH="$(curl -fsS -m 8 "http://127.0.0.1:${PORT}/sb/${CREATED_SECRET}/health" 2>/dev/null || true)"
if printf '%s' "$HEALTH" | grep -q '"ok": true'; then
  ok "health check passed (live Cloudflare ECH config fetched)"
else
  warn "health check returned: ${HEALTH:-no response}"
  note "the gateway still works if ECH lookup fails later - check: sb-ech health"
fi

# ---- 6. nginx ------------------------------------------------------------------
say "step 6/6 - nginx"
NGINX_CONF_FILE="/etc/nginx/conf.d/sb-ech-gateway.conf"
NGINX_HTTP=0
if [ "$DO_NGINX" -eq 1 ] && command -v nginx >/dev/null 2>&1; then
  SERVER_NAME="${PUBLIC_DOMAIN:-_}"
  cat > "$NGINX_CONF_FILE" <<EOF
# sb-ech-gateway: sing-box / mihomo subscriptions with live ECH
server {
    listen 80;
    listen [::]:80;
    server_name ${SERVER_NAME};

    location /sb/ {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 30s;
        proxy_buffering off;
        add_header Cache-Control "no-store" always;
    }
}
EOF
  NGINX_HTTP=1
  ok "wrote $NGINX_CONF_FILE (plain HTTP on port 80)"

  # try to include it into the HTTPS (443) server block of the panel domain
  if [ -n "$PUBLIC_DOMAIN" ]; then
    mapfile -t CANDIDATES < <(grep -rl "server_name.*${PUBLIC_DOMAIN}" /etc/nginx 2>/dev/null || true)
    TARGET=""
    for f in "${CANDIDATES[@]:-}"; do
      [ -z "$f" ] && continue
      if grep -q "listen.*443" "$f"; then TARGET="$f"; fi
    done
    if [ -n "$TARGET" ]; then
      if grep -q "sb-ech-gateway" "$TARGET"; then
        ok "nginx already includes the gateway in $TARGET"
      else
        BACKUP="$TARGET.sb-ech-backup.$(date +%s)"
        cp "$TARGET" "$BACKUP"
        SNIPPET_FILE="/etc/nginx/snippets/sb-ech-gateway-location.conf"
        mkdir -p /etc/nginx/snippets
        cat > "$SNIPPET_FILE" <<EOF
location /sb/ {
    proxy_pass http://127.0.0.1:${PORT};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_read_timeout 30s;
    proxy_buffering off;
    add_header Cache-Control "no-store" always;
}
EOF
        awk -v inc="$SNIPPET_FILE" '
          { print }
          /server[ \t]*\{/ && !done { depth=1; print "    include " inc ";"; done=1 }
        ' "$TARGET" > "$TARGET.new" && mv "$TARGET.new" "$TARGET"
        if nginx -t >/dev/null 2>&1; then
          systemctl is-active --quiet nginx 2>/dev/null && systemctl reload nginx || systemctl start nginx >/dev/null 2>&1 || true
          ok "nginx includes the gateway in $TARGET (reloaded)"
          NGINX_SNIPPET="$SNIPPET_FILE"
        else
          cp "$BACKUP" "$TARGET"
          warn "nginx config test failed - restored $TARGET from backup"
          note "add this include manually inside your 443 server block:  include ${SNIPPET_FILE};"
        fi
      fi
    else
      warn "could not find your HTTPS server block for '${PUBLIC_DOMAIN}' in /etc/nginx"
      note "add this line manually inside the 443 server block of your panel domain:"
      printf '%s\n' "        include ${NGINX_HTTP:+}"
      note "location /sb/ { proxy_pass http://127.0.0.1:${PORT}; proxy_set_header Host \$host; }"
    fi
  fi

  if nginx -t >/dev/null 2>&1; then
    if systemctl is-active --quiet nginx 2>/dev/null; then
      systemctl reload nginx 2>/dev/null && ok "nginx reloaded"
    else
      systemctl start nginx >/dev/null 2>&1 && ok "nginx started" || warn "nginx is not running - start it manually: systemctl start nginx"
    fi
  else
    warn "nginx -t reports an error - check: nginx -t"
  fi
else
  if [ "$DO_NGINX" -eq 1 ]; then
    warn "nginx was not found on this server"
  else
    note "nginx step skipped"
  fi
  note "the gateway itself only listens on 127.0.0.1:${PORT}"
fi

# ---- summary --------------------------------------------------------------------
SCHEME="https"; PORT_PART=""
if [ "$NGINX_HTTP" -eq 1 ] && [ -z "$PUBLIC_DOMAIN" ]; then SCHEME="http"; fi
HOST="${PUBLIC_DOMAIN:-$(hostname -f 2>/dev/null || hostname)}"
BASE="${SCHEME}://${HOST}/sb/${CREATED_SECRET}"

echo
printf '%s\n' "${C_G}===============================================================${C_R}"
printf '%s\n' "${C_G}  INSTALL COMPLETE  -  save the secret and the URLs now${C_R}"
printf '%s\n' "${C_G}===============================================================${C_R}"
echo
printf '%s\n' "  ${C_B}SECRET${C_R}  (keep it private - it is part of every URL):"
printf '%s\n' "      ${CREATED_SECRET}"
echo
printf '%s\n' "  ${C_B}Subscription URLs${C_R} (replace <SUBID> with each user's Sub ID):"
printf '%s\n' "      ${BASE}/auto/<SUBID>     ${C_D}<- one link for every client (recommended)${C_R}"
printf '%s\n' "      ${BASE}/sub/<SUBID>      ${C_D}<- sing-box, live ECH from DNS${C_R}"
printf '%s\n' "      ${BASE}/subs/<SUBID>     ${C_D}<- sing-box, static ECH (Hiddify)${C_R}"
printf '%s\n' "      ${BASE}/clash/<SUBID>    ${C_D}<- Clash / Mihomo${C_R}"
printf '%s\n' "      ${BASE}/xray/<SUBID>     ${C_D}<- raw panel links (v2rayNG & co)${C_R}"
echo
printf '%s\n' "  ${C_B}Useful commands${C_R}:"
printf '%s\n' "      sb-ech url        show URLs and the secret"
printf '%s\n' "      sb-ech health     ECH status + last key fetch time"
printf '%s\n' "      sb-ech logs       service logs"
printf '%s\n' "      sb-ech edit       edit config.json and restart"
echo
printf '%s\n' "  ${C_Y}Next${C_R}:"
printf '%s\n' "   1. paste one URL above into your sing-box client (sing-box, Hiddify, NekoBox, Karing...)"
printf '%s\n' "   2. test the tunnel:  ${PREFIX}/test-on-server.sh --sub '<panel sub url>'"
printf '%s\n' "   3. Xray users (v2rayNG, Happ) can keep using the panel's own subscription - nothing changed"
echo
