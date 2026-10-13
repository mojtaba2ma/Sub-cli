// sub_gateway - bridge 3x-ui (Xray) subscriptions to sing-box / mihomo clients
// with working Encrypted Client Hello (ECH).
//
//	Xray:     echConfigList = "cloudflare-ech.com+https://8.8.8.8/dns-query"   (Xray-only)
//	sing-box: "ech": {"enabled": true, "query_server_name": "cloudflare-ech.com"}
//	          "ech": {"enabled": true, "config": ["-----BEGIN ECH CONFIGS-----", ...]}
//
// sing-box hard-fails when the ECH config cannot be found, and Cloudflare only
// publishes the shared config on cloudflare-ech.com, so the query_server_name
// (or a static PEM config) is mandatory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const version = "2.0.0"

func main() {
	cfgPath := flag.String("config", "config.json", "path to config.json")
	selftestFlag := flag.Bool("selftest", false, "build sample profiles from built-in links and exit")
	fromFile := flag.String("from-file", "", "build a profile from links/base64 in FILE and exit")
	echMode := flag.String("ech-mode", "dynamic", "dynamic|static|off")
	out := flag.String("out", "-", "output file for --from-file/-selftest (- = stdout)")
	showVersion := flag.Bool("version", false, "print version and exit")
	printURLs := flag.Bool("urls", false, "print the subscription URLs of this installation and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sb-ech-gateway %s\n", version)
		return
	}

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("config: %v", err)
		}
		log.Printf("[warn] config %s not found - using defaults", *cfgPath)
	}
	log.SetFlags(log.LstdFlags)

	if *printURLs {
		fmt.Printf("secret: %s\n", cfg.Secret)
		fmt.Printf("endpoints (append /<subid>):\n")
		for _, ep := range []string{"auto", "sub", "subs", "clash", "xray"} {
			fmt.Printf("  %s/%s/%s/\n", cfg.BasePath, cfg.Secret, ep)
		}
		return
	}

	if *selftestFlag {
		if err := selftest(cfg, *out); err != nil {
			log.Fatalf("selftest: %v", err)
		}
		return
	}

	if *fromFile != "" {
		data, err := os.ReadFile(*fromFile)
		if err != nil {
			log.Fatalf("read %s: %v", *fromFile, err)
		}
		body, err := BuildFromText(cfg, string(data), *echMode)
		if err != nil {
			log.Fatalf("build: %v", err)
		}
		if *out == "-" {
			fmt.Println(string(body))
		} else if err := os.WriteFile(*out, append(body, '\n'), 0o644); err != nil {
			log.Fatalf("write %s: %v", *out, err)
		} else {
			log.Printf("[info] wrote %s", *out)
		}
		return
	}

	addr := fmt.Sprintf("%s:%d", cfg.Listen, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           &gateway{cfg: cfg},
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("[info] sb-ech-gateway v%s listening on http://%s%s/<secret>/...", version, addr, cfg.BasePath)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// BuildFromText converts a panel subscription body into a sing-box profile.
func BuildFromText(cfg Config, text string, echMode string) ([]byte, error) {
	nodes, skipped := ParseLinks(text)
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no usable node (skipped: %v)", skipped)
	}
	echErr := ""
	if echMode == "static" {
		if _, _, _, err := getECHConfig(cfg, false); err != nil {
			echErr = "could not fetch ECHConfigList from DoH resolvers: " + err.Error()
		}
	}
	profile := buildSingboxProfile(cfg, nodes, echMode, echErr, skipped)
	return json.MarshalIndent(stripMeta(profile), "", "  ")
}
