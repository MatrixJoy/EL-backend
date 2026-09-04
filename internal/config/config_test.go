package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LEARNING_ENV", "")
	t.Setenv("LEARNING_HTTP_ADDR", "")
	t.Setenv("LEARNING_SHUTDOWN_TIMEOUT", "")
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
	t.Setenv("LEARNING_FETCH_TIMEOUT", "tomorrow")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestLoadPrefersNeutralEnvironmentNames(t *testing.T) {
	t.Setenv("LEARNING_HTTP_ADDR", ":9090")
	t.Setenv("VOA_HTTP_ADDR", ":8080")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr=%q want=:9090", cfg.HTTPAddr)
	}
}
