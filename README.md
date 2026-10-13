# sb-ech-gateway

**Make a [3x-ui](https://github.com/MHSanaei/3x-ui) `VLESS + WS + TLS` node behind Cloudflare's
CDN work on sing-box clients (Hiddify, sing-box, NekoBox, Karing, SFI, GUI.for.SingBox …)
*with Encrypted Client Hello (ECH) enabled*.**

Xray clients accept this string in the panel:

```
cloudflare-ech.com+https://8.8.8.8/dns-query      # Xray-only "DNS-query" form
```

sing-box has a completely different model, and it **hard-fails** (`no ECH config found in DNS records`)
when it cannot find an ECH config — it never falls back to plain TLS. On top of that, Cloudflare publishes
its shared ECH config **only on `cloudflare-ech.com`**; your own CDN domain does not carry an `ech=`
SvcParam, so a query name must be provided explicitly.

This project is a tiny single-binary gateway that sits between the panel and the client: it reads the
panel's **existing subscription links** and re-renders them as a complete sing-box profile (and a mihomo
YAML profile) with the correct ECH block.

```
                    ┌────────────────────────────┐
 3x-ui panel  ───▶  │  sb-ech-gateway :2199      │ ───▶ /auto/<subid>   one link for every client
 (raw sub links)    │  (systemd, 127.0.0.1 only) │ ───▶ /sub/<subid>    sing-box, live ECH from DNS
                    │                            │ ───▶ /subs/<subid>   sing-box, static ECH (PEM)
 nginx :443 ──────▶ │                            │ ───▶ /clash/<subid>  mihomo, ech-opts
                    └────────────────────────────┘ ───▶ /xray/<subid>   raw panel links untouched
```

## Quick start (server-side, one command)

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/sb-ech-gateway/main/scripts/install-remote.sh | sudo bash
```

The installer:

1. detects your panel (`http://127.0.0.1:2096`) and asks for its subscription path,
2. generates a random **secret** and shows it at the end (save it!),
3. builds/installs the Go binary (no runtime dependencies) and a hardened systemd unit,
4. adds the `/sb/` location to nginx (and rolls back automatically if `nginx -t` fails),
5. runs an ECH health check and prints ready-to-use subscription URLs.

Interactive flags: `--panel URL`, `--sub-path /sub`, `--domain panel.example.com`, `--port 2199`,
`--no-nginx`, `--no-systemd`, `--bin-source auto|build|release|local`, `--yes`, `--uninstall`.

## What the client gets

| URL | Output | Best for |
|---|---|---|
| `/sb/<secret>/auto/<subid>` | picks a format from the User-Agent | give this one to everybody |
| `/sb/<secret>/sub/<subid>` | sing-box profile, `ech.query_server_name` | sing-box, NekoBox, Karing, SFI |
| `/sb/<secret>/subs/<subid>` | sing-box profile, static ECH **PEM** | Hiddify (it replaces the DNS section) |
| `/sb/<secret>/clash/<subid>` | mihomo YAML with `ech-opts` | Clash Verge Rev, Mihomo, FlClash |
| `/sb/<secret>/xray/<subid>` | the panel's raw links | v2rayNG, Happ, Shadowrocket |
| `/sb/<secret>/health` | ECH status JSON | monitoring (cron) |

The ECH block that lands in every sing-box node:

```jsonc
"tls": {
  "enabled": true,
  "server_name": "cdn.example.com",
  "alpn": ["h2", "http/1.1"],
  "utls": { "enabled": true, "fingerprint": "chrome" },
  "ech": { "enabled": true, "query_server_name": "cloudflare-ech.com" }   // live key, every connection
}
```

## Why not just put the Xray string in the client?

Tested with a real sing-box (v1.14.3) and real Cloudflare:

* `echConfigList`-style strings are **not** understood by sing-box — the key is silently absent.
* without `query_server_name`/`config`, sing-box dies with
  `FATAL … no ECH config found in DNS records` (no fallback).
* static ECH must be **PEM**, not raw base64: raw base64 → `invalid ECH configs pem`.
* Cloudflare rotates the shared ECH key (observed: `config_id` 64 → 124 within an hour), so a baked-in
  key goes stale — the dynamic form (or the gateway's `/subs/` endpoint, which re-fetches it) is the safe path.
* sing-box has **no `xhttp` transport**; such nodes are skipped with a clear log line.
* the panel itself (3.10.0) emits no `ech-opts` in its Clash output and no sing-box output at all.

Raw logs: [`evidence/ech-tests.log`](evidence/ech-tests.log).

## Install from source (for developers)

```bash
git clone https://github.com/OWNER/sb-ech-gateway && cd sb-ech-gateway
make build            # -> ./sb-ech-gateway  (pure Go, stdlib only)
./sb-ech-gateway --selftest      # writes samples/profile-{dynamic,static}.json + mihomo yaml
sudo bash install.sh             # interactive installer
```

Verify a built profile against the real core:

```bash
sing-box check -c samples/profile-dynamic.json
/opt/sb-ech-gateway/test-on-server.sh --sub 'http://127.0.0.1:2096/sub/<SUBID>'
```

## Management

```bash
sb-ech url        # URLs + secret
sb-ech health     # ECH cache state and last fetch
sb-ech logs       # journalctl -u sb-ech-gateway
sb-ech edit       # edit config.json and restart
sb-ech update     # pull the latest build from GitHub
bash /opt/sb-ech-gateway/install.sh --uninstall
```

## Configuration

`/opt/sb-ech-gateway/config.json` (see [config.example.json](config.example.json)):
`panel.sub_url` (with `{subid}` placeholder), `panel.user_agent`, `ech.domain`,
`ech.resolvers`, `ech.mode` (`all` | `domain-list` | `off`), `profile.*` for the generated client profile.

## Notes & caveats

* ECH **requires TLS 1.3 and a CDN that supports ECH**; the node's SNI never leaves the client in clear text.
* Never enable ECH on nodes that are **not** behind Cloudflare — the key will not match.
* `xhttp` nodes cannot work on sing-box; use WS (or give those users an Xray client).
* This tool does not touch your panel: Xray users keep using the panel's original subscription.

MIT licensed.
