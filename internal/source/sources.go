package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"qimeng-feed/internal/config"
)

// Sources 按 config.json 的最新内容提供数据源。
//
// 保存配置后不需要重启：每次取源都会重新读一遍磁盘上的配置，
// 配置变化时重建全部在线源实例（连同缓存一起清掉）。
type Sources struct {
	load        func() config.Settings
	alist       *AlistSource
	drpyTimeout time.Duration
	// static 是不随配置变化的数据源（目前只有兔小贝直连），用名字直接索引。
	static map[string]VideoProvider

	mu       sync.Mutex
	key      string
	online   map[string]*OnlineSource
	ordered  []string
	router   *onlineRouter
}

// NewSources 创建数据源管理器。alist 为网盘 provider，static 可为 nil。
func NewSources(load func() config.Settings, alist *AlistProvider, drpyTimeout time.Duration, static map[string]VideoProvider) *Sources {
	if drpyTimeout <= 0 {
		drpyTimeout = 10 * time.Second
	}
	s := &Sources{
		load:        load,
		alist:       NewAlistSource(alist),
		drpyTimeout: drpyTimeout,
		static:      static,
	}
	s.router = &onlineRouter{sources: s}
	return s
}

// Primary 返回首页默认数据源：有启用的在线源时是混合流，否则回退到网盘。
func (s *Sources) Primary() VideoProvider {
	if len(s.onlineSources()) > 0 {
		return s.router
	}
	return s.alist
}

// Provider 按名字返回数据源；未配置（如在线源没填完整）时返回 nil。
func (s *Sources) Provider(name string) VideoProvider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "alist":
		return s.alist
	case "online", "drpy":
		if len(s.onlineSources()) == 0 {
			return nil
		}
		return s.router
	default:
		return s.static[name]
	}
}

// Online 返回指定 ID 的在线源；未配置或未启用时返回 nil。
func (s *Sources) Online(sourceID string) *OnlineSource {
	return s.onlineByID(sourceID)
}

// OnlineList 返回全部已启用的在线源（按配置顺序）。
func (s *Sources) OnlineList() []*OnlineSource {
	all := s.onlineSources()
	out := make([]*OnlineSource, 0, len(all))
	for _, id := range s.orderedSnapshot() {
		if o := all[id]; o != nil {
			out = append(out, o)
		}
	}
	return out
}

func (s *Sources) orderedSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ordered...)
}

func (s *Sources) onlineByID(id string) *OnlineSource {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	return s.onlineSources()[id]
}

func (s *Sources) firstOnlineID() string {
	order := s.orderedSnapshot()
	if len(order) == 0 {
		return ""
	}
	return order[0]
}

// onlineSources 按当前配置构建在线源集合；配置未变时直接复用。
func (s *Sources) onlineSources() map[string]*OnlineSource {
	cfg := s.load()
	key := sourcesKey(cfg.Sources)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.online != nil && s.key == key {
		return s.online
	}

	built := make(map[string]*OnlineSource, len(cfg.Sources))
	order := make([]string, 0, len(cfg.Sources))
	for _, sc := range cfg.Sources {
		sc.ID = strings.TrimSpace(sc.ID)
		if sc.ID == "" || !sc.Enabled || !sc.Configured() {
			continue
		}
		if _, exists := built[sc.ID]; exists {
			continue
		}
		o, err := newOnlineSource(sc, s.drpyTimeout)
		if err != nil {
			slog.Warn("在线数据源初始化失败", "source_id", sc.ID, "name", sc.Name, "err", err)
			continue
		}
		built[sc.ID] = o
		order = append(order, sc.ID)
	}
	s.online = built
	s.ordered = order
	s.key = key
	return built
}

// sourcesKey 是配置指纹：源列表任何字段变化都会触发重建。
func sourcesKey(list []config.SourceSettings) string {
	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Sprintf("%v", list)
	}
	return string(data)
}

// 首页混排的取数预算：勾选栏目越多，每个栏目只取越少的条目。
// 目标总量约 120 屏，避免勾了 20+ 个栏目后首页直接变成上千个滑屏。
const (
	feedTotalTarget     = 120
	feedPerCategoryMin  = 3
	feedPerCategoryMax  = 30
	feedCategoryWorkers = 6
	feedSourceTimeout   = 12 * time.Second
)

// feedPerCategoryCount 按勾选栏目数算每个栏目取几条：少勾就多取，多勾就少取。
func feedPerCategoryCount(categories int) int {
	if categories <= 0 {
		return feedPerCategoryMax
	}
	n := feedTotalTarget / categories
	if n < feedPerCategoryMin {
		n = feedPerCategoryMin
	}
	if n > feedPerCategoryMax {
		n = feedPerCategoryMax
	}
	return n
}

// capFeedItems 截断单个栏目的结果，超出部分留给合集面板按需拉取。
func capFeedItems(items []FeedItem, limit int) []FeedItem {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

// MixedFeed 把全部已启用数据源的勾选栏目合并后随机打乱。
// 网盘源按「基础视频路径」参与混排；没有任何在线源时保留原来按目录浏览的行为。
func (s *Sources) MixedFeed(ctx context.Context, dir string) ([]FeedItem, error) {
	cfg := s.load()
	online := s.onlineSources()

	if len(online) == 0 {
		if !cfg.Alist.Configured() || cfg.Alist.Disabled {
			return nil, errors.New("还没有启用任何数据源")
		}
		if strings.TrimSpace(dir) == "" {
			dir = cfg.Alist.BasePath
		}
		if strings.TrimSpace(dir) == "" {
			return nil, errors.New("缺少网盘目录")
		}
		return s.alist.Feed(ctx, dir, "")
	}

	// 在线源一个栏目都没勾选、网盘也没参与时，直接告诉用户去勾栏目，
	// 不要再逐个请求上游后报「都没有内容」。
	selected := 0
	for _, sc := range cfg.Sources {
		if sc.Enabled && sc.Configured() && len(sc.Categories) > 0 {
			selected++
		}
	}
	if selected == 0 && !cfg.Alist.Included() {
		return nil, errors.New("已启用的数据源都没有勾选栏目")
	}

	type result struct {
		name  string
		items []FeedItem
		err   error
	}
	results := make(chan result, len(online)+1)
	var wg sync.WaitGroup

	order := s.orderedSnapshot()
	for _, id := range order {
		o := online[id]
		if o == nil {
			continue
		}
		wg.Add(1)
		go func(o *OnlineSource) {
			defer wg.Done()
			items, err := o.Feed(ctx)
			results <- result{name: o.Name(), items: items, err: err}
		}(o)
	}

	// 网盘源也参与混排：勾选栏目的语义换成「基础视频路径」。
	if cfg.Alist.Configured() && cfg.Alist.Included() {
		wg.Add(1)
		go func(basePath string) {
			defer wg.Done()
			items, err := s.alist.Feed(ctx, basePath, "")
			for i := range items {
				items[i].Category = "本地网盘"
			}
			results <- result{name: "本地网盘", items: items, err: err}
		}(cfg.Alist.BasePath)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]FeedItem, 0, 256)
	var firstErr error
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			slog.Warn("数据源拉取失败，已跳过", "source", r.name, "err", r.err)
			continue
		}
		all = append(all, r.items...)
	}
	if len(all) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("所有数据源都拉取失败: %w", firstErr)
		}
		return nil, errors.New("已启用的数据源都没有内容")
	}

	rand.Shuffle(len(all), func(i, j int) {
		all[i], all[j] = all[j], all[i]
	})
	return all, nil
}
