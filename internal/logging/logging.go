package logging

import (
	"log"
	"log/slog"
	"os"

	appcfg "gmbox/internal/config"
)

// Configure 按项目配置初始化全局 slog，并将标准库 log 桥接到同一输出。
func Configure(cfg *appcfg.Config) *slog.Logger {
	level := slog.LevelInfo
	addSource := false
	if cfg != nil {
		level = cfg.SlogLevel()
		addSource = cfg.DebugMode()
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: addSource,
	}))
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), slog.LevelInfo).Writer())
	return logger
}
