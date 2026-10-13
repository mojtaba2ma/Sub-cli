package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Sample links that cover the shapes 3x-ui 3.10.0 emits (vless+ws+tls with ech,
// vmess, trojan, shadowsocks, hysteria2, reality, plain tcp) plus an xhttp node
// that must be skipped and an invalid reality key that must be skipped too.
const selftestLinks = `vless://11111111-1111-4111-8111-111111111111@cdn.example.com:443?type=ws&security=tls&encryption=none&sni=cdn.example.com&fp=chrome&alpn=h2%2Chttp%2F1.1&host=cdn.example.com&path=%2Fmyws%3Fed%3D2560&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F8.8.8.8%2Fdns-query#Node-WS-CDN
vless://22222222-2222-4222-8222-222222222222@1.2.3.4:8443?type=ws&security=tls&sni=sni.example.com&host=sni.example.com&path=%2Fws#Node-WS-Direct
vmess://eyJ2IjoiMiIsInBzIjoiTm9kZS1WbWVzcyIsImFkZCI6ImNkbi5leGFtcGxlLmNvbSIsInBvcnQiOiI0NDMiLCJpZCI6IjMzMzMzMzMzLTMzMzMtNDMzMy04MzMzLTMzMzMzMzMzMzMzMyIsImFpZCI6IjAiLCJzY3kiOiJhdXRvIiwibmV0Ijoid3MiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiJjZG4uZXhhbXBsZS5jb20iLCJwYXRoIjoiL3dtZXNzIiwidGxzIjoidGxzIiwic25pIjoiY2RuLmV4YW1wbGUuY29tIn0=
trojan://passw0rd@cdn.example.com:443?type=ws&security=tls&sni=cdn.example.com&host=cdn.example.com&path=%2Ftj#Node-Trojan
ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@10.0.0.5:8388#Node-SS
hysteria2://mypass@hy.example.com:8443?sni=hy.example.com&insecure=1#Node-Hy2
vless://44444444-4444-4444-8444-444444444444@1.2.3.4:443?type=xhttp&security=tls&sni=x.example.com&host=x.example.com&path=%2Fx#Node-XHTTP-Should-Be-Skipped
vless://55555555-5555-4555-8555-555555555555@cdn.example.com:443?type=tcp&security=reality&sni=www.microsoft.com&pbk=jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0&sid=0123#Node-Reality-No-ECH
vless://66666666-6666-4666-8666-666666666666@tcp.example.com:8443?type=tcp&security=none#Node-Plain-TCP
`

func selftest(cfg Config, out string) error {
	nodes, skipped := ParseLinks(selftestLinks)
	fmt.Fprintf(os.Stderr, "[selftest] parsed %d node(s); skipped: %v\n", len(nodes), skipped)
	dir := "."
	if out != "-" {
		dir = filepath.Dir(out)
	} else {
		dir = "samples"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, mode := range []string{"dynamic", "static"} {
		echErr := ""
		if mode == "static" {
			if _, _, _, err := getECHConfig(cfg, false); err != nil {
				echErr = "doh fetch failed: " + err.Error()
			}
		}
		profile := buildSingboxProfile(cfg, nodes, mode, echErr, skipped)
		body, err := json.MarshalIndent(stripMeta(profile), "", "  ")
		if err != nil {
			return err
		}
		path := filepath.Join(dir, "profile-"+mode+".json")
		if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[selftest] wrote %s (ech=%s, nodes=%d)\n", path, mode, len(nodes))
	}
	yamlPath := filepath.Join(dir, "profile-mihomo.yaml")
	if err := os.WriteFile(yamlPath, []byte(buildMihomoProfile(cfg, nodes, "dynamic")), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[selftest] wrote %s\n", yamlPath)
	return nil
}
