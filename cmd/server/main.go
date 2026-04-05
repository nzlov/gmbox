package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gmbox/frontend"
	appcfg "gmbox/internal/config"
	"gmbox/internal/httpapi"
	"gmbox/internal/logging"
	"gmbox/internal/runtime"
)

// main 负责串联配置、运行时初始化和 HTTP 服务生命周期。
func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := appcfg.Load("config.yaml")
	if err != nil {
		slog.Error("加载配置失败", "err", err)
		os.Exit(1)
	}
	logging.Configure(cfg)

	app, err := runtime.New(ctx, cfg)
	if err != nil {
		slog.Error("初始化应用失败", "err", err)
		os.Exit(1)
	}
	defer app.Close()

	router := httpapi.NewRouter(app, frontend.Sub())
	server := &http.Server{
		Addr:              cfg.App.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("gmbox 启动成功", "addr", cfg.App.Addr, "env", cfg.App.Env, "log_level", cfg.SlogLevel())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP 服务异常退出", "err", err)
		os.Exit(1)
	}
}
