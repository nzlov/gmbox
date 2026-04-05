package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	utilsdb "github.com/nzlov/utils/db"
	"gorm.io/gorm"

	"gmbox/internal/auth"
	appcfg "gmbox/internal/config"
	"gmbox/internal/crypto"
	"gmbox/internal/mail"
	"gmbox/internal/model"
)

// App 聚合运行时依赖，避免在 handler 中重复组装对象。
type App struct {
	Config  *appcfg.Config
	DB      *gorm.DB
	SQLDB   *sql.DB
	JWT     *auth.JWTService
	Crypto  *crypto.AESService
	Syncer  *mail.Syncer
	Mailer  *mail.Service
	closers []func() error
}

// New 完成数据库、管理员和调度器的初始化。
func New(ctx context.Context, cfg *appcfg.Config) (*App, error) {
	if err := ensureDataDir(cfg.DB); err != nil {
		return nil, err
	}

	dbcfg := &utilsdb.Config{Driver: cfg.DB.Driver, URL: cfg.DB.DSN}
	db, err := dbcfg.Open()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层数据库失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnLifetimeDuration())

	if err := migrate(db); err != nil {
		return nil, err
	}
	if err := initAdmin(db, cfg); err != nil {
		return nil, err
	}

	app := &App{
		Config:  cfg,
		DB:      db,
		SQLDB:   sqlDB,
		JWT:     auth.NewJWTService(cfg.App.SecretKey, cfg.JWTExpireDuration()),
		Crypto:  crypto.NewAESService(cfg.App.SecretKey),
		closers: []func() error{sqlDB.Close},
	}
	app.Mailer = mail.NewService(db, app.Crypto, cfg)
	app.Syncer = mail.NewSyncer(cfg, db, app.Mailer)
	if err := app.Syncer.Start(ctx); err != nil {
		return nil, err
	}
	return app, nil
}

// Close 负责按逆序释放运行时资源。
func (a *App) Close() {
	if a.Syncer != nil {
		a.Syncer.Stop()
	}
	for i := len(a.closers) - 1; i >= 0; i-- {
		if err := a.closers[i](); err != nil {
			slog.Error("关闭资源失败", "err", err)
		}
	}
}

// migrate 使用自动迁移快速搭起当前版本所需表结构。
func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserPreference{},
		&model.MailAccount{},
		&model.Mailbox{},
		&model.Message{},
		&model.MessageBody{},
		&model.Attachment{},
		&model.SyncState{},
		&model.SyncLog{},
	); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}

// initAdmin 仅在用户表为空时创建默认管理员，并把随机密码输出到日志方便首次登录。
func initAdmin(db *gorm.DB, cfg *appcfg.Config) error {
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("检查管理员数量失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	password, err := generateRandomPassword(18)
	if err != nil {
		return fmt.Errorf("生成管理员随机密码失败: %w", err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("生成管理员密码哈希失败: %w", err)
	}
	admin := &model.User{Username: cfg.Auth.InitUsername, PasswordHash: hash, SessionVersion: 1}
	if err := db.Create(admin).Error; err != nil {
		return fmt.Errorf("初始化管理员失败: %w", err)
	}
	reportInitAdminPassword(admin.Username, password)
	return nil
}

// generateRandomPassword 使用系统安全随机源生成可直接复制的初始密码，避免默认弱口令进入部署环境。
func generateRandomPassword(length int) (string, error) {
	if length < 8 {
		length = 8
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	password := base64.RawURLEncoding.EncodeToString(buf)
	if len(password) > length {
		password = password[:length]
	}
	return password, nil
}

// reportInitAdminPassword 同时写结构化日志和标准错误，避免高日志级别时丢失首次登录凭据。
func reportInitAdminPassword(username string, password string) {
	slog.Info("默认管理员初始化完成，请妥善保存首次登录密码", "username", username, "password", password)
	_, _ = fmt.Fprintf(os.Stderr, "默认管理员初始化完成，请妥善保存首次登录密码 username=%s password=%s\n", username, password)
}

// ensureDataDir 提前创建 sqlite 所需目录，避免首次启动因路径不存在失败。
func ensureDataDir(cfg appcfg.DBConfig) error {
	if cfg.Driver != "sqlite" {
		return nil
	}
	path := filepath.Dir(cfg.DSN)
	if path == "." || path == "" {
		return nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("创建 sqlite 数据目录失败: %w", err)
	}
	return nil
}

// Now 为后续测试替换时间源预留统一入口。
func Now() time.Time {
	return time.Now()
}
