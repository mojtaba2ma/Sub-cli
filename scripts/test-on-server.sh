#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# test-on-server.sh - real-environment ECH test for a 3x-ui node, using the
# sing-box core itself (the same engine Hiddify / NekoBox / Karing / the
# official sing-box apps use).
#
# It answers three questions with evidence:
#   1. does sing-box fetch the live Cloudflare ECHConfigList from DNS?
#   2. does the TLS+ECH handshake to your node succeed?
#   3. does traffic actually exit from your server's IP?
#
# Usage
#   ./test-on-server.sh --link 'vless://....'          # one share link
#   ./test-on-server.sh --sub  'http://127.0.0.1:2096/sub/SUBID'
#   ./test-on-server.sh --sub  'https://panel.example.com:2096/sub/SUBID'
#   ./test-on-server.sh --link '...' --ech static      # force static ECH PEM
#   ./test-on-server.sh --link '...' --singbox /usr/local/bin/sing-box
#
# Options
#   --ech dynamic|static|off   default: dynamic
#   --port N                   local socks/mixed port (default 2080)
#   --singbox PATH             use an existing sing-box binary
#   --gateway PATH             path to the sb-ech-gateway binary
#   --config PATH              path to config.json (default: next to the binary)
#   --keep                     leave sing-box running at the end
# ---------------------------------------------------------------------------
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# the gateway binary sits next to this script after an install (see install.sh);
# override with --gateway PATH when running straight from the repository.
GATEWAY=""
SB_VERSION="${SB_VERSION:-1.14.3}"
CONFIG=""

LINK=""
SUB=""
ECH_MODE="dynamic"
PORT=2080
SB_BIN=""
KEEP=0
TMP=""

die() { echo "ERROR: $*" >&2; exit 1; }
say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --link) LINK="$2"; shift 2 ;;
    --sub)  SUB="$2";  shift 2 ;;
    --ech)  ECH_MODE="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --singbox) SB_BIN="$2"; shift 2 ;;
    --gateway) GATEWAY="$2"; shift 2 ;;
    --config)  CONFIG="$2";  shift 2 ;;
    --keep) KEEP=1; shift ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) die "unknown option $1" ;;
  esac
done

[ -n "$LINK" ] || [ -n "$SUB" ] || die "pass --link or --sub"

if [ -z "$GATEWAY" ]; then
  for cand in "$SCRIPT_DIR/sb-ech-gateway" "$SCRIPT_DIR/../sb-ech-gateway" /opt/sb-ech-gateway/sb-ech-gateway "$(command -v sb-ech-gateway || true)"; do
    [ -n "$cand" ] && [ -x "$cand" ] && { GATEWAY="$cand"; break; }
  done
fi
[ -n "$GATEWAY" ] && [ -x "$GATEWAY" ] || die "cannot find the sb-ech-gateway binary - pass --gateway PATH"
if [ -z "$CONFIG" ]; then
  for cand in "$(dirname "$GATEWAY")/config.json" /opt/sb-ech-gateway/config.json; do
    [ -f "$cand" ] && { CONFIG="$cand"; break; }
  done
fi
[ -n "$CONFIG" ] && [ -f "$CONFIG" ] || die "cannot find config.json - pass --config PATH"

TMP="$(mktemp -d /tmp/sb-ech-test.XXXXXX)"
trap '[ "$KEEP" = 1 ] || { [ -n "${SB_PID:-}" ] && kill "$SB_PID" 2>/dev/null || true; }; rm -rf "$TMP" 2>/dev/null || true' EXIT

# --- 1. sing-box binary -----------------------------------------------------
if [ -z "$SB_BIN" ]; then
  if command -v sing-box >/dev/null 2>&1; then
    SB_BIN="$(command -v sing-box)"
  else
    ARCH="$(uname -m)"
    case "$ARCH" in
      x86_64|amd64) A=amd64 ;;
      aarch64|arm64) A=arm64 ;;
      *) die "unsupported arch $ARCH - pass --singbox PATH" ;;
    esac
    say "downloading sing-box ${SB_VERSION} (${A})"
    URL="https://github.com/SagerNet/sing-box/releases/download/v${SB_VERSION}/sing-box-${SB_VERSION}-linux-${A}.tar.gz"
    curl -fsSL "$URL" -o "$TMP/sb.tar.gz" || die "download failed"
    tar xzf "$TMP/sb.tar.gz" -C "$TMP"
    SB_BIN="$TMP/sing-box-${SB_VERSION}-linux-${A}/sing-box"
    chmod +x "$SB_BIN"
  fi
fi
"$SB_BIN" version | head -1

# --- 2. build a client profile from the node -------------------------------
if [ -n "$SUB" ]; then
  say "fetching subscription from the panel"
  curl -fsSL -A "v2rayNG/1.9.44" "$SUB" -o "$TMP/sub.txt" || die "could not fetch $SUB"
else
  printf '%s\n' "$LINK" > "$TMP/sub.txt"
fi
"$GATEWAY" --config "$CONFIG" --from-file "$TMP/sub.txt" --ech-mode "$ECH_MODE" --out "$TMP/profile.json"
python3 - "$TMP/profile.json" "$PORT" <<'PY'
import json, sys
path, port = sys.argv[1], int(sys.argv[2])
cfg = json.load(open(path))
# test profile: no tun (needs root + hijacks the host), debug logs, loopback socks
cfg["inbounds"] = [i for i in cfg["inbounds"] if i["type"] != "tun"]
cfg.setdefault("inbounds", []).append({"type": "mixed", "tag": "mixed-test",
                                       "listen": "127.0.0.1", "listen_port": port})
cfg["log"]["level"] = "debug"
cfg["route"]["final"] = "PROXY"
json.dump(cfg, open(path, "w"), indent=2)
print("profile ready:", path)
PY

nodes="$(python3 -c "import json;print(len([o for o in json.load(open('$TMP/profile.json'))['outbounds'] if o['type'] in ('vless','vmess','trojan','shadowsocks','hysteria2','tuic')]))")"
[ "$nodes" -gt 0 ] || die "no usable node was built from the input"
echo "nodes in profile: $nodes"

# --- 3. run sing-box --------------------------------------------------------
say "starting sing-box (socks 127.0.0.1:$PORT, log $TMP/sb.log)"
"$SB_BIN" run -c "$TMP/profile.json" > "$TMP/sb.log" 2>&1 &
SB_PID=$!
for _ in $(seq 1 40); do
  grep -q "sing-box started" "$TMP/sb.log" 2>/dev/null && break
  grep -qi "FATAL" "$TMP/sb.log" 2>/dev/null && { echo "--- sing-box log ---"; cat "$TMP/sb.log"; die "sing-box failed to start"; }
  sleep 0.25
done

# --- 4. tests ---------------------------------------------------------------
HOST_IP="$(curl -fsS -m 10 https://api.ipify.org || echo '?')"
say "exit IP without proxy: $HOST_IP"

PROXY_IP="$(curl -fsS -m 25 --socks5-hostname "127.0.0.1:$PORT" https://api.ipify.org || echo 'FAILED')"
say "exit IP through the node: $PROXY_IP"

TRACE="$(curl -fsS -m 25 --socks5-hostname "127.0.0.1:$PORT" https://www.cloudflare.com/cdn-cgi/trace || echo 'FAILED')"
echo "$TRACE" | grep -E '^(ip|loc|colo)=' || true

sleep 1
say "ECH evidence from the sing-box log"
grep -a -E "cloudflare-ech\.com.*HTTPS|ech=\"" "$TMP/sb.log" | head -3 || echo "(no ECH HTTPS lookup seen)"
if grep -qa "no ECH config found in DNS records" "$TMP/sb.log"; then
  echo "!! sing-box reported: no ECH config found in DNS records"
fi
grep -a -iE "FATAL|handshake|error" "$TMP/sb.log" | tail -5 || true

say "result"
if [ "$PROXY_IP" != "FAILED" ] && [ "$PROXY_IP" != "$HOST_IP" ] && [ -n "$PROXY_IP" ]; then
  echo "PASS: tunnel works and traffic exits from your node ($PROXY_IP)."
elif [ "$PROXY_IP" = "$HOST_IP" ]; then
  echo "WARN: exit IP equals the host IP - traffic may not be going through the node."
else
  echo "FAIL: could not reach the internet through the node. Log above."
fi
[ "$KEEP" = 1 ] || echo "(details kept in $TMP until this shell exits)"
[ "$KEEP" = 1 ] && echo "sing-box still running (pid $SB_PID), config: $TMP/profile.json, log: $TMP/sb.log"
exit 0
