package main

import (
	"os"

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
	Addr          string `yaml:"addr"`
	TailnetDomain string `yaml:"tailnet_domain"`
}

type StorageConfig struct {
	DBPath          string `yaml:"db_path"`
	BlobDir         string `yaml:"blob_dir"`
	MaxBlobStorage  string `yaml:"max_blob_storage"`
	MaxUploadSize   string `yaml:"max_upload_size"`
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
			Addr:          "localhost:3000",
			TailnetDomain: "example.ts.net",
		},
		Storage: StorageConfig{
			DBPath:         "./data/tailchat.db",
			BlobDir:        "./data/blobs",
			MaxBlobStorage: "10GB",
			MaxUploadSize:  "100MB",
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
// Missing file is not an error — defaults are used.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
