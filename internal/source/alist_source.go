package source

import (
	"context"
	"errors"
	"net/http"
)

// AlistSource 把网盘当作数据源：Feed 列目录，直链靠 fs/get 实时换取。
type AlistSource struct {
	provider *AlistProvider
}

// NewAlistSource 包装 AlistProvider，使其满足 VideoProvider。
func NewAlistSource(provider *AlistProvider) *AlistSource {
	return &AlistSource{provider: provider}
}

// Name 实现 VideoProvider。
func (s *AlistSource) Name() string { return "alist" }

// Feed 列出网盘目录下的视频（懒登录，未配置网盘时返回错误）。
func (s *AlistSource) Feed(ctx context.Context, dir, password string) ([]FeedItem, error) {
	client := s.provider.Client(ctx)
	if client == nil {
		return nil, errors.New("未配置 alist 网盘源")
	}
	if dir == "" {
		return nil, errors.New("缺少网盘目录")
	}
	return client.Feed(ctx, dir, password)
}

// Resolve 换取网盘直链。百度 dlink 只认客户端 UA（浏览器 UA 会 403），
// 这个 UA 只在服务端代理时使用。
func (s *AlistSource) Resolve(ctx context.Context, itemPath, password string) (*PlaySource, error) {
	client := s.provider.Client(ctx)
	if client == nil {
		return nil, errors.New("未配置 alist 网盘源")
	}
	info, err := client.VideoURL(ctx, itemPath, password)
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	header.Set("User-Agent", "okhttp/3.15")
	return &PlaySource{URL: info.RawURL, Header: header}, nil
}

// Invalidate 清掉某个文件的直链缓存，供上游返回 403 时刷新重试。
func (s *AlistSource) Invalidate(ctx context.Context, itemPath string) {
	if client := s.provider.Client(ctx); client != nil {
		client.InvalidateVideoURL(itemPath)
	}
}
