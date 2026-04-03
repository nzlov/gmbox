package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"gmbox/frontend"
	appcfg "gmbox/internal/config"
	"gmbox/internal/httpapi"
	"gmbox/internal/runtime"
)

// main 负责串联配置、运行时初始化和 HTTP 服务生命周期。
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := appcfg.Load("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	app, err := runtime.New(ctx, cfg)
	if err != nil {
		log.Fatalf("初始化应用失败: %v", err)
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

	log.Printf("gmbox 启动成功，监听地址: %s", cfg.App.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP 服务异常退出: %v", err)
	}
}
