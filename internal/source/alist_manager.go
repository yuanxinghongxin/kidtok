package source

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"qimeng-feed/internal/config"
)

// AlistProvider 按 config.json 的最新内容提供 alist 客户端：
// 每次取用都重新读配置，配置没变就复用已登录的客户端，变了就重建并重新登录。
type AlistProvider struct {
	mu      sync.Mutex
	timeout time.Duration
	load    func() config.AlistSettings
	applied config.AlistSettings
	client  *Alist
}

// NewAlistProvider 创建 provider；load 每次调用都应返回磁盘上的最新配置。
func NewAlistProvider(load func() config.AlistSettings, timeout time.Duration) *AlistProvider {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &AlistProvider{timeout: timeout, load: load}
}

// Client 返回与当前配置匹配的客户端，未填 Host 时返回 nil。
func (p *AlistProvider) Client(ctx context.Context) *Alist {
	settings := p.load()
	baseURL := settings.BaseURL()
	if baseURL == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil && p.applied == settings {
		return p.client
	}
	return p.applyLocked(ctx, settings)
}

// Apply 立即按给定配置登录，成功才替换缓存；供保存配置时做一次连通性校验。
func (p *AlistProvider) Apply(ctx context.Context, settings config.AlistSettings) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	client := NewAlist(settings.BaseURL(), "", p.timeout)
	if strings.TrimSpace(settings.Username) != "" {
		token, err := p.login(ctx, client, settings)
		if err != nil {
			return err
		}
		client.SetToken(token)
	}
	p.client = client
	p.applied = settings
	return nil
}

// Timeout 返回请求超时，写配置时一并回传给前端。
func (p *AlistProvider) Timeout() time.Duration {
	return p.timeout
}

func (p *AlistProvider) applyLocked(ctx context.Context, settings config.AlistSettings) *Alist {
	client := NewAlist(settings.BaseURL(), "", p.timeout)
	if strings.TrimSpace(settings.Username) != "" {
		token, err := p.login(ctx, client, settings)
		if err != nil {
			slog.Warn("alist 登录失败，网盘接口将不可用", "host", settings.Host, "err", err)
		} else {
			client.SetToken(token)
			slog.Info("alist 登录成功", "base_url", settings.BaseURL())
		}
	}
	p.client = client
	p.applied = settings
	return client
}

func (p *AlistProvider) login(ctx context.Context, client *Alist, settings config.AlistSettings) (string, error) {
	loginCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	return client.Login(loginCtx, settings.Username, settings.Password)
}
