package main

import (
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var subIDRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`)

// ---- panel subscription cache ---------------------------------------------

type panelEntry struct {
	expires time.Time
	body    string
	headers http.Header
}

var (
	panelMu    sync.Mutex
	panelCache = map[string]panelEntry{}
)

func fetchPanelSub(cfg Config, subid string) (string, http.Header, error) {
	now := time.Now()
	panelMu.Lock()
	if e, ok := panelCache[subid]; ok && now.Before(e.expires) {
		panelMu.Unlock()
		return e.body, e.headers, nil
	}
	panelMu.Unlock()

	subURL := strings.ReplaceAll(cfg.Panel.SubURL, "{subid}", url.PathEscape(subid))
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", cfg.Panel.UserAgent)
	req.Header.Set("Accept", "*/*")
	if cfg.Panel.HostHeader != "" {
		req.Host = cfg.Panel.HostHeader
	}
	client := &http.Client{
		Timeout: time.Duration(cfg.Panel.TimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Panel.InsecureTLS},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.Header, fmt.Errorf("panel returned HTTP %d", resp.StatusCode)
	}
	panelMu.Lock()
	if len(panelCache) > 512 {
		for k := range panelCache {
			delete(panelCache, k)
			if len(panelCache) < 384 {
				break
			}
		}
	}
	panelCache[subid] = panelEntry{expires: now.Add(30 * time.Second), body: string(body), headers: resp.Header}
	panelMu.Unlock()
	return string(body), resp.Header, nil
}

// ---- HTTP server -----------------------------------------------------------

type gateway struct {
	cfg Config
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[error] panic: %v", rec)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}()
	parsed, err := url.Parse(r.URL.RequestURI())
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	q := parsed.Query()
	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	base := strings.Trim(g.cfg.BasePath, "/")

	if len(parts) < 2 || parts[0] != base {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(g.cfg.Secret)) != 1 {
		log.Printf("[warn] bad secret from %s", clientIP(r))
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rest := parts[2:]

	if len(rest) == 1 && rest[0] == "health" {
		g.handleHealth(w)
		return
	}
	if len(rest) < 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	kind, subid := rest[0], rest[1]
	if !subIDRe.MatchString(subid) {
		http.Error(w, "bad subscription id", http.StatusBadRequest)
		return
	}

	kind = g.resolveKind(r, kind, q)
	switch kind {
	case "sub", "subs", "clash", "mihomo", "xray", "auto":
	default:
		http.Error(w, "unknown endpoint", http.StatusNotFound)
		return
	}

	raw, panelHeaders, err := fetchPanelSub(g.cfg, subid)
	if err != nil {
		log.Printf("[error] panel fetch failed for %s: %v", maskID(subid), err)
		http.Error(w, "panel fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if strings.Contains(strings.ToLower(firstN(raw, 400)), "<html") {
		http.Error(w, "panel returned an HTML page - check panel.sub_url / user_agent", http.StatusBadGateway)
		return
	}

	echMode := q.Get("ech")
	if echMode != "dynamic" && echMode != "static" && echMode != "off" {
		echMode = "dynamic"
		if kind == "subs" {
			echMode = "static"
		}
	}
	if !g.cfg.ECH.StaticEndpoint && echMode == "static" {
		echMode = "dynamic"
	}

	baseInterval := g.cfg.Response.ProfileUpdateIntervalHours
	if kind == "subs" && baseInterval > 1 {
		baseInterval = 1 // static ECH keys rotate (~3 h)
	}
	hdr := http.Header{}
	hdr.Set("Profile-Update-Interval", fmt.Sprint(baseInterval))
	if ui := panelHeaders.Get("Subscription-Userinfo"); ui != "" {
		hdr.Set("Subscription-Userinfo", ui)
	}
	if g.cfg.Response.ProfileTitle != "" {
		hdr.Set("Profile-Title", g.cfg.Response.ProfileTitle)
	}

	switch kind {
	case "xray":
		ctype := panelHeaders.Get("Content-Type")
		if ctype == "" {
			ctype = "text/plain; charset=utf-8"
		}
		writeBody(w, http.StatusOK, ctype, []byte(raw), hdr)
		log.Printf("[info] served xray-passthrough/%s", maskID(subid))
	case "clash", "mihomo":
		nodes, skipped := ParseLinks(raw)
		if len(nodes) == 0 {
			http.Error(w, "no usable node (skipped: "+strings.Join(skipped, ", ")+")", http.StatusBadGateway)
			return
		}
		body := buildMihomoProfile(g.cfg, nodes, echMode)
		hdr.Del("Profile-Update-Interval")
		writeBody(w, http.StatusOK, "text/yaml; charset=utf-8", []byte(body), hdr)
		log.Printf("[info] served clash/%s: %d node(s), ech=%s", maskID(subid), len(nodes), echMode)
	default:
		nodes, skipped := ParseLinks(raw)
		if len(nodes) == 0 {
			http.Error(w, "no usable node (skipped: "+strings.Join(skipped, ", ")+")", http.StatusBadGateway)
			return
		}
		echErr := ""
		if echMode == "static" {
			if _, _, _, err := getECHConfig(g.cfg, false); err != nil {
				echErr = "could not fetch ECHConfigList from DoH resolvers: " + err.Error()
				log.Printf("[warn] %s", echErr)
			}
		}
		profile := buildSingboxProfile(g.cfg, nodes, echMode, echErr, skipped)
		for _, s := range skipped {
			log.Printf("[info] skipped link: %s", s)
		}
		body, err := json.MarshalIndent(stripMeta(profile), "", "  ")
		if err != nil {
			http.Error(w, "json error", http.StatusInternalServerError)
			return
		}
		writeBody(w, http.StatusOK, "application/json; charset=utf-8", body, hdr)
		log.Printf("[info] served %s/%s: %d node(s), ech=%s", kind, maskID(subid), len(nodes), echMode)
	}
}

// resolveKind decides the output format, honouring ?format= and the User-Agent.
func (g *gateway) resolveKind(r *http.Request, kind string, q url.Values) string {
	if f := strings.ToLower(q.Get("format")); f != "" {
		switch f {
		case "clash", "mihomo":
			return "clash"
		case "singbox", "sing-box", "sub", "json":
			return "sub"
		case "static", "subs":
			return "subs"
		case "xray", "raw", "links":
			return "xray"
		}
	}
	if kind != "auto" {
		return kind
	}
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	switch {
	case containsAny(ua, "clash", "mihomo", "stash", "flclash", "verge"):
		return "clash"
	case strings.Contains(ua, "hiddify"):
		return "subs" // Hiddify replaces the DNS section; the static key is safest
	case containsAny(ua, "sing-box", "singbox", "sfi", "nekobox", "nekoray", "karing",
		"gui.for", "gui-for", "streisand", "sfa", "sft", "sing"):
		return "sub"
	default:
		return "xray"
	}
}

func (g *gateway) handleHealth(w http.ResponseWriter) {
	b64, ttl, at, err := getECHConfig(g.cfg, false)
	resp := map[string]any{
		"ok":             err == nil && b64 != "",
		"version":        version,
		"ech_domain":     g.cfg.ECH.Domain,
		"ech_cached":     b64 != "",
		"ech_ttl":        ttl,
		"ech_fetched_at": "",
		"time":           time.Now().Format(time.RFC3339),
	}
	if !at.IsZero() {
		resp["ech_fetched_at"] = at.Format("2006-01-02 15:04:05")
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	body, _ := json.MarshalIndent(resp, "", "  ")
	writeBody(w, http.StatusOK, "application/json; charset=utf-8", body, nil)
}

// ---- helpers ---------------------------------------------------------------

func writeBody(w http.ResponseWriter, code int, ctype string, body []byte, hdr http.Header) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	for k, vals := range hdr {
		for _, v := range vals {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(code)
	if _, err := w.Write(body); err != nil {
		log.Printf("[warn] write failed: %v", err)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func maskID(id string) string {
	if len(id) <= 4 {
		return "***"
	}
	return id[:4] + "***"
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Real-IP"); xff != "" {
		return xff
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
