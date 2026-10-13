package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Node is a transport-agnostic view of one share link.
type Node struct {
	Type       string
	Tag        string
	Server     string
	Port       int
	UUID       string
	Password   string
	Method     string
	Plugin     string
	PluginOpts string
	AlterID    int
	Cipher     string // vmess security
	Network    string
	Security   string
	SNI        string
	FP         string
	Host       string
	Path       string
	Service    string
	ALPN       []string
	PBK        string
	SID        string
	Flow       string
	Insecure   bool
	EarlyData  int
	ObfsPass   string
	Congestion string
	Username   string
}

var errSkip = fmt.Errorf("skipped")

func q1(v url.Values, keys ...string) string {
	for _, k := range keys {
		if s := v.Get(k); s != "" {
			return s
		}
	}
	return ""
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var edRe = regexp.MustCompile(`[?&]ed=(\d+)`)

func stripEarlyData(path string) (string, int) {
	m := edRe.FindStringSubmatch(path)
	if m == nil {
		return path, 0
	}
	ed, _ := strconv.Atoi(m[1])
	clean := edRe.ReplaceAllString(path, "")
	clean = strings.TrimRight(clean, "?&")
	if clean == "" {
		clean = "/"
	}
	return clean, ed
}

func hostPort(u *url.URL, defaultPort int) (string, int, error) {
	host := u.Hostname()
	if host == "" {
		return "", 0, fmt.Errorf("missing host")
	}
	port := defaultPort
	if p := u.Port(); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil {
			return "", 0, fmt.Errorf("bad port")
		}
		port = v
	}
	if port == 0 {
		return "", 0, fmt.Errorf("missing port")
	}
	return host, port, nil
}

func parseVLESS(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	n := Node{Type: "vless", Tag: u.Fragment}
	if u.User != nil {
		n.UUID = u.User.Username()
	}
	n.Server, n.Port, err = hostPort(u, 443)
	if err != nil {
		return n, err
	}
	q := u.Query()
	n.Network = q1(q, "type", "network")
	if n.Network == "" {
		n.Network = "tcp"
	}
	n.Security = q1(q, "security")
	n.SNI = q1(q, "sni", "serverName", "peer")
	n.FP = q1(q, "fp")
	n.ALPN = splitCSV(q1(q, "alpn"))
	n.Host = q1(q, "host")
	n.Path, n.EarlyData = stripEarlyData(q1(q, "path"))
	n.Service = q1(q, "serviceName")
	n.PBK = q1(q, "pbk")
	n.SID = q1(q, "sid")
	n.Flow = q1(q, "flow")
	n.Insecure = q1(q, "allowInsecure", "insecure") == "1"
	return n, nil
}

func parseTrojan(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	n := Node{Type: "trojan", Tag: u.Fragment}
	if u.User != nil {
		n.Password = u.User.Username()
	}
	n.Server, n.Port, err = hostPort(u, 443)
	if err != nil {
		return n, err
	}
	q := u.Query()
	n.Network = q1(q, "type", "network")
	if n.Network == "" {
		n.Network = "tcp"
	}
	n.Security = q1(q, "security")
	if n.Security == "" {
		n.Security = "tls"
	}
	n.SNI = q1(q, "sni", "peer", "serverName")
	n.FP = q1(q, "fp")
	n.ALPN = splitCSV(q1(q, "alpn"))
	n.Host = q1(q, "host")
	n.Path, n.EarlyData = stripEarlyData(q1(q, "path"))
	n.Service = q1(q, "serviceName")
	n.Insecure = q1(q, "allowInsecure", "insecure") == "1"
	return n, nil
}

func parseVMess(raw string) (Node, error) {
	body := strings.TrimPrefix(raw, "vmess://")
	body = strings.TrimSpace(body)
	dec, err := base64.StdEncoding.DecodeString(padB64(body))
	if err != nil {
		dec, err = base64.RawStdEncoding.DecodeString(body)
		if err != nil {
			return Node{}, fmt.Errorf("bad base64")
		}
	}
	var obj map[string]any
	if err := json.Unmarshal(dec, &obj); err != nil {
		return Node{}, fmt.Errorf("bad vmess json")
	}
	s := func(k string) string {
		if v, ok := obj[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				return t
			case float64:
				return strconv.FormatInt(int64(t), 10)
			}
		}
		return ""
	}
	n := Node{Type: "vmess", Tag: s("ps"), Server: s("add")}
	if n.Server == "" {
		return n, fmt.Errorf("missing host")
	}
	if p := s("port"); p != "" {
		n.Port, _ = strconv.Atoi(p)
	}
	if n.Port == 0 {
		n.Port = 443
	}
	n.UUID = s("id")
	n.Cipher = s("scy")
	if n.Cipher == "" {
		n.Cipher = "auto"
	}
	if aid := s("aid"); aid != "" {
		n.AlterID, _ = strconv.Atoi(aid)
	}
	n.Network = strings.ToLower(s("net"))
	if n.Network == "" {
		n.Network = "tcp"
	}
	tls := strings.ToLower(s("tls"))
	n.Security = tls
	if n.Security == "" {
		n.Security = "none"
	}
	n.SNI = s("sni")
	n.FP = s("fp")
	n.ALPN = splitCSV(s("alpn"))
	n.Host = s("host")
	n.Path, n.EarlyData = stripEarlyData(s("path"))
	if n.Network == "grpc" {
		n.Service = s("path")
	}
	return n, nil
}

func padB64(s string) string {
	if m := len(s) % 4; m != 0 {
		return s + strings.Repeat("=", 4-m)
	}
	return s
}

func parseSS(raw string) (Node, error) {
	body := raw
	if i := strings.Index(body, "://"); i >= 0 {
		body = body[i+3:]
	}
	frag := ""
	if i := strings.Index(body, "#"); i >= 0 {
		if f, err := url.QueryUnescape(body[i+1:]); err == nil {
			frag = f
		} else {
			frag = body[i+1:]
		}
		body = body[:i]
	}
	query := ""
	if i := strings.Index(body, "?"); i >= 0 {
		query, body = body[i+1:], body[:i]
	}
	var method, password, hostpart string
	if i := strings.LastIndex(body, "@"); i >= 0 {
		userinfo, host := body[:i], body[i+1:]
		if dec, err := base64.StdEncoding.DecodeString(padB64(userinfo)); err == nil {
			method, password = splitCreds(string(dec))
		} else {
			method, password = splitCreds(userinfo)
		}
		hostpart = host
	} else {
		dec, err := base64.StdEncoding.DecodeString(padB64(body))
		if err != nil {
			return Node{}, fmt.Errorf("bad ss link")
		}
		creds, host := splitCredsAt(string(dec))
		method, password = splitCreds(creds)
		hostpart = host
	}
	host, portStr, ok := strings.Cut(hostpart, ":")
	if !ok {
		return Node{}, fmt.Errorf("missing port")
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		port = 8388
	}
	n := Node{Type: "shadowsocks", Tag: frag, Server: host, Port: port,
		Method: method, Password: password}
	if query != "" {
		v, _ := url.ParseQuery(query)
		if p := v.Get("plugin"); p != "" {
			if dec, err := url.QueryUnescape(p); err == nil {
				p = dec
			}
			parts := strings.SplitN(p, ";", 2)
			n.Plugin = parts[0]
			if len(parts) > 1 {
				n.PluginOpts = parts[1]
			}
		}
	}
	return n, nil
}

func splitCreds(s string) (string, string) {
	m, p, _ := strings.Cut(s, ":")
	return m, p
}

func splitCredsAt(s string) (string, string) {
	i := strings.LastIndex(s, "@")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func parseHysteria2(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	n := Node{Type: "hysteria2", Tag: u.Fragment, Security: "tls", Network: "tcp"}
	if u.User != nil {
		n.Password = u.User.Username()
	}
	n.Server, n.Port, err = hostPort(u, 443)
	if err != nil {
		return n, err
	}
	q := u.Query()
	n.SNI = q1(q, "sni", "peer")
	n.ALPN = splitCSV(q1(q, "alpn"))
	n.Insecure = q1(q, "insecure", "allowInsecure") == "1"
	if q1(q, "obfs") == "salamander" || q1(q, "obfs-password") != "" {
		n.ObfsPass = q1(q, "obfs-password", "obfsPassword")
	}
	return n, nil
}

func parseTUIC(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	n := Node{Type: "tuic", Tag: u.Fragment, Security: "tls", Network: "tcp"}
	if u.User != nil {
		n.UUID = u.User.Username()
		n.Password, _ = u.User.Password()
	}
	n.Server, n.Port, err = hostPort(u, 443)
	if err != nil {
		return n, err
	}
	q := u.Query()
	n.SNI = q1(q, "sni")
	n.ALPN = splitCSV(q1(q, "alpn"))
	n.Insecure = q1(q, "insecure", "allow_insecure") == "1"
	n.Congestion = q1(q, "congestion_control", "congestion")
	return n, nil
}

func parseSocksHTTP(raw string, kind string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}
	def := 1080
	if kind == "http" {
		def = 8080
	}
	n := Node{Type: kind, Tag: u.Fragment, Security: "none", Network: "tcp"}
	n.Server, n.Port, err = hostPort(u, def)
	if err != nil {
		return n, err
	}
	if u.User != nil && kind == "socks" {
		n.Username = u.User.Username()
		n.Password, _ = u.User.Password()
	}
	return n, nil
}

// ParseLinks accepts plain text links or a base64 blob of links (3x-ui default).
func ParseLinks(text string) ([]Node, []string) {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "://") {
		if dec, err := base64.StdEncoding.DecodeString(padB64(strings.ReplaceAll(text, "\n", ""))); err == nil {
			text = string(dec)
		}
	}
	var nodes []Node
	var skipped []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || !strings.Contains(line, "://") {
			continue
		}
		scheme := strings.ToLower(strings.SplitN(line, "://", 2)[0])
		var (
			n   Node
			err error
		)
		switch scheme {
		case "vless":
			n, err = parseVLESS(line)
		case "vmess":
			n, err = parseVMess(line)
		case "trojan":
			n, err = parseTrojan(line)
		case "ss":
			n, err = parseSS(line)
		case "hysteria2", "hy2":
			n, err = parseHysteria2(line)
		case "tuic":
			n, err = parseTUIC(line)
		case "socks", "socks5":
			n, err = parseSocksHTTP(line, "socks")
		case "http":
			n, err = parseSocksHTTP(line, "http")
		default:
			skipped = append(skipped, scheme+" (unsupported scheme)")
			continue
		}
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s (parse error: %v)", scheme, err))
			continue
		}
		if n.Server == "" || n.Port == 0 {
			skipped = append(skipped, scheme+" (missing host/port)")
			continue
		}
		if scheme == "vless" && n.Network == "xhttp" {
			skipped = append(skipped, "vless/xhttp (sing-box has no xhttp transport)")
			continue
		}
		if n.Security == "reality" {
			if raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(n.PBK, "=")); err != nil || len(raw) != 32 {
				skipped = append(skipped, scheme+" (invalid reality public key)")
				continue
			}
		}
		if n.SNI == "" && (n.Security == "tls" || n.Security == "reality") {
			if n.Host != "" {
				n.SNI = n.Host
			} else {
				n.SNI = n.Server
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, skipped
}
