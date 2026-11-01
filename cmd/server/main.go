// qimeng-feed 服务入口：加载配置、装配依赖、启动 HTTP 服务并优雅退出。
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"qimeng-feed/internal/api"
	"qimeng-feed/internal/config"
	"qimeng-feed/internal/source"
	"qimeng-feed/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("服务退出", "err", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	configPath := os.Getenv("QIMENG_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// 数据源参数以 config.json 为准；文件不存在时用 config.yaml 里的 alist / source 段做初始值。
	configJSON := os.Getenv("QIMENG_CONFIG_JSON")
	if configJSON == "" {
		configJSON = "config.json"
	}
	store, err := config.NewStore(configJSON, config.Seed(cfg.Alist, cfg.Source))
	if err != nil {
		return err
	}

	alistProvider := source.NewAlistProvider(store.GetAlist, cfg.Alist.Timeout)
	if !store.GetAlist().Configured() {
		slog.Warn("网盘未配置，可在前端「我的 → 数据源配置」里填写", "config", configJSON)
	}

	// 所有数据源都实现 VideoProvider：在线源（drpy / 上传的 JS 规则）按 config.json 的
	// sources 实时解析，勾选栏目后首页一起随机混排；参数改了会自动重建实例。
	sources := source.NewSources(store.Get, alistProvider, cfg.Source.Drpy.Timeout, map[string]source.VideoProvider{
		"tuxiaobei": source.NewTuxiaobeiSource("", cfg.Alist.Timeout),
	})
	enabled := 0
	for _, s := range store.Get().Sources {
		if s.Enabled && s.Configured() {
			enabled++
			slog.Info("已启用数据源", "id", s.ID, "kind", s.Kind, "name", s.Name, "categories", len(s.Categories))
		}
	}
	slog.Info("数据源统计", "total", len(store.Get().Sources), "enabled", enabled, "alist", store.GetAlist().BaseURL())

	webFS, err := fs.Sub(web.Files, ".")
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           api.NewRouter(api.NewHandler(alistProvider, store, sources), http.FS(webFS)),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP 服务启动", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("收到退出信号，开始关闭服务")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Info("服务已退出", "at", time.Now().Format(time.RFC3339))
	return nil
}
