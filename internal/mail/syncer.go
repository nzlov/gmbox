package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
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
	logger := cron.PrintfLogger(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo))
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

// RunOnce 主动执行一轮全邮箱同步，并把本轮所有邮箱结果汇总成一条日志。
func (s *Syncer) RunOnce(ctx context.Context) error {
	var accounts []model.MailAccount
	if err := s.db.Where("enabled = ?", true).Find(&accounts).Error; err != nil {
		return err
	}
	return s.runAccounts(ctx, accounts, "cron")
}

// RunAccountsNow 手动同步多个邮箱，并把结果写成一条聚合日志。
func (s *Syncer) RunAccountsNow(ctx context.Context, accounts []model.MailAccount) error {
	return s.runAccounts(ctx, accounts, "manual")
}

// runAccounts 统一处理批量同步执行和聚合日志落库，避免手动与定时链路口径不一致。
func (s *Syncer) runAccounts(ctx context.Context, accounts []model.MailAccount, trigger string) error {
	if len(accounts) == 0 {
		return nil
	}
	start := time.Now()
	sem := make(chan struct{}, s.cfg.Mail.MaxConcurrency)
	results := make([]model.SyncLogDetail, 0, len(accounts))
	var failed []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, account := range accounts {
		account := account
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, err := s.syncAccount(ctx, account)
			mu.Lock()
			results = append(results, result)
			if err != nil {
				failed = append(failed, account.Email)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	finished := time.Now()
	s.writeSyncLog(trigger, start, finished, results)
	if len(failed) > 0 {
		return fmt.Errorf("以下邮箱同步失败: %s", strings.Join(failed, ", "))
	}
	return nil
}

// RunAccountNow 手动同步单个邮箱，并按统一聚合格式写一条日志。
func (s *Syncer) RunAccountNow(ctx context.Context, account model.MailAccount) error {
	return s.runAccounts(ctx, []model.MailAccount{account}, "manual")
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

// summarizeSyncLog 统一生成聚合文案，避免前后端各自重复计算成功率摘要。
func summarizeSyncLog(results []model.SyncLogDetail) (int, int, float64, string) {
	accountCount := len(results)
	if accountCount == 0 {
		return 0, 0, 0, "本轮没有可同步的邮箱"
	}
	successCount := 0
	for _, item := range results {
		if item.Success {
			successCount++
		}
	}
	successRate := float64(successCount) / float64(accountCount) * 100
	return accountCount, successCount, successRate, fmt.Sprintf("本轮同步 %d 个邮箱，成功 %d 个，成功率 %.0f%%", accountCount, successCount, successRate)
}

// writeSyncLog 将整轮同步结果聚合写入历史表，避免多邮箱同步时刷出大量单邮箱日志。
func (s *Syncer) writeSyncLog(trigger string, startedAt time.Time, finishedAt time.Time, results []model.SyncLogDetail) {
	sort.Slice(results, func(i int, j int) bool {
		return results[i].AccountEmail < results[j].AccountEmail
	})
	accountCount, successCount, successRate, summary := summarizeSyncLog(results)
	details, err := json.Marshal(results)
	if err != nil {
		summary = summary + "，但明细序列化失败"
		details = []byte("[]")
	}
	logEntry := model.SyncLog{
		Trigger:        trigger,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		DurationMs:     finishedAt.Sub(startedAt).Milliseconds(),
		AccountCount:   accountCount,
		SuccessCount:   successCount,
		SuccessRate:    successRate,
		SummaryMessage: summary,
		Details:        string(details),
	}
	_ = s.db.Create(&logEntry).Error
}

// syncAccount 执行单邮箱同步，并返回写入聚合日志所需的最小明细结果。
func (s *Syncer) syncAccount(ctx context.Context, account model.MailAccount) (model.SyncLogDetail, error) {
	start := time.Now()
	logDetail := model.SyncLogDetail{
		AccountID:    account.Model.ID,
		AccountName:  account.Name,
		AccountEmail: account.Email,
	}
	state := model.SyncState{AccountID: account.Model.ID}
	if err := s.db.Where("account_id = ?", account.Model.ID).FirstOrCreate(&state, model.SyncState{AccountID: account.Model.ID}).Error; err != nil {
		logDetail.DurationMs = time.Since(start).Milliseconds()
		logDetail.ErrorMessage = err.Error()
		return logDetail, err
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
	logDetail.DurationMs = finished.Sub(start).Milliseconds()
	if result != nil {
		logDetail.NewMessages = result.NewMessages
	}
	if err != nil {
		state.LastStatus = "error"
		state.LastError = err.Error()
		if retriedOAuth {
			state.LastMessage = "同步执行失败，且已自动刷新 OAuth token 重试过一次"
		} else {
			state.LastMessage = "同步执行失败"
		}
		_ = s.db.Save(&state).Error
		logDetail.ErrorMessage = err.Error()
		return logDetail, err
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
		logDetail.ErrorMessage = saveErr.Error()
		return logDetail, saveErr
	}
	logDetail.Success = true
	return logDetail, nil
}
