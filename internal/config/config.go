package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config is the top-level node configuration.
type Config struct {
	// Identity
	DataDir string `json:"data_dir"`

	// Network
	ListenAddr string   `json:"listen_addr"`
	Bootstrap  []string `json:"bootstrap"`
	Rendezvous string   `json:"rendezvous"`
	TopicName  string   `json:"topic_name"`

	// PoW
	Difficulty int `json:"difficulty"` // leading zero bits

	// Rate Limiting
	RatePerMinute int `json:"rate_per_minute"`
	RateBurst     int `json:"rate_burst"`

	// Storage
	MaxMessages int           `json:"max_messages"` // 0 = unlimited
	MaxAge      Duration      `json:"max_age"`      // 0 = unlimited
	PruneInterval Duration    `json:"prune_interval"`

	// Trust
	TrustAlpha float64 `json:"trust_alpha"` // indirect trust weight
	TrustDecay float64 `json:"trust_decay"` // per-hop decay

	// Display
	DefaultView string `json:"default_view"`
	PageSize    int    `json:"page_size"`

	// Replay
	MaxMsgAge Duration `json:"max_msg_age"`

	// Logging
	Verbose bool `json:"verbose"`
}

// Duration wraps time.Duration for JSON marshaling.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try as number (seconds)
		var secs int64
		if err := json.Unmarshal(data, &secs); err != nil {
			return err
		}
		d.Duration = time.Duration(secs) * time.Second
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

// DefaultConfig returns production defaults.
func DefaultConfig() Config {
	return Config{
		ListenAddr:    "/ip4/0.0.0.0/tcp/4001",
		Rendezvous:    "dmsg-v1",
		TopicName:     "dmsg-global",
		Difficulty:    8,
		RatePerMinute: 10,
		RateBurst:     20,
		MaxMessages:   100_000,
		MaxAge:        Duration{72 * time.Hour},
		PruneInterval: Duration{1 * time.Hour},
		TrustAlpha:    0.5,
		TrustDecay:    0.5,
		DefaultView:   "All",
		PageSize:      50,
		MaxMsgAge:     Duration{24 * time.Hour},
		Verbose:       false,
	}
}

// Load reads config from <dataDir>/config.json, returns defaults if not found.
func Load(dataDir string) (Config, error) {
	path := filepath.Join(dataDir, "config.json")
	cfg := DefaultConfig()
	cfg.DataDir = dataDir

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Write defaults
			Save(cfg)
			return cfg, nil
		}
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.DataDir = dataDir
	return cfg, nil
}

// Save writes config to <dataDir>/config.json.
func Save(cfg Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(cfg.DataDir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ExportConfig exports config as JSON string.
func ExportConfig(cfg Config) (string, error) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	return string(data), err
}

// ImportConfig imports config from JSON string.
func ImportConfig(jsonStr string) (Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
