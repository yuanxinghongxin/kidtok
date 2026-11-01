package source

import (
	"strings"
	"sync"
	"time"

	"qimeng-feed/internal/config"
)

// Sources 按 config.json 的最新内容提供数据源。
//
// 保存配置后不需要重启：每次取源都会重新读一遍磁盘上的配置，
// 默认源随 primary_source 切换，drpy 参数变化时会重建 DrpySource（连同缓存一起清掉）。
type Sources struct {
	load        func() config.Settings
	alist       *AlistSource
	drpyTimeout time.Duration
	// static 是不随配置变化的数据源（目前只有兔小贝直连），用名字直接索引。
	static map[string]VideoProvider

	mu      sync.Mutex
	drpyKey string
	drpy    *DrpySource
}

// NewSources 创建数据源管理器。alist 为网盘 provider，static 可为 nil。
func NewSources(load func() config.Settings, alist *AlistProvider, drpyTimeout time.Duration, static map[string]VideoProvider) *Sources {
	if drpyTimeout <= 0 {
		drpyTimeout = 10 * time.Second
	}
	return &Sources{
		load:        load,
		alist:       NewAlistSource(alist),
		drpyTimeout: drpyTimeout,
		static:      static,
	}
}

// Primary 返回当前默认数据源：按 primary_source 选，选中的源没配好时回退到网盘。
func (s *Sources) Primary() VideoProvider {
	cfg := s.load()
	if strings.EqualFold(strings.TrimSpace(cfg.PrimarySource), "drpy") {
		if d := s.Drpy(); d != nil {
			return d
		}
	}
	return s.alist
}

// Provider 按名字返回数据源；未配置（如 drpy 没填地址）时返回 nil。
func (s *Sources) Provider(name string) VideoProvider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "alist":
		return s.alist
	case "drpy":
		return s.Drpy()
	default:
		return s.static[name]
	}
}

// Drpy 返回与当前配置匹配的 drpy 源；地址或源名没填时返回 nil。
func (s *Sources) Drpy() *DrpySource {
	d := s.load().Drpy
	if !d.Configured() {
		return nil
	}

	key := d.BaseURL + "|" + d.Name + "|" + d.Pwd + "|" + d.Keyword
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.drpy != nil && s.drpyKey == key {
		return s.drpy
	}
	s.drpy = NewDrpySource(d.BaseURL, d.Name, d.Pwd, d.Keyword, s.drpyTimeout)
	s.drpyKey = key
	return s.drpy
}
