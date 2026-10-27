package main

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all server configuration.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	Cleanup CleanupConfig `yaml:"cleanup"`
	Logging LoggingConfig `yaml:"logging"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	// Domain is the hostname users open over the VPN (e.g. chat.example.com).
	// It must resolve to the WireGuard server IP and is the only allowed Origin.
	Domain string `yaml:"domain"`
	// VPNCIDRs are the WireGuard peer subnets allowed to reach the app.
	VPNCIDRs []string `yaml:"vpn_cidrs"`
	// TrustedProxies are the direct peers (the reverse proxy) whose
	// X-Forwarded-For header is believed.
	TrustedProxies []string `yaml:"trusted_proxies"`
	// ExtraOrigins are additional browser Origins allowed for CORS/WebSocket.
	// Leave empty in production; useful only for local testing over http.
	ExtraOrigins []string `yaml:"extra_origins"`
}

type StorageConfig struct {
	DBPath         string `yaml:"db_path"`
	BlobDir        string `yaml:"blob_dir"`
	MaxBlobStorage string `yaml:"max_blob_storage"`
	MaxUploadSize  string `yaml:"max_upload_size"`
	MaxUserStorage string `yaml:"max_user_storage"` // per-account upload quota
}

type CleanupConfig struct {
	Interval string `yaml:"interval"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Addr:           "127.0.0.1:3000",
			Domain:         "chat.example.com",
			VPNCIDRs:       []string{"10.8.0.0/24"},
			TrustedProxies: []string{"127.0.0.1/32"},
		},
		Storage: StorageConfig{
			DBPath:         "./data/tailchat.db",
			BlobDir:        "./data/blobs",
			MaxBlobStorage: "10GB",
			MaxUploadSize:  "100MB",
			MaxUserStorage: "2GB",
		},
		Cleanup: CleanupConfig{
			Interval: "1h",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// LoadConfig reads config.yaml and returns a Config.
// Missing file is not an error — defaults are used. Environment variables
// (CHAT_ADDR, CHAT_DOMAIN, VPN_CIDRS, TRUSTED_PROXIES, DB_PATH, BLOB_DIR)
// override the file so the container can be configured from .env.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	if err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}

	envStr := func(key string, dst *string) {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			*dst = v
		}
	}
	envList := func(key string, dst *[]string) {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			*dst = strings.Split(v, ",")
		}
	}
	envStr("CHAT_ADDR", &cfg.Server.Addr)
	envStr("CHAT_DOMAIN", &cfg.Server.Domain)
	envList("VPN_CIDRS", &cfg.Server.VPNCIDRs)
	envList("TRUSTED_PROXIES", &cfg.Server.TrustedProxies)
	envList("EXTRA_ORIGINS", &cfg.Server.ExtraOrigins)
	envStr("DB_PATH", &cfg.Storage.DBPath)
	envStr("BLOB_DIR", &cfg.Storage.BlobDir)
	return cfg, nil
}
