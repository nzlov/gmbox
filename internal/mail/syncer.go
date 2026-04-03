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
			return s.syncAccount(runCtx, account)
		})
	}
	return g.Wait()
}

// RunAccountNow 手动同步单个邮箱，便于前端在账户详情页即时触发。
func (s *Syncer) RunAccountNow(ctx context.Context, account model.MailAccount) error {
	return s.syncAccount(ctx, account)
}

// syncAccount 当前先落同步骨架，确保状态机、并发控制和接口可用。
func (s *Syncer) syncAccount(ctx context.Context, account model.MailAccount) error {
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

	var err error
	if account.IncomingProtocol == "imap" {
		err = s.mailer.SyncIMAP(ctx, account, &state, s.cfg.Mail.FetchBody)
	} else {
		err = s.mailer.SyncPOP3(ctx, account, &state, s.cfg.Mail.FetchBody)
	}
	finished := time.Now()
	state.Running = false
	state.LastSyncAt = &finished
	state.LastDuration = time.Since(start).Milliseconds()
	if err != nil {
		state.LastStatus = "error"
		state.LastError = err.Error()
		state.LastMessage = "同步执行失败"
		_ = s.db.Save(&state).Error
		return err
	}
	state.LastStatus = "ok"
	state.LastError = ""
	if strings.TrimSpace(state.LastMessage) == "" {
		state.LastMessage = "同步完成"
	}
	return s.db.Save(&state).Error
}
