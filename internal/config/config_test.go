package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("APP_HTTP_PORT", "9099")
	t.Setenv("APP_DB_NAME", "skeleton_test")
	t.Setenv("APP_AUTH_JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("APP_LOG_FORMAT", "text")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Port != 9099 {
		t.Errorf("HTTP.Port = %d, want 9099", cfg.HTTP.Port)
	}
	if cfg.DB.Name != "skeleton_test" {
		t.Errorf("DB.Name = %q, want skeleton_test", cfg.DB.Name)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want text", cfg.Log.Format)
	}
	if cfg.HTTP.ReadTimeout != 15*time.Second {
		t.Errorf("HTTP.ReadTimeout = %v, want default 15s", cfg.HTTP.ReadTimeout)
	}
}

func TestLoadRejectsShortSecret(t *testing.T) {
	t.Setenv("APP_AUTH_JWT_SECRET", "short")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with a short JWT secret, want error")
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("APP_AUTH_JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("APP_APP_ENV", "stagingish")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with an invalid app.env, want error")
	}
}

func TestDSN(t *testing.T) {
	cfg := DBConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "app",
		Password: "secret",
		Name:     "app",
		SSLMode:  "disable",
	}

	want := "postgres://app:secret@localhost:5432/app?sslmode=disable"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}
