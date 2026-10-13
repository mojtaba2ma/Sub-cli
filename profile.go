package main

import (
	"fmt"
	"time"
)

// ---- sing-box profile ------------------------------------------------------

func wantECH(cfg Config, n Node, echMode string) bool {
	if !cfg.ECH.Enabled || echMode == "off" || cfg.ECH.Mode == "off" {
		return false
	}
	if n.Security != "tls" || n.Network == "xhttp" {
		return false
	}
	if cfg.ECH.Mode == "domain-list" {
		names := []string{n.SNI, n.Server, n.Host}
		for _, suffix := range cfg.ECH.Domains {
			for _, name := range names {
				if name == suffix || (len(name) > len(suffix) && name[len(name)-len(suffix)-1:] == "."+suffix) {
					return true
				}
			}
		}
		return false
	}
	return true
}

func tlsBlock(cfg Config, n Node, echMode string) map[string]any {
	tls := map[string]any{"enabled": true}
	if n.SNI != "" {
		tls["server_name"] = n.SNI
	}
	if len(n.ALPN) > 0 {
		tls["alpn"] = n.ALPN
	}
	if n.Insecure {
		tls["insecure"] = true
	}
	fp := n.FP
	if fp == "" {
		fp = cfg.Profile.Fingerprint
	}
	if n.Security == "reality" && fp == "" {
		fp = "chrome" // sing-box rejects reality outbounds without uTLS
	}
	if fp != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	if n.Security == "reality" {
		tls["reality"] = map[string]any{
			"enabled":    true,
			"public_key": n.PBK,
			"short_id":   n.SID,
		}
		return tls
	}
	if wantECH(cfg, n, echMode) {
		if echMode == "static" {
			if b64, _, _, err := getECHConfig(cfg, false); err == nil && b64 != "" {
				tls["ech"] = map[string]any{"enabled": true, "config": echPEMLines(b64)}
			}
		} else {
			tls["ech"] = map[string]any{"enabled": true, "query_server_name": cfg.ECH.Domain}
		}
	}
	return tls
}

func transportBlock(n Node) map[string]any {
	host := n.Host
	if host == "" {
		host = n.SNI
	}
	switch n.Network {
	case "ws":
		tr := map[string]any{"type": "ws", "path": orSlash(n.Path)}
		if host != "" {
			tr["headers"] = map[string]any{"Host": host}
		}
		if n.EarlyData > 0 {
			tr["max_early_data"] = n.EarlyData
			tr["early_data_header_name"] = "Sec-WebSocket-Protocol"
		}
		return tr
	case "httpupgrade":
		tr := map[string]any{"type": "httpupgrade", "path": orSlash(n.Path)}
		if host != "" {
			tr["host"] = host
		}
		return tr
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": n.Service}
	case "h2", "http":
		tr := map[string]any{"type": "http", "path": orSlash(n.Path)}
		if host != "" {
			tr["host"] = []string{host}
		}
		return tr
	case "quic":
		return map[string]any{"type": "quic"}
	}
	return nil
}

func orSlash(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

func nodeToOutbound(cfg Config, n Node, index int, echMode string) (string, map[string]any) {
	name := n.Tag
	if name == "" {
		name = n.Server
	}
	if r := []rune(name); len(r) > 40 {
		name = string(r[:40])
	}
	tag := fmt.Sprintf("%02d %s", index+1, name)

	ob := map[string]any{"type": n.Type, "tag": tag, "server": n.Server, "server_port": n.Port}
	switch n.Type {
	case "vless":
		ob["uuid"] = n.UUID
		if n.Flow != "" {
			ob["flow"] = n.Flow
		}
	case "vmess":
		ob["uuid"] = n.UUID
		ob["security"] = n.Cipher
		if n.AlterID != 0 {
			ob["alter_id"] = n.AlterID
		}
	case "trojan":
		ob["password"] = n.Password
	case "shadowsocks":
		ob["method"] = n.Method
		ob["password"] = n.Password
		if n.Plugin != "" {
			ob["plugin"] = n.Plugin
			if n.PluginOpts != "" {
				ob["plugin_opts"] = n.PluginOpts
			}
		}
	case "hysteria2":
		ob["password"] = n.Password
		if n.ObfsPass != "" {
			ob["obfs"] = map[string]any{"type": "salamander", "password": n.ObfsPass}
		}
	case "tuic":
		ob["uuid"] = n.UUID
		ob["password"] = n.Password
		if n.Congestion != "" {
			ob["congestion_control"] = n.Congestion
		}
	case "socks", "http":
		if n.Username != "" {
			ob["username"] = n.Username
			ob["password"] = n.Password
		}
		if n.Type == "socks" {
			ob["version"] = "5"
		}
	}
	if n.Security == "tls" || n.Security == "reality" {
		ob["tls"] = tlsBlock(cfg, n, echMode)
	}
	if tr := transportBlock(n); tr != nil {
		ob["transport"] = tr
	}
	return tag, ob
}

func uniqueTags(tags []string) []string {
	seen := map[string]int{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if n, ok := seen[t]; ok {
			seen[t] = n + 1
			out = append(out, fmt.Sprintf("%s (%d)", t, n+1))
		} else {
			seen[t] = 0
			out = append(out, t)
		}
	}
	return out
}

func buildSingboxProfile(cfg Config, nodes []Node, echMode string, echErr string, skipped []string) map[string]any {
	tags := make([]string, 0, len(nodes))
	outs := make([]any, 0, len(nodes)+3)
	refs := make([]string, 0, len(nodes))
	for i, n := range nodes {
		tag, ob := nodeToOutbound(cfg, n, i, echMode)
		tags = append(tags, tag)
		outs = append(outs, ob)
	}
	tags = uniqueTags(tags)
	for i := range nodes {
		if ob, ok := outs[i].(map[string]any); ok {
			ob["tag"] = tags[i]
		}
		refs = append(refs, tags[i])
	}
	outs = append(outs, map[string]any{
		"type": "selector", "tag": "PROXY",
		"outbounds": append([]string{"AUTO"}, append(refs, "DIRECT")...),
		"default":   "AUTO",
	})
	outs = append(outs, map[string]any{
		"type": "urltest", "tag": "AUTO", "outbounds": refs,
		"url": cfg.Profile.URLTestURL, "interval": cfg.Profile.URLTestInterval, "tolerance": 50,
	})
	outs = append(outs, map[string]any{
		"type": "direct", "tag": "DIRECT",
		"domain_resolver": map[string]any{"server": "dns-direct"},
	})

	dnsServers := []any{}
	for i, ip := range cfg.Profile.DNSDirect {
		tag := "dns-direct"
		if i > 0 {
			tag = fmt.Sprintf("dns-direct-%d", i)
		}
		dnsServers = append(dnsServers, map[string]any{
			"tag": tag, "type": "https", "server": ip, "detour": "DIRECT",
		})
	}
	dnsServers = append(dnsServers, map[string]any{
		"tag": "dns-ech", "type": "https", "server": cfg.Profile.DNSEch, "detour": "DIRECT",
	})
	dnsRules := []any{}
	if cfg.ECH.Enabled && echMode != "off" {
		dnsRules = append(dnsRules, map[string]any{
			"domain": []string{cfg.ECH.Domain}, "query_type": []string{"HTTPS"}, "server": "dns-ech",
		})
	}

	inbounds := []any{}
	if cfg.Profile.IncludeTun {
		inbounds = append(inbounds, map[string]any{
			"type": "tun", "tag": "tun-in",
			"address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
			"mtu":     9000, "auto_route": true, "strict_route": false, "stack": "mixed",
		})
	}
	inbounds = append(inbounds, map[string]any{
		"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": cfg.Profile.MixedPort,
	})

	meta := map[string]any{
		"generated_by": "sb-ech-gateway v" + version,
		"generated_at": time.Now().Unix(),
		"ech_mode":     echMode,
		"ech_domain":   cfg.ECH.Domain,
		"nodes":        len(nodes),
	}
	if echErr != "" {
		meta["ech_error"] = echErr
	}
	if len(skipped) > 0 {
		meta["skipped_links"] = skipped
	}

	return map[string]any{
		"log":       map[string]any{"level": cfg.Profile.LogLevel, "timestamp": true},
		"dns":       map[string]any{"servers": dnsServers, "rules": dnsRules, "final": "dns-direct", "strategy": cfg.Profile.Strategy},
		"inbounds":  inbounds,
		"outbounds": outs,
		"route": map[string]any{
			"rules": []any{
				map[string]any{"action": "sniff"},
				map[string]any{"protocol": "dns", "action": "hijack-dns"},
				map[string]any{"action": "route", "outbound": "DIRECT", "ip_is_private": true},
			},
			"final":                   "PROXY",
			"default_domain_resolver": map[string]any{"server": "dns-direct"},
			"auto_detect_interface":   true,
		},
		"_meta": meta,
	}
}

// stripMeta removes the "_meta" documentation block from a profile.
func stripMeta(p map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range p {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		out[k] = v
	}
	return out
}
