package source

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TuxiaobeiPathPrefix 是直连兔小贝条目在 FeedItem.Path 里的前缀。
const TuxiaobeiPathPrefix = "tuxiaobei:"

// TuxiaobeiSource 把原有「直连兔小贝站点」的抓取逻辑包装成 VideoProvider，
// 保留原来的 Search / Detail 实现，不依赖 drpy-node。
type TuxiaobeiSource struct {
	client  *Tuxiaobei
	keyword string
}

// NewTuxiaobeiSource 创建直连数据源；keyword 为空时用「儿歌」。
func NewTuxiaobeiSource(keyword string, timeout time.Duration) *TuxiaobeiSource {
	if strings.TrimSpace(keyword) == "" {
		keyword = "儿歌"
	}
	return &TuxiaobeiSource{
		client:  NewTuxiaobei(timeout),
		keyword: strings.TrimSpace(keyword),
	}
}

// Name 实现 VideoProvider。
func (s *TuxiaobeiSource) Name() string { return "tuxiaobei" }

// Feed 用关键词搜索站点，把结果映射成通用 Feed 条目。
// dir 不以 / 开头时视为临时关键词。
func (s *TuxiaobeiSource) Feed(ctx context.Context, dir, _ string) ([]FeedItem, error) {
	keyword := s.keyword
	if dir != "" && !strings.HasPrefix(dir, "/") {
		keyword = dir
	}

	items, err := s.client.Search(ctx, keyword)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("搜索「%s」没有结果", keyword)
	}

	out := make([]FeedItem, 0, len(items))
	for _, item := range items {
		// 搜索结果里偶尔有解析不出 id 的条目，跳过避免生成无效路径
		if item.VideoID <= 0 {
			continue
		}
		itemPath := TuxiaobeiPathPrefix + strconv.Itoa(item.VideoID)
		out = append(out, FeedItem{
			Name:     item.Name,
			Path:     itemPath,
			CoverURL: item.Image,
		})
	}
	return out, nil
}

// Resolve 走原有详情解析，取页面里的 video-src 作为直链。
func (s *TuxiaobeiSource) Resolve(ctx context.Context, itemPath, _ string) (*PlaySource, error) {
	raw := strings.TrimPrefix(itemPath, TuxiaobeiPathPrefix)
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("非法条目路径 %q: %w", itemPath, err)
	}

	detail, err := s.client.Detail(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail.PlayURL == "" {
		return nil, fmt.Errorf("详情 %d 没有解析到播放直链", id)
	}

	header := http.Header{}
	header.Set("User-Agent", tuxiaobeiUA)
	header.Set("Accept", "*/*")
	header.Set("Referer", TuxiaobeiHost+"/")
	return &PlaySource{URL: detail.PlayURL, Header: header}, nil
}
