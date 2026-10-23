package config

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App     AppConfig     `mapstructure:"app"`
	HTTP    HTTPConfig    `mapstructure:"http"`
	DB      DBConfig      `mapstructure:"db"`
	Log     LogConfig     `mapstructure:"log"`
	Auth    AuthConfig    `mapstructure:"auth"`
	Imports ImportsConfig `mapstructure:"imports"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type HTTPConfig struct {
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	TrustedProxies    []string      `mapstructure:"trusted_proxies"`
}

type DBConfig struct {
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	User           string        `mapstructure:"user"`
	Password       string        `mapstructure:"password"`
	Name           string        `mapstructure:"name"`
	SSLMode        string        `mapstructure:"sslmode"`
	MaxConns       int32         `mapstructure:"max_conns"`
	MinConns       int32         `mapstructure:"min_conns"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type AuthConfig struct {
	JWTSecret       string               `mapstructure:"jwt_secret"`
	JWTIssuer       string               `mapstructure:"jwt_issuer"`
	AccessTokenTTL  time.Duration        `mapstructure:"access_token_ttl"`
	RefreshTokenTTL time.Duration        `mapstructure:"refresh_token_ttl"`
	InviteTokenTTL  time.Duration        `mapstructure:"invite_token_ttl"`
	ResetTokenTTL   time.Duration        `mapstructure:"reset_token_ttl"`
	CookieSecure    bool                 `mapstructure:"cookie_secure"`
	BcryptCost      int                  `mapstructure:"bcrypt_cost"`
	AllowedOrigins  []string             `mapstructure:"allowed_origins"`
	LoginRateLimit  int                  `mapstructure:"login_rate_limit"`
	LoginRateWindow time.Duration        `mapstructure:"login_rate_window"`
	BootstrapAdmin  BootstrapAdminConfig `mapstructure:"bootstrap_admin"`
}

type BootstrapAdminConfig struct {
	Username string `mapstructure:"username"`
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
	FullName string `mapstructure:"full_name"`
}

type ImportsConfig struct {
	Dir            string        `mapstructure:"dir"`
	MaxFileSize    int64         `mapstructure:"max_file_size"`
	MaxRows        int           `mapstructure:"max_rows"`
	FileRetention  time.Duration `mapstructure:"file_retention"`
	ErrorRetention time.Duration `mapstructure:"error_retention"`
}

func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("IFTER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path := os.Getenv("CONFIG_FILE"); path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file %q: %w", path, err)
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("configs")
		v.AddConfigPath(".")
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return nil, fmt.Errorf("read config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "backend-ifter")
	v.SetDefault("app.env", "development")

	v.SetDefault("http.host", "0.0.0.0")
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", 10*time.Second)
	v.SetDefault("http.read_header_timeout", 5*time.Second)
	v.SetDefault("http.write_timeout", 15*time.Second)
	v.SetDefault("http.idle_timeout", 60*time.Second)
	v.SetDefault("http.shutdown_timeout", 10*time.Second)
	v.SetDefault("http.trusted_proxies", []string{})

	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.user", "ifter")
	v.SetDefault("db.password", "")
	v.SetDefault("db.name", "ifter")
	v.SetDefault("db.sslmode", "disable")
	v.SetDefault("db.max_conns", 10)
	v.SetDefault("db.min_conns", 0)
	v.SetDefault("db.connect_timeout", 5*time.Second)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("auth.jwt_issuer", "backend-ifter")
	v.SetDefault("auth.access_token_ttl", 15*time.Minute)
	v.SetDefault("auth.refresh_token_ttl", 7*24*time.Hour)
	v.SetDefault("auth.invite_token_ttl", 72*time.Hour)
	v.SetDefault("auth.reset_token_ttl", time.Hour)
	v.SetDefault("auth.cookie_secure", false)
	v.SetDefault("auth.bcrypt_cost", 12)
	v.SetDefault("auth.allowed_origins", []string{})
	v.SetDefault("auth.login_rate_limit", 30)
	v.SetDefault("auth.login_rate_window", time.Minute)
	v.SetDefault("auth.bootstrap_admin.username", "")
	v.SetDefault("auth.bootstrap_admin.email", "")
	v.SetDefault("auth.bootstrap_admin.password", "")
	v.SetDefault("auth.bootstrap_admin.full_name", "")

	v.SetDefault("imports.dir", "./storage/imports")
	v.SetDefault("imports.max_file_size", 5<<20)
	v.SetDefault("imports.max_rows", 2000)
	v.SetDefault("imports.file_retention", 7*24*time.Hour)
	v.SetDefault("imports.error_retention", 30*24*time.Hour)
}

func (c *Config) normalize() {
	c.App.Env = strings.ToLower(strings.TrimSpace(c.App.Env))
	proxies := make([]string, 0, len(c.HTTP.TrustedProxies))
	for _, proxy := range c.HTTP.TrustedProxies {
		if proxy = strings.TrimSpace(proxy); proxy != "" {
			proxies = append(proxies, proxy)
		}
	}
	c.HTTP.TrustedProxies = proxies
	c.DB.SSLMode = strings.ToLower(strings.TrimSpace(c.DB.SSLMode))
	c.Log.Level = strings.ToLower(strings.TrimSpace(c.Log.Level))
	c.Log.Format = strings.ToLower(strings.TrimSpace(c.Log.Format))

	c.Auth.JWTIssuer = strings.TrimSpace(c.Auth.JWTIssuer)
	c.Auth.BootstrapAdmin.Username = strings.TrimSpace(c.Auth.BootstrapAdmin.Username)
	c.Auth.BootstrapAdmin.Email = strings.TrimSpace(c.Auth.BootstrapAdmin.Email)
	c.Auth.BootstrapAdmin.FullName = strings.TrimSpace(c.Auth.BootstrapAdmin.FullName)
	if c.Auth.BootstrapAdmin.Username != "" && c.Auth.BootstrapAdmin.FullName == "" {
		c.Auth.BootstrapAdmin.FullName = "Administrator"
	}
	origins := make([]string, 0, len(c.Auth.AllowedOrigins))
	for _, origin := range c.Auth.AllowedOrigins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	c.Auth.AllowedOrigins = origins
	c.Imports.Dir = strings.TrimSpace(c.Imports.Dir)
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.App.Name) == "" {
		return errors.New("app.name is required")
	}
	switch c.App.Env {
	case "development", "staging", "production":
	default:
		return fmt.Errorf("app.env must be development, staging, or production (got %q)", c.App.Env)
	}

	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port must be between 1 and 65535 (got %d)", c.HTTP.Port)
	}
	for name, value := range map[string]time.Duration{
		"http.read_timeout":        c.HTTP.ReadTimeout,
		"http.read_header_timeout": c.HTTP.ReadHeaderTimeout,
		"http.write_timeout":       c.HTTP.WriteTimeout,
		"http.idle_timeout":        c.HTTP.IdleTimeout,
		"http.shutdown_timeout":    c.HTTP.ShutdownTimeout,
	} {
		if value <= 0 {
			return fmt.Errorf("%s must be greater than zero", name)
		}
	}
	for _, proxy := range c.HTTP.TrustedProxies {
		if net.ParseIP(proxy) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(proxy); err != nil {
			return fmt.Errorf("http.trusted_proxies contains an invalid IP or CIDR %q", proxy)
		}
	}

	if strings.TrimSpace(c.DB.Host) == "" {
		return errors.New("db.host is required")
	}
	if c.DB.Port < 1 || c.DB.Port > 65535 {
		return fmt.Errorf("db.port must be between 1 and 65535 (got %d)", c.DB.Port)
	}
	if strings.TrimSpace(c.DB.User) == "" {
		return errors.New("db.user is required")
	}
	if strings.TrimSpace(c.DB.Name) == "" {
		return errors.New("db.name is required")
	}
	switch c.DB.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("db.sslmode must be disable, require, verify-ca, or verify-full (got %q)", c.DB.SSLMode)
	}
	if c.DB.MaxConns < 1 {
		return errors.New("db.max_conns must be at least 1")
	}
	if c.DB.MinConns < 0 || c.DB.MinConns > c.DB.MaxConns {
		return errors.New("db.min_conns must be between 0 and db.max_conns")
	}
	if c.DB.ConnectTimeout <= 0 {
		return errors.New("db.connect_timeout must be greater than zero")
	}
	if c.App.Env == "production" && c.DB.Password == "" {
		return errors.New("db.password is required in production")
	}

	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level must be debug, info, warn, or error (got %q)", c.Log.Level)
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("log.format must be json or text (got %q)", c.Log.Format)
	}

	if len(c.Auth.JWTSecret) < 32 {
		return errors.New("auth.jwt_secret must be at least 32 characters")
	}
	if c.Auth.JWTIssuer == "" {
		return errors.New("auth.jwt_issuer is required")
	}
	if c.Auth.AccessTokenTTL <= 0 {
		return errors.New("auth.access_token_ttl must be greater than zero")
	}
	if c.Auth.RefreshTokenTTL <= c.Auth.AccessTokenTTL {
		return errors.New("auth.refresh_token_ttl must be greater than auth.access_token_ttl")
	}
	if c.Auth.InviteTokenTTL <= 0 {
		return errors.New("auth.invite_token_ttl must be greater than zero")
	}
	if c.Auth.ResetTokenTTL <= 0 {
		return errors.New("auth.reset_token_ttl must be greater than zero")
	}
	if c.Auth.BcryptCost < 10 || c.Auth.BcryptCost > 14 {
		return fmt.Errorf("auth.bcrypt_cost must be between 10 and 14 (got %d)", c.Auth.BcryptCost)
	}
	if c.Auth.LoginRateLimit < 1 {
		return errors.New("auth.login_rate_limit must be at least 1")
	}
	if c.Auth.LoginRateWindow <= 0 {
		return errors.New("auth.login_rate_window must be greater than zero")
	}
	if c.App.Env == "production" && !c.Auth.CookieSecure {
		return errors.New("auth.cookie_secure must be true in production")
	}
	for _, origin := range c.Auth.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("auth.allowed_origins contains an invalid origin %q", origin)
		}
	}

	if c.Imports.Dir == "" {
		return errors.New("imports.dir is required")
	}
	if c.Imports.MaxFileSize <= 0 {
		return errors.New("imports.max_file_size must be greater than zero")
	}
	if c.Imports.MaxRows < 1 {
		return errors.New("imports.max_rows must be at least 1")
	}
	if c.Imports.FileRetention <= 0 {
		return errors.New("imports.file_retention must be greater than zero")
	}
	if c.Imports.ErrorRetention <= 0 {
		return errors.New("imports.error_retention must be greater than zero")
	}

	admin := c.Auth.BootstrapAdmin
	adminSet := admin.Username != "" || admin.Email != "" || admin.Password != "" || admin.FullName != ""
	if adminSet {
		if admin.Username == "" {
			return errors.New("auth.bootstrap_admin.username is required when bootstrap is configured")
		}
		if len(admin.Password) < 12 {
			return errors.New("auth.bootstrap_admin.password must be at least 12 characters")
		}
		if len(admin.Password) > 72 {
			return errors.New("auth.bootstrap_admin.password must be at most 72 bytes")
		}
		if admin.Email != "" {
			addr, err := mail.ParseAddress(admin.Email)
			if err != nil || addr.Address != admin.Email {
				return fmt.Errorf("auth.bootstrap_admin.email is invalid: %q", admin.Email)
			}
		}
	}
	return nil
}

func (h HTTPConfig) Addr() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

func (d DBConfig) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(d.Host, strconv.Itoa(d.Port)),
		Path:   d.Name,
	}
	if d.Password == "" {
		u.User = url.User(d.User)
	} else {
		u.User = url.UserPassword(d.User, d.Password)
	}
	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}
