package config

import (
	"encoding/json"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Difficulty != 8 {
		t.Fatalf("expected difficulty 8, got %d", cfg.Difficulty)
	}
	if cfg.RatePerMinute != 10 {
		t.Fatalf("expected rate 10, got %d", cfg.RatePerMinute)
	}
	if cfg.MaxMessages != 100_000 {
		t.Fatalf("expected max messages 100k, got %d", cfg.MaxMessages)
	}
}

func TestDurationJSON(t *testing.T) {
	d := Duration{72 * 3600 * 1e9} // 72h in nanoseconds
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}

	var d2 Duration
	if err := json.Unmarshal(data, &d2); err != nil {
		t.Fatal(err)
	}
	if d.Duration != d2.Duration {
		t.Fatalf("duration roundtrip failed: %v != %v", d.Duration, d2.Duration)
	}
}

func TestImportExport(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "/ip4/127.0.0.1/tcp/9999"
	cfg.Difficulty = 16

	jsonStr, err := ExportConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	cfg2, err := ImportConfig(jsonStr)
	if err != nil {
		t.Fatal(err)
	}

	if cfg2.ListenAddr != cfg.ListenAddr {
		t.Fatal("import/export listen_addr mismatch")
	}
	if cfg2.Difficulty != cfg.Difficulty {
		t.Fatal("import/export difficulty mismatch")
	}
}
