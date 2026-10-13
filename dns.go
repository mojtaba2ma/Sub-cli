package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Minimal RFC 8484 (DoH) client used to read the live Cloudflare ECHConfigList.
// We speak the wire format by hand to avoid any dependency.

const (
	dnsTypeHTTPS  = 65
	svcParamECH   = 5
	dnsMsgMaxSize = 4096
)

func buildDNSQuery(name string, qtype uint16) ([]byte, uint16, int, error) {
	var buf bytes.Buffer
	id := uint16(time.Now().UnixNano() & 0xffff)
	_ = binary.Write(&buf, binary.BigEndian, id)
	_ = binary.Write(&buf, binary.BigEndian, uint16(0x0100)) // RD
	_ = binary.Write(&buf, binary.BigEndian, uint16(1))      // QDCOUNT
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	for _, label := range splitLabels(name) {
		if len(label) == 0 || len(label) > 63 {
			return nil, 0, 0, fmt.Errorf("invalid dns name %q", name)
		}
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.WriteByte(0)
	qnameLen := buf.Len() - 12
	_ = binary.Write(&buf, binary.BigEndian, qtype)
	_ = binary.Write(&buf, binary.BigEndian, uint16(1))
	return buf.Bytes(), id, qnameLen, nil
}

func splitLabels(name string) []string {
	var labels []string
	cur := ""
	for _, r := range name {
		if r == '.' {
			labels = append(labels, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		labels = append(labels, cur)
	}
	return labels
}

// readName decodes a (possibly compressed) DNS name.
func readName(msg []byte, off int) (string, int, error) {
	var labels []string
	end := off
	jumped := false
	for {
		if off >= len(msg) {
			return "", 0, fmt.Errorf("truncated name")
		}
		ln := int(msg[off])
		if ln == 0 {
			off++
			if !jumped {
				end = off
			}
			break
		}
		if ln&0xC0 == 0xC0 {
			if off+1 >= len(msg) {
				return "", 0, fmt.Errorf("truncated pointer")
			}
			ptr := int(binary.BigEndian.Uint16(msg[off:off+2]) & 0x3FFF)
			if !jumped {
				end = off + 2
			}
			off = ptr
			jumped = true
			continue
		}
		if off+1+ln > len(msg) {
			return "", 0, fmt.Errorf("truncated label")
		}
		labels = append(labels, string(msg[off+1:off+1+ln]))
		off += 1 + ln
	}
	name := ""
	for i, l := range labels {
		if i > 0 {
			name += "."
		}
		name += l
	}
	return name, end, nil
}

// parseECHAnswers extracts the ECHConfigList (base64) and TTL from a DNS response.
func parseECHAnswers(msg []byte, wantID uint16, qnameLen int) (string, int, error) {
	if len(msg) < 12 {
		return "", 0, fmt.Errorf("short response")
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	ancount := int(binary.BigEndian.Uint16(msg[6:8]))
	if id != wantID {
		return "", 0, fmt.Errorf("dns id mismatch")
	}
	if rc := flags & 0x000F; rc != 0 {
		return "", 0, fmt.Errorf("dns rcode %d", rc)
	}
	off := 12 + qnameLen + 4
	for i := 0; i < ancount; i++ {
		if _, next, err := readName(msg, off); err != nil {
			return "", 0, err
		} else {
			off = next
		}
		if off+10 > len(msg) {
			return "", 0, fmt.Errorf("truncated rr")
		}
		rtype := binary.BigEndian.Uint16(msg[off : off+2])
		ttl := int(binary.BigEndian.Uint32(msg[off+4 : off+8]))
		rdlen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
		off += 10
		if off+rdlen > len(msg) {
			return "", 0, fmt.Errorf("truncated rdata")
		}
		rdata := msg[off : off+rdlen]
		off += rdlen
		if rtype != dnsTypeHTTPS {
			continue
		}
		if len(rdata) < 3 {
			continue
		}
		p := 2 // priority
		if _, next, err := readName(rdata, p); err == nil {
			p = next
		} else {
			continue
		}
		for p+4 <= len(rdata) {
			key := binary.BigEndian.Uint16(rdata[p : p+2])
			vlen := int(binary.BigEndian.Uint16(rdata[p+2 : p+4]))
			p += 4
			if p+vlen > len(rdata) {
				break
			}
			val := rdata[p : p+vlen]
			p += vlen
			if key == svcParamECH && len(val) > 0 {
				return base64.StdEncoding.EncodeToString(val), ttl, nil
			}
		}
	}
	return "", 0, fmt.Errorf("no ech SvcParam in HTTPS record")
}

func dohHTTPSRecord(cfg Config, name string) (string, int, error) {
	var lastErr error
	for _, url := range cfg.ECH.Resolvers {
		query, id, qnameLen, err := buildDNSQuery(name, dnsTypeHTTPS)
		if err != nil {
			return "", 0, err
		}
		req, err := http.NewRequest("POST", url, bytes.NewReader(query))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/dns-message")
		req.Header.Set("Accept", "application/dns-message")
		client := &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Panel.InsecureTLS},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, dnsMsgMaxSize))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		b64, ttl, err := parseECHAnswers(body, id, qnameLen)
		if err != nil {
			lastErr = err
			continue
		}
		return b64, ttl, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no resolver configured")
	}
	return "", 0, lastErr
}

// ---- ECH cache -------------------------------------------------------------

type echCacheEntry struct {
	value   string
	ttl     int
	expires time.Time
	fetched time.Time
}

var (
	echMu    sync.Mutex
	echCache echCacheEntry
)

func getECHConfig(cfg Config, force bool) (string, int, time.Time, error) {
	echMu.Lock()
	if !force && echCache.value != "" && time.Now().Before(echCache.expires) {
		v, ttl, at := echCache.value, echCache.ttl, echCache.fetched
		echMu.Unlock()
		return v, ttl, at, nil
	}
	echMu.Unlock()

	value, ttl, err := dohHTTPSRecord(cfg, cfg.ECH.Domain)
	echMu.Lock()
	defer echMu.Unlock()
	if err != nil {
		if echCache.value != "" { // keep the last good value rather than break everyone
			return echCache.value, echCache.ttl, echCache.fetched, nil
		}
		return "", 0, time.Time{}, err
	}
	keep := ttl
	if keep < 60 {
		keep = 60
	}
	if keep > cfg.ECH.CacheSeconds {
		keep = cfg.ECH.CacheSeconds
	}
	echCache = echCacheEntry{value: value, ttl: keep,
		expires: time.Now().Add(time.Duration(keep) * time.Second), fetched: time.Now()}
	return value, keep, echCache.fetched, nil
}

func echPEMLines(b64 string) []string {
	return []string{"-----BEGIN ECH CONFIGS-----", b64, "-----END ECH CONFIGS-----", ""}
}
