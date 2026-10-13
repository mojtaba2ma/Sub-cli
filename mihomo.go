package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---- mihomo / Clash.Meta YAML ---------------------------------------------
// A tiny deterministic block-style YAML writer (no external dependency).

var yamlNeedsQuote = regexp.MustCompile(`[:#\[\]{},&*!|>'"%@` + "`" + `\n]`)

func yamlScalar(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		if t == "" || yamlNeedsQuote.MatchString(t) || strings.TrimSpace(t) != t {
			b, _ := json.Marshal(t)
			return string(b)
		}
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func yamlBlock(v any, indent int) []string {
	pad := strings.Repeat("  ", indent)
	var out []string
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := t[k]
			switch vv := val.(type) {
			case map[string]any:
				if len(vv) == 0 {
					out = append(out, pad+k+": {}")
				} else {
					out = append(out, pad+k+":")
					out = append(out, yamlBlock(vv, indent+1)...)
				}
			case []any:
				if len(vv) == 0 {
					out = append(out, pad+k+": []")
				} else {
					out = append(out, pad+k+":")
					out = append(out, yamlBlock(vv, indent+1)...)
				}
			default:
				out = append(out, pad+k+": "+yamlScalar(val))
			}
		}
	case []any:
		for _, item := range t {
			switch vv := item.(type) {
			case map[string]any:
				keys := make([]string, 0, len(vv))
				for k := range vv {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				// keep "name" first for readability
				sort.SliceStable(keys, func(i, j int) bool {
					if keys[i] == "name" {
						return true
					}
					if keys[j] == "name" {
						return false
					}
					return keys[i] < keys[j]
				})
				first := true
				for _, k := range keys {
					val := vv[k]
					prefix := pad + "  "
					if first {
						prefix = pad + "- "
						first = false
					}
					switch sub := val.(type) {
					case map[string]any:
						if len(sub) == 0 {
							out = append(out, prefix+k+": {}")
						} else {
							out = append(out, prefix+k+":")
							out = append(out, yamlBlock(sub, indent+2)...)
						}
					case []any:
						if len(sub) == 0 {
							out = append(out, prefix+k+": []")
						} else {
							out = append(out, prefix+k+":")
							out = append(out, yamlBlock(sub, indent+2)...)
						}
					default:
						out = append(out, prefix+k+": "+yamlScalar(val))
					}
				}
			default:
				out = append(out, pad+"- "+yamlScalar(item))
			}
		}
	}
	return out
}

func buildMihomoProfile(cfg Config, nodes []Node, echMode string) string {
	proxies := make([]any, 0, len(nodes))
	names := make([]any, 0, len(nodes))
	tags := uniqueTags(func() []string {
		t := make([]string, 0, len(nodes))
		for i, n := range nodes {
			tag, _ := nodeToOutbound(cfg, n, i, echMode)
			t = append(t, tag)
		}
		return t
	}())

	for i, n := range nodes {
		tag := tags[i]
		_, ob := nodeToOutbound(cfg, n, i, echMode)
		p := map[string]any{"name": tag, "server": n.Server, "port": n.Port, "udp": true}
		tls, _ := ob["tls"].(map[string]any)
		tr, _ := ob["transport"].(map[string]any)

		switch n.Type {
		case "vless":
			p["type"] = "vless"
			p["uuid"] = n.UUID
			p["encryption"] = ""
			if n.Flow != "" {
				p["flow"] = n.Flow
			}
		case "vmess":
			p["type"] = "vmess"
			p["uuid"] = n.UUID
			p["alterId"] = n.AlterID
			p["cipher"] = n.Cipher
		case "trojan":
			p["type"] = "trojan"
			p["password"] = n.Password
		case "shadowsocks":
			p["type"] = "ss"
			p["cipher"] = n.Method
			p["password"] = n.Password
			if n.Plugin != "" {
				p["plugin"] = n.Plugin
				p["plugin-opts"] = n.PluginOpts
			}
		case "hysteria2":
			p["type"] = "hysteria2"
			p["password"] = n.Password
			if n.ObfsPass != "" {
				p["obfs"] = "salamander"
				p["obfs-password"] = n.ObfsPass
			}
		case "tuic":
			p["type"] = "tuic"
			p["uuid"] = n.UUID
			p["password"] = n.Password
			if n.Congestion != "" {
				p["congestion-controller"] = n.Congestion
			}
		default:
			continue
		}
		if tls != nil {
			p["tls"] = true
			if sn, ok := tls["server_name"].(string); ok && sn != "" {
				p["sni"] = sn
			}
			if alpn, ok := tls["alpn"].([]string); ok && len(alpn) > 0 {
				a := make([]any, 0, len(alpn))
				for _, x := range alpn {
					a = append(a, x)
				}
				p["alpn"] = a
			}
			if in, ok := tls["insecure"].(bool); ok && in {
				p["skip-cert-verify"] = true
			}
			if utls, ok := tls["utls"].(map[string]any); ok {
				if fp, ok := utls["fingerprint"].(string); ok {
					p["client-fingerprint"] = fp
				}
			}
			if real, ok := tls["reality"].(map[string]any); ok {
				p["reality-opts"] = map[string]any{
					"public-key": real["public_key"], "short-id": real["short_id"],
				}
			}
			if _, ok := tls["ech"]; ok {
				p["ech-opts"] = map[string]any{
					"enable": true, "query-server-name": cfg.ECH.Domain,
				}
			}
		}
		if tr != nil {
			switch tr["type"] {
			case "ws":
				p["network"] = "ws"
				ws := map[string]any{"path": tr["path"]}
				if h, ok := tr["headers"].(map[string]any); ok {
					ws["headers"] = map[string]any{"Host": h["Host"]}
				}
				if med, ok := tr["max_early_data"].(int); ok && med > 0 {
					ws["max-early-data"] = med
					ws["early-data-header-name"] = tr["early_data_header_name"]
				}
				p["ws-opts"] = ws
			case "grpc":
				p["network"] = "grpc"
				p["grpc-opts"] = map[string]any{"grpc-service-name": tr["service_name"]}
			case "httpupgrade":
				p["network"] = "httpupgrade"
				p["httpupgrade-opts"] = map[string]any{"path": tr["path"],
					"headers": map[string]any{"Host": tr["host"]}}
			case "http":
				p["network"] = "h2"
				p["h2-opts"] = map[string]any{"path": tr["path"], "host": tr["host"]}
			}
		}
		proxies = append(proxies, p)
		names = append(names, tag)
	}

	nsList := make([]any, 0, len(cfg.Profile.DNSDirect))
	for _, ip := range cfg.Profile.DNSDirect {
		nsList = append(nsList, "https://"+ip+"/dns-query")
	}
	doc := map[string]any{
		"mixed-port": cfg.Profile.MixedPort,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "warning",
		"dns": map[string]any{
			"enable": true, "ipv6": false,
			"nameserver":              nsList,
			"proxy-server-nameserver": nsList,
		},
		"proxies": proxies,
		"proxy-groups": []any{
			map[string]any{"name": "PROXY", "type": "select",
				"proxies": append([]any{"AUTO"}, names...)},
			map[string]any{"name": "AUTO", "type": "url-test", "proxies": names,
				"url": cfg.Profile.URLTestURL, "interval": 180},
		},
		"rules": []any{"GEOIP,LAN,DIRECT,no-resolve", "GEOIP,CN,DIRECT", "MATCH,PROXY"},
	}
	header := fmt.Sprintf("# generated by sb-ech-gateway v%s - mihomo profile\n"+
		"# ECH is set per proxy via ech-opts (query-server-name: %s)\n", version, cfg.ECH.Domain)
	return header + strings.Join(yamlBlock(doc, 0), "\n") + "\n"
}
