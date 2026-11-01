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
	"strings"
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

	// 网盘挂载参数以 config.json 为准；文件不存在时用 config.yaml 里的 alist 段做初始值。
	configJSON := os.Getenv("QIMENG_CONFIG_JSON")
	if configJSON == "" {
		configJSON = "config.json"
	}
	store, err := config.NewStore(configJSON, config.SeedFromAlist(cfg.Alist))
	if err != nil {
		return err
	}

	alistProvider := source.NewAlistProvider(store.Get, cfg.Alist.Timeout)
	if store.Get().Host == "" {
		slog.Warn("网盘未配置，可在前端「我的 → 服务器配置」里填写", "config", configJSON)
	}

	// 所有数据源都实现 VideoProvider，这里按配置组装：默认源 + 名字索引。
	providers := map[string]source.VideoProvider{
		"alist":     source.NewAlistSource(alistProvider),
		"tuxiaobei": source.NewTuxiaobeiSource("", cfg.Alist.Timeout),
	}
	if cfg.Source.Drpy.Enabled() {
		d := cfg.Source.Drpy
		providers["drpy"] = source.NewDrpySource(d.BaseURL, d.Name, d.Pwd, d.Keyword, d.Timeout)
		slog.Info("已启用 drpy 数据源", "base_url", d.BaseURL, "source", d.Name, "keyword", d.Keyword)
	}

	primaryName := strings.ToLower(strings.TrimSpace(cfg.Source.Primary))
	if primaryName == "" {
		primaryName = "alist"
	}
	primary := providers[primaryName]
	if primary == nil {
		slog.Warn("配置的数据源不可用，已回退", "want", primaryName)
		if providers["drpy"] != nil {
			primary = providers["drpy"]
			primaryName = "drpy"
		} else {
			primary = providers["alist"]
			primaryName = "alist"
		}
	}
	slog.Info("Feed 默认数据源", "source", primaryName)

	webFS, err := fs.Sub(web.Files, ".")
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           api.NewRouter(api.NewHandler(alistProvider, store, primary, providers), http.FS(webFS)),
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
