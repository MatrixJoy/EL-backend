package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("VOA_ENV", "")
	t.Setenv("VOA_HTTP_ADDR", "")
	t.Setenv("VOA_SHUTDOWN_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("VOA_FETCH_TIMEOUT", "tomorrow")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}
