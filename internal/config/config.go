package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-yaml/yaml"
)

// Config 汇总系统运行所需的全部配置。
type Config struct {
	App            AppConfig            `yaml:"app"`
	Auth           AuthConfig           `yaml:"auth"`
	DB             DBConfig             `yaml:"db"`
	Log            LogConfig            `yaml:"log"`
	Mail           MailConfig           `yaml:"mail"`
	Frontend       FrontendConfig       `yaml:"frontend"`
	MicrosoftOAuth MicrosoftOAuthConfig `yaml:"microsoft_oauth"`
}

// AppConfig 保存应用级配置。
type AppConfig struct {
	Name      string `yaml:"name"`
	Env       string `yaml:"env"`
	Addr      string `yaml:"addr"`
	SecretKey string `yaml:"secret_key"`
}

// AuthConfig 保存认证相关配置。
type AuthConfig struct {
	InitUsername string `yaml:"init_username"`
	JWTExpire    string `yaml:"jwt_expire"`
	CookieName   string `yaml:"cookie_name"`
}

// DBConfig 保存数据库连接配置。
type DBConfig struct {
	Driver          string `yaml:"driver"`
	DSN             string `yaml:"dsn"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
}

// LogConfig 保存结构化日志相关配置。
type LogConfig struct {
	Level string `yaml:"level"`
}

// MailConfig 保存邮件同步相关配置。
type MailConfig struct {
	SyncCron       string `yaml:"sync_cron"`
	MaxConcurrency int    `yaml:"max_concurrency"`
	FetchBody      bool   `yaml:"fetch_body"`
	PageSize       int    `yaml:"page_size"`
}

// FrontendConfig 保存前端构建路径配置。
type FrontendConfig struct {
	Dist string `yaml:"dist"`
}

// MicrosoftOAuthConfig 保存微软 OAuth 接入配置。
type MicrosoftOAuthConfig struct {
	TenantID     string `yaml:"tenant_id"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

// Load 按默认值、配置文件、环境变量的顺序加载配置。
func Load(path string) (*Config, error) {
	cfg := defaults()

	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	applyEnv(cfg)
	normalize(cfg)
	if cfg.App.SecretKey == "" {
		return nil, fmt.Errorf("app.secret_key 不能为空，可通过配置文件或 APP_SECRET_KEY 提供")
	}
	return cfg, nil
}

// JWTExpireDuration 将 JWT 过期配置转换为时长。
func (c Config) JWTExpireDuration() time.Duration {
	d, err := time.ParseDuration(c.Auth.JWTExpire)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

// ConnLifetimeDuration 将连接最长生存时间转换为时长。
func (c Config) ConnLifetimeDuration() time.Duration {
	d, err := time.ParseDuration(c.DB.ConnMaxLifetime)
	if err != nil {
		return time.Hour
	}
	return d
}

// DebugMode 返回是否开启调试日志模式；开启后统一放宽为 debug 等级并打印服务商交互日志。
func (c Config) DebugMode() bool {
	return strings.EqualFold(strings.TrimSpace(c.Log.Level), "debug")
}

// SlogLevel 将配置中的日志等级转换为 slog 级别。
func (c Config) SlogLevel() slog.Level {
	if c.DebugMode() {
		return slog.LevelDebug
	}
	switch strings.ToLower(strings.TrimSpace(c.Log.Level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// defaults 提供可回落的默认配置。
func defaults() *Config {
	return &Config{
		App: AppConfig{
			Name:      "gmbox",
			Env:       "dev",
			Addr:      ":8080",
			SecretKey: "change-me-32-bytes-secret-key-1234",
		},
		Auth: AuthConfig{
			InitUsername: "admin",
			JWTExpire:    "24h",
			CookieName:   "gmbox_token",
		},
		DB: DBConfig{
			Driver:          "sqlite",
			DSN:             "data/gmbox.db",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: "1h",
		},
		Log: LogConfig{
			Level: "info",
		},
		Mail: MailConfig{
			SyncCron:       "*/1 * * * *",
			MaxConcurrency: 5,
			FetchBody:      false,
			PageSize:       50,
		},
		Frontend: FrontendConfig{
			Dist: "frontend/dist",
		},
		MicrosoftOAuth: MicrosoftOAuthConfig{
			TenantID: "common",
		},
	}
}

// applyEnv 只对出现的环境变量执行强覆盖，不回落到文件值。
func applyEnv(cfg *Config) {
	setString(&cfg.App.Addr, os.Getenv("APP_ADDR"))
	setString(&cfg.App.Env, os.Getenv("APP_ENV"))
	setString(&cfg.App.SecretKey, os.Getenv("APP_SECRET_KEY"))
	setString(&cfg.Auth.InitUsername, os.Getenv("AUTH_INIT_USERNAME"))
	setString(&cfg.Auth.JWTExpire, os.Getenv("AUTH_JWT_EXPIRE"))
	setString(&cfg.Auth.CookieName, os.Getenv("AUTH_COOKIE_NAME"))
	setString(&cfg.DB.Driver, os.Getenv("DB_DRIVER"))
	setString(&cfg.DB.DSN, os.Getenv("DB_DSN"))
	setString(&cfg.Log.Level, os.Getenv("LOG_LEVEL"))
	setString(&cfg.Mail.SyncCron, os.Getenv("MAIL_SYNC_CRON"))

	setInt(&cfg.DB.MaxOpenConns, os.Getenv("DB_MAX_OPEN_CONNS"))
	setInt(&cfg.DB.MaxIdleConns, os.Getenv("DB_MAX_IDLE_CONNS"))
	setString(&cfg.DB.ConnMaxLifetime, os.Getenv("DB_CONN_MAX_LIFETIME"))
	setInt(&cfg.Mail.MaxConcurrency, os.Getenv("MAIL_MAX_CONCURRENCY"))
	setBool(&cfg.Mail.FetchBody, os.Getenv("MAIL_FETCH_BODY"))
	setInt(&cfg.Mail.PageSize, os.Getenv("MAIL_PAGE_SIZE"))
	setString(&cfg.Frontend.Dist, os.Getenv("FRONTEND_DIST"))
	setString(&cfg.MicrosoftOAuth.TenantID, os.Getenv("MICROSOFT_OAUTH_TENANT_ID"))
	setString(&cfg.MicrosoftOAuth.ClientID, os.Getenv("MICROSOFT_OAUTH_CLIENT_ID"))
	setString(&cfg.MicrosoftOAuth.ClientSecret, os.Getenv("MICROSOFT_OAUTH_CLIENT_SECRET"))
	setString(&cfg.MicrosoftOAuth.RedirectURL, os.Getenv("MICROSOFT_OAUTH_REDIRECT_URL"))
}

// normalize 对配置做最小兜底，避免非法值把运行时拖进阻塞或崩溃状态。
func normalize(cfg *Config) {
	if cfg.Mail.MaxConcurrency < 1 {
		cfg.Mail.MaxConcurrency = 1
	}
	if cfg.Mail.PageSize < 1 {
		cfg.Mail.PageSize = 50
	}
	if cfg.DB.MaxOpenConns < 1 {
		cfg.DB.MaxOpenConns = 10
	}
	if cfg.DB.MaxIdleConns < 1 {
		cfg.DB.MaxIdleConns = 5
	}
}

// setString 仅在环境变量非空时覆盖目标值。
func setString(target *string, value string) {
	if strings.TrimSpace(value) != "" {
		*target = value
	}
}

// setInt 仅在环境变量合法时覆盖目标值。
func setInt(target *int, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		*target = parsed
	}
}

// setBool 仅在环境变量合法时覆盖目标值。
func setBool(target *bool, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		*target = parsed
	}
}
