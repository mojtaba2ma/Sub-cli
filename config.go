package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config mirrors config.example.json. Defaults are applied first and the file
// is unmarshalled on top, so a partial config keeps sane defaults everywhere.
type Config struct {
	Listen   string  `json:"listen"`
	Port     int     `json:"port"`
	BasePath string  `json:"base_path"`
	Secret   string  `json:"secret"`
	Panel    Panel   `json:"panel"`
	ECH      ECH     `json:"ech"`
	Profile  Profile `json:"profile"`
	Response Resp    `json:"response"`
}

type Panel struct {
	SubURL         string `json:"sub_url"`
	UserAgent      string `json:"user_agent"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	InsecureTLS    bool   `json:"insecure_tls"`
	HostHeader     string `json:"host_header"`
}

type ECH struct {
	Enabled        bool     `json:"enabled"`
	Domain         string   `json:"domain"`
	Resolvers      []string `json:"resolvers"`
	Mode           string   `json:"mode"` // all | domain-list | off
	Domains        []string `json:"domains"`
	CacheSeconds   int      `json:"cache_seconds"`
	StaticEndpoint bool     `json:"static_endpoint"`
}

type Profile struct {
	LogLevel        string   `json:"log_level"`
	DNSDirect       []string `json:"dns_direct"`
	DNSEch          string   `json:"dns_ech"`
	Strategy        string   `json:"strategy"`
	Fingerprint     string   `json:"fingerprint"`
	IncludeTun      bool     `json:"include_tun"`
	MixedPort       int      `json:"mixed_port"`
	URLTestURL      string   `json:"urltest_url"`
	URLTestInterval string   `json:"urltest_interval"`
}

type Resp struct {
	ProfileUpdateIntervalHours int    `json:"profile_update_interval_hours"`
	ProfileTitle               string `json:"profile_title"`
}

func defaultConfig() Config {
	return Config{
		Listen:   "127.0.0.1",
		Port:     2199,
		BasePath: "/sb",
		Secret:   "CHANGE-ME-to-a-long-random-string",
		Panel: Panel{
			SubURL:         "http://127.0.0.1:2096/sub/{subid}",
			UserAgent:      "v2rayNG/1.9.44",
			TimeoutSeconds: 15,
		},
		ECH: ECH{
			Enabled:        true,
			Domain:         "cloudflare-ech.com",
			Resolvers:      []string{"https://8.8.8.8/dns-query", "https://1.1.1.1/dns-query"},
			Mode:           "all",
			CacheSeconds:   300,
			StaticEndpoint: true,
		},
		Profile: Profile{
			LogLevel:        "warn",
			DNSDirect:       []string{"8.8.8.8", "1.1.1.1"},
			DNSEch:          "1.1.1.1",
			Strategy:        "prefer_ipv4",
			Fingerprint:     "chrome",
			IncludeTun:      true,
			MixedPort:       2080,
			URLTestURL:      "http://cp.cloudflare.com/generate_204",
			URLTestInterval: "3m",
		},
		Response: Resp{ProfileUpdateIntervalHours: 6},
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	if cfg.Panel.SubURL == "" {
		cfg.Panel.SubURL = defaultConfig().Panel.SubURL
	}
	if cfg.Panel.UserAgent == "" {
		cfg.Panel.UserAgent = defaultConfig().Panel.UserAgent
	}
	if cfg.Panel.TimeoutSeconds <= 0 {
		cfg.Panel.TimeoutSeconds = 15
	}
	if cfg.ECH.Domain == "" {
		cfg.ECH.Domain = "cloudflare-ech.com"
	}
	if len(cfg.ECH.Resolvers) == 0 {
		cfg.ECH.Resolvers = defaultConfig().ECH.Resolvers
	}
	if cfg.ECH.CacheSeconds <= 0 {
		cfg.ECH.CacheSeconds = 300
	}
	if len(cfg.Profile.DNSDirect) == 0 {
		cfg.Profile.DNSDirect = defaultConfig().Profile.DNSDirect
	}
	if cfg.Profile.DNSEch == "" {
		cfg.Profile.DNSEch = "1.1.1.1"
	}
	if cfg.Profile.Strategy == "" {
		cfg.Profile.Strategy = "prefer_ipv4"
	}
	if cfg.Profile.MixedPort <= 0 {
		cfg.Profile.MixedPort = 2080
	}
	if cfg.Profile.URLTestURL == "" {
		cfg.Profile.URLTestURL = defaultConfig().Profile.URLTestURL
	}
	if cfg.Profile.URLTestInterval == "" {
		cfg.Profile.URLTestInterval = defaultConfig().Profile.URLTestInterval
	}
	if cfg.Response.ProfileUpdateIntervalHours <= 0 {
		cfg.Response.ProfileUpdateIntervalHours = 6
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "/sb"
	}
	return cfg, nil
}
