package mail

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	appcfg "gmbox/internal/config"
	"gmbox/internal/model"
)

// SyncResult 保存单次同步的结构化统计，避免日志只能依赖文案解析。
type SyncResult struct {
	NewMessages  int
	MailboxCount int
}

// Syncer 负责按照 cron 表达式调度多邮箱并发同步。
type Syncer struct {
	cfg    *appcfg.Config
	db     *gorm.DB
	mailer *Service
	cron   *cron.Cron
}

// NewSyncer 创建 cron 同步器。
func NewSyncer(cfg *appcfg.Config, db *gorm.DB, mailer *Service) *Syncer {
	logger := cron.PrintfLogger(log.Default())
	return &Syncer{
		cfg:    cfg,
		db:     db,
		mailer: mailer,
		cron: cron.New(cron.WithChain(
			cron.Recover(logger),
			cron.SkipIfStillRunning(logger),
		)),
	}
}

// Start 注册 cron 任务并启动调度器。
func (s *Syncer) Start(ctx context.Context) error {
	if _, err := s.cron.AddFunc(s.cfg.Mail.SyncCron, func() {
		_ = s.RunOnce(ctx)
	}); err != nil {
		return fmt.Errorf("注册同步任务失败: %w", err)
	}
	s.cron.Start()
	return nil
}

// Stop 停止调度器，避免服务退出后继续接收任务。
func (s *Syncer) Stop() {
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
	}
}

// RunOnce 主动执行一轮同步，供手动触发和定时任务共用。
func (s *Syncer) RunOnce(ctx context.Context) error {
	var accounts []model.MailAccount
	if err := s.db.Where("enabled = ?", true).Find(&accounts).Error; err != nil {
		return err
	}
	if len(accounts) == 0 {
		return nil
	}
	sem := make(chan struct{}, s.cfg.Mail.MaxConcurrency)
	g, runCtx := errgroup.WithContext(ctx)
	for _, account := range accounts {
		account := account
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			return s.syncAccount(runCtx, account, "cron")
		})
	}
	return g.Wait()
}

// RunAccountNow 手动同步单个邮箱，便于前端在账户详情页即时触发。
func (s *Syncer) RunAccountNow(ctx context.Context, account model.MailAccount) error {
	return s.syncAccount(ctx, account, "manual")
}

// runSyncAttempt 执行单次协议同步，便于 OAuth 失败后复用同一入口重试。
func (s *Syncer) runSyncAttempt(ctx context.Context, account model.MailAccount, state *model.SyncState) (*SyncResult, error) {
	if account.IncomingProtocol == "imap" {
		return s.mailer.SyncIMAP(ctx, account, state, s.cfg.Mail.FetchBody)
	}
	return s.mailer.SyncPOP3(ctx, account, state, s.cfg.Mail.FetchBody)
}

// shouldRetryOAuthSync 仅在明显认证或 token 相关失败时触发一次强制刷新，避免把业务解析错误误判成 OAuth 失效。
func shouldRetryOAuthSync(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	keywords := []string{
		"oauth",
		"access token",
		"refresh token",
		"connection closed",
		"authenticat",
		"invalid_grant",
		"expired",
		"unauthorized",
	}
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// writeSyncLog 将单次同步结果写入历史表，便于排查和展示。
func (s *Syncer) writeSyncLog(account model.MailAccount, trigger string, startedAt time.Time, finishedAt time.Time, result *SyncResult, retriedOAuth bool, err error, summary string) {
	logEntry := model.SyncLog{
		AccountID:      account.Model.ID,
		AccountName:    account.Name,
		AccountEmail:   account.Email,
		Trigger:        trigger,
		Protocol:       account.IncomingProtocol,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		DurationMs:     finishedAt.Sub(startedAt).Milliseconds(),
		Success:        err == nil,
		RetriedOAuth:   retriedOAuth,
		SummaryMessage: summary,
	}
	if result != nil {
		logEntry.NewMessages = result.NewMessages
		logEntry.MailboxCount = result.MailboxCount
	}
	if err != nil {
		logEntry.ErrorMessage = err.Error()
	}
	_ = s.db.Create(&logEntry).Error
}

// syncAccount 当前先落同步骨架，确保状态机、并发控制和接口可用。

func (s *Syncer) syncAccount(ctx context.Context, account model.MailAccount, trigger string) error {
	start := time.Now()
	state := model.SyncState{AccountID: account.Model.ID}
	if err := s.db.Where("account_id = ?", account.Model.ID).FirstOrCreate(&state, model.SyncState{AccountID: account.Model.ID}).Error; err != nil {
		return err
	}

	now := time.Now()
	state.Running = true
	state.LastStatus = "running"
	state.LastMessage = "开始执行同步任务"
	state.LastSyncAt = &now
	_ = s.db.Save(&state).Error

	result, err := s.runSyncAttempt(ctx, account, &state)
	retriedOAuth := false
	if err != nil && normalizeAuthType(account.AuthType) == "oauth" && shouldRetryOAuthSync(err) {
		retriedOAuth = true
		if _, refreshErr := s.mailer.ForceRefreshOAuthAccessToken(ctx, &account); refreshErr == nil {
			state.LastMessage = "OAuth 认证失败，已自动刷新 token 后重试"
			result, err = s.runSyncAttempt(ctx, account, &state)
		} else {
			err = fmt.Errorf("%w；自动刷新 OAuth token 失败: %v", err, refreshErr)
		}
	}
	finished := time.Now()
	state.Running = false
	state.LastSyncAt = &finished
	state.LastDuration = time.Since(start).Milliseconds()
	state.LastMessageAt = &finished
	if err != nil {
		state.LastStatus = "error"
		state.LastError = err.Error()
		if retriedOAuth {
			state.LastMessage = "同步执行失败，且已自动刷新 OAuth token 重试过一次"
		} else {
			state.LastMessage = "同步执行失败"
		}
		_ = s.db.Save(&state).Error
		s.writeSyncLog(account, trigger, start, finished, result, retriedOAuth, err, state.LastMessage)
		return err
	}
	state.LastStatus = "ok"
	state.LastError = ""
	if strings.TrimSpace(state.LastMessage) == "" {
		state.LastMessage = "同步完成"
	}
	if retriedOAuth {
		state.LastMessage = state.LastMessage + "（期间已自动刷新 OAuth token 并重试成功）"
	}
	if saveErr := s.db.Save(&state).Error; saveErr != nil {
		return saveErr
	}
	s.writeSyncLog(account, trigger, start, finished, result, retriedOAuth, nil, state.LastMessage)
	return nil
}
