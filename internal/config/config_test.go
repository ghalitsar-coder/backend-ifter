package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromFile(t *testing.T) {
	path := writeConfig(t, `
app:
  name: test-app
  env: development
http:
  host: 127.0.0.1
  port: 9090
  read_timeout: 2s
  read_header_timeout: 1s
  write_timeout: 3s
  idle_timeout: 4s
  shutdown_timeout: 5s
  trusted_proxies:
    - 127.0.0.1/32
db:
  host: db
  port: 5433
  user: app
  password: secret
  name: school
  sslmode: disable
  max_conns: 5
  min_conns: 1
  connect_timeout: 2s
log:
  level: debug
  format: text
auth:
  jwt_secret: test-secret-at-least-32-characters-long
  jwt_issuer: test-issuer
  access_token_ttl: 10m
  refresh_token_ttl: 24h
  cookie_secure: false
  bcrypt_cost: 10
  allowed_origins:
    - http://localhost:3000/
  login_rate_limit: 5
  login_rate_window: 30s
hardening:
  metrics_token: metrics-secret
  ingest_rate_limit: 120
  ingest_rate_window: 30s
maintenance:
  audit_log_retention: 8760h
  refresh_token_retention: 168h
  interval: 6h
`)
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "test-app" || cfg.App.Env != "development" {
		t.Fatalf("unexpected app config: %+v", cfg.App)
	}
	if cfg.HTTP.Addr() != "127.0.0.1:9090" {
		t.Fatalf("Addr() = %q", cfg.HTTP.Addr())
	}
	if cfg.HTTP.ReadTimeout != 2*time.Second {
		t.Fatalf("ReadTimeout = %v", cfg.HTTP.ReadTimeout)
	}
	if len(cfg.HTTP.TrustedProxies) != 1 || cfg.HTTP.TrustedProxies[0] != "127.0.0.1/32" {
		t.Fatalf("TrustedProxies = %#v", cfg.HTTP.TrustedProxies)
	}
	if cfg.DB.Port != 5433 || cfg.DB.MaxConns != 5 || cfg.DB.MinConns != 1 {
		t.Fatalf("unexpected db config: %+v", cfg.DB)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
	}
	if cfg.Auth.JWTIssuer != "test-issuer" || cfg.Auth.AccessTokenTTL != 10*time.Minute {
		t.Fatalf("unexpected auth config: %+v", cfg.Auth)
	}
	if cfg.Auth.InviteTokenTTL != 72*time.Hour || cfg.Auth.ResetTokenTTL != time.Hour {
		t.Fatalf("unexpected credential token ttl config: %+v", cfg.Auth)
	}
	if cfg.Imports.Dir != "./storage/imports" || cfg.Imports.MaxFileSize != 5<<20 || cfg.Imports.MaxRows != 2000 {
		t.Fatalf("unexpected imports config: %+v", cfg.Imports)
	}
	if len(cfg.Auth.AllowedOrigins) != 1 || cfg.Auth.AllowedOrigins[0] != "http://localhost:3000" {
		t.Fatalf("unexpected allowed origins: %#v", cfg.Auth.AllowedOrigins)
	}
	if cfg.Hardening.MetricsToken != "metrics-secret" ||
		cfg.Hardening.IngestRateLimit != 120 ||
		cfg.Hardening.IngestRateWindow != 30*time.Second {
		t.Fatalf("unexpected hardening config: %+v", cfg.Hardening)
	}
	if cfg.Maintenance.AuditLogRetention != 8760*time.Hour ||
		cfg.Maintenance.RefreshTokenRetention != 168*time.Hour ||
		cfg.Maintenance.Interval != 6*time.Hour {
		t.Fatalf("unexpected maintenance config: %+v", cfg.Maintenance)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	const secret = "test-secret-at-least-32-characters-long"
	tests := []struct {
		name  string
		yaml  string
		error string
	}{
		{
			name:  "invalid port",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nhttp:\n  port: 70000\n",
			error: "http.port",
		},
		{
			name:  "invalid log level",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nlog:\n  level: verbose\n",
			error: "log.level",
		},
		{
			name:  "production without database password",
			yaml:  "app:\n  env: production\ndb:\n  password: \"\"\nauth:\n  jwt_secret: " + secret + "\n",
			error: "db.password",
		},
		{
			name:  "invalid trusted proxy",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nhttp:\n  trusted_proxies:\n    - not-a-cidr\n",
			error: "http.trusted_proxies",
		},
		{
			name:  "short jwt secret",
			yaml:  "auth:\n  jwt_secret: too-short\n",
			error: "auth.jwt_secret",
		},
		{
			name:  "refresh ttl not longer than access ttl",
			yaml:  "auth:\n  jwt_secret: " + secret + "\n  access_token_ttl: 2h\n  refresh_token_ttl: 1h\n",
			error: "auth.refresh_token_ttl",
		},
		{
			name:  "invalid invite token ttl",
			yaml:  "auth:\n  jwt_secret: " + secret + "\n  invite_token_ttl: 0s\n",
			error: "auth.invite_token_ttl",
		},
		{
			name:  "invalid reset token ttl",
			yaml:  "auth:\n  jwt_secret: " + secret + "\n  reset_token_ttl: -1h\n",
			error: "auth.reset_token_ttl",
		},
		{
			name:  "production without secure cookie",
			yaml:  "app:\n  env: production\ndb:\n  password: secret\n  sslmode: require\nauth:\n  jwt_secret: " + secret + "\n  cookie_secure: false\n",
			error: "auth.cookie_secure",
		},
		{
			name:  "production with sslmode disable",
			yaml:  "app:\n  env: production\ndb:\n  password: secret\n  sslmode: disable\nauth:\n  jwt_secret: " + secret + "\n  cookie_secure: true\n",
			error: "db.sslmode",
		},
		{
			name:  "invalid ingest rate limit",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nhardening:\n  ingest_rate_limit: 0\n",
			error: "hardening.ingest_rate_limit",
		},
		{
			name:  "invalid ingest rate window",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nhardening:\n  ingest_rate_window: -1s\n",
			error: "hardening.ingest_rate_window",
		},
		{
			name:  "negative audit retention",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nmaintenance:\n  audit_log_retention: -1h\n",
			error: "maintenance.audit_log_retention",
		},
		{
			name:  "negative refresh token retention",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nmaintenance:\n  refresh_token_retention: -1h\n",
			error: "maintenance.refresh_token_retention",
		},
		{
			name:  "invalid maintenance interval",
			yaml:  "auth:\n  jwt_secret: " + secret + "\nmaintenance:\n  interval: 0s\n",
			error: "maintenance.interval",
		},
		{
			name:  "invalid bcrypt cost",
			yaml:  "auth:\n  jwt_secret: " + secret + "\n  bcrypt_cost: 4\n",
			error: "auth.bcrypt_cost",
		},
		{
			name:  "partial bootstrap admin",
			yaml:  "auth:\n  jwt_secret: " + secret + "\n  bootstrap_admin:\n    password: change-this-password\n",
			error: "auth.bootstrap_admin.username",
		},
		{
			name:  "invalid allowed origin",
			yaml:  "auth:\n  jwt_secret: " + secret + "\n  allowed_origins:\n    - ftp://example.com\n",
			error: "auth.allowed_origins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CONFIG_FILE", writeConfig(t, tt.yaml))
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.error) {
				t.Fatalf("Load() error = %v, want containing %q", err, tt.error)
			}
		})
	}
}

func TestLoadAllowedOriginsFromEnvironment(t *testing.T) {
	t.Setenv("CONFIG_FILE", writeConfig(t, "auth:\n  jwt_secret: test-secret-at-least-32-characters-long\n"))
	t.Setenv("IFTER_AUTH_ALLOWED_ORIGINS", "http://localhost:3000,https://school.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Auth.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins = %#v, want 2 entries", cfg.Auth.AllowedOrigins)
	}
}

func TestLoadBootstrapAdminFromEnvironment(t *testing.T) {
	t.Setenv("CONFIG_FILE", writeConfig(t, "auth:\n  jwt_secret: test-secret-at-least-32-characters-long\n"))
	t.Setenv("IFTER_AUTH_BOOTSTRAP_ADMIN_USERNAME", "admin")
	t.Setenv("IFTER_AUTH_BOOTSTRAP_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("IFTER_AUTH_BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-password")
	t.Setenv("IFTER_AUTH_BOOTSTRAP_ADMIN_FULL_NAME", "Administrator Sekolah")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.BootstrapAdmin.Username != "admin" ||
		cfg.Auth.BootstrapAdmin.Email != "admin@example.com" ||
		cfg.Auth.BootstrapAdmin.Password != "bootstrap-password" {
		t.Fatalf("unexpected bootstrap config: %+v", cfg.Auth.BootstrapAdmin)
	}
}

func TestLoadRejectsMissingExplicitFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "missing.yaml"))

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestDSNEscapesCredentials(t *testing.T) {
	db := DBConfig{
		Host: "localhost", Port: 5432, User: "app", Password: "p@ss word",
		Name: "school", SSLMode: "disable",
	}

	dsn := db.DSN()
	if !strings.Contains(dsn, "p%40ss%20word") {
		t.Fatalf("DSN() = %q, want escaped password", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("DSN() = %q, want sslmode", dsn)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
