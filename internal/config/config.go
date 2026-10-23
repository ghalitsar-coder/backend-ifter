package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the fully resolved application configuration. Values come from
// configs/config.yaml (or CONFIG_FILE) and are overridden by APP_* environment
// variables, e.g. APP_HTTP_PORT or APP_DB_PASSWORD.
type Config struct {
	App  AppConfig  `mapstructure:"app"`
	HTTP HTTPConfig `mapstructure:"http"`
	DB   DBConfig   `mapstructure:"db"`
	Log  LogConfig  `mapstructure:"log"`
	Auth AuthConfig `mapstructure:"auth"`
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
	MaxBodyBytes      int64         `mapstructure:"max_body_bytes"`
	RateLimitRequests int           `mapstructure:"rate_limit_requests"`
	RateLimitWindow   time.Duration `mapstructure:"rate_limit_window"`
	TrustedProxies    []string      `mapstructure:"trusted_proxies"`
	AllowedOrigins    []string      `mapstructure:"allowed_origins"`
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
	JWTSecret      string        `mapstructure:"jwt_secret"`
	JWTIssuer      string        `mapstructure:"jwt_issuer"`
	JWTAudience    string        `mapstructure:"jwt_audience"`
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`
}

func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("APP")
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
	v.SetDefault("app.name", "backend-skeleton")
	v.SetDefault("app.env", "development")

	v.SetDefault("http.host", "0.0.0.0")
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", 15*time.Second)
	v.SetDefault("http.read_header_timeout", 5*time.Second)
	v.SetDefault("http.write_timeout", 15*time.Second)
	v.SetDefault("http.idle_timeout", 60*time.Second)
	v.SetDefault("http.shutdown_timeout", 10*time.Second)
	v.SetDefault("http.max_body_bytes", 1<<20)
	v.SetDefault("http.rate_limit_requests", 120)
	v.SetDefault("http.rate_limit_window", time.Minute)
	v.SetDefault("http.trusted_proxies", []string{})
	v.SetDefault("http.allowed_origins", []string{})

	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.user", "app")
	v.SetDefault("db.password", "")
	v.SetDefault("db.name", "app")
	v.SetDefault("db.sslmode", "disable")
	v.SetDefault("db.max_conns", 10)
	v.SetDefault("db.min_conns", 0)
	v.SetDefault("db.connect_timeout", 5*time.Second)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	// No default secret on purpose: the service must fail fast when it is not
	// configured instead of silently signing tokens with a well-known value.
	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("auth.jwt_issuer", "backend-skeleton")
	v.SetDefault("auth.jwt_audience", "backend-skeleton-api")
	v.SetDefault("auth.access_token_ttl", 15*time.Minute)
}

func (c *Config) normalize() {
	c.App.Env = strings.ToLower(strings.TrimSpace(c.App.Env))
	c.DB.SSLMode = strings.ToLower(strings.TrimSpace(c.DB.SSLMode))
	c.Log.Level = strings.ToLower(strings.TrimSpace(c.Log.Level))
	c.Log.Format = strings.ToLower(strings.TrimSpace(c.Log.Format))
	c.Auth.JWTIssuer = strings.TrimSpace(c.Auth.JWTIssuer)
	c.Auth.JWTAudience = strings.TrimSpace(c.Auth.JWTAudience)

	proxies := make([]string, 0, len(c.HTTP.TrustedProxies))
	for _, proxy := range c.HTTP.TrustedProxies {
		if proxy = strings.TrimSpace(proxy); proxy != "" {
			proxies = append(proxies, proxy)
		}
	}
	c.HTTP.TrustedProxies = proxies

	origins := make([]string, 0, len(c.HTTP.AllowedOrigins))
	for _, origin := range c.HTTP.AllowedOrigins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	c.HTTP.AllowedOrigins = origins
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
	if c.HTTP.MaxBodyBytes < 1024 {
		return errors.New("http.max_body_bytes must be at least 1024")
	}
	if c.HTTP.RateLimitRequests < 1 {
		return errors.New("http.rate_limit_requests must be at least 1")
	}
	if c.HTTP.RateLimitWindow <= 0 {
		return errors.New("http.rate_limit_window must be greater than zero")
	}
	for _, proxy := range c.HTTP.TrustedProxies {
		if net.ParseIP(proxy) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(proxy); err != nil {
			return fmt.Errorf("http.trusted_proxies contains an invalid IP or CIDR %q", proxy)
		}
	}
	for _, origin := range c.HTTP.AllowedOrigins {
		u, err := url.Parse(origin)
		if origin != "*" && (err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
			u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "") {
			return fmt.Errorf("http.allowed_origins contains an invalid origin %q", origin)
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
