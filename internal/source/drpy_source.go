package source

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// drpyEpisodeTTL 是「vod_id → 第一集直链」的缓存时长。
// 一次播放会对同一地址发多次 Range 请求（mpegts 尤其明显），
// 每次都调一次详情接口既慢又容易被上游限流，所以缓存一小段时间。
const drpyEpisodeTTL = 10 * time.Minute

// DrpySource 把本地 drpy-node 当作数据源：
// Feed = SearchVideos(关键词)，播放 = detail 接口里的 vod_play_url 第一集。
type DrpySource struct {
	client  *Drpy
	keyword string

	mu    sync.Mutex
	cache map[string]cachedPlay
}

type cachedPlay struct {
	url    string
	expire time.Time
}

// NewDrpySource 创建 drpy 数据源；keyword 为空时用「儿歌」。
func NewDrpySource(baseURL, sourceName, pwd, keyword string, timeout time.Duration) *DrpySource {
	if strings.TrimSpace(keyword) == "" {
		keyword = "儿歌"
	}
	return &DrpySource{
		client:  NewDrpy(baseURL, sourceName, pwd, timeout),
		keyword: strings.TrimSpace(keyword),
		cache:   make(map[string]cachedPlay),
	}
}

// Name 实现 VideoProvider。
func (s *DrpySource) Name() string { return "drpy" }

// Feed 用关键词搜索一批视频作为初始数据。
// dir 传的不是路径（不含 / 开头）时当作临时关键词，方便手工换个词刷一批；
// 以 / 开头的是网盘目录语义，drpy 忽略。
func (s *DrpySource) Feed(ctx context.Context, dir, _ string) ([]FeedItem, error) {
	keyword := s.keyword
	if dir != "" && !strings.HasPrefix(dir, "/") {
		keyword = dir
	}

	videos, err := s.client.SearchVideos(ctx, keyword)
	if err != nil {
		return nil, err
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("搜索「%s」没有结果", keyword)
	}

	items := make([]FeedItem, 0, len(videos))
	for _, v := range videos {
		items = append(items, FeedItem{
			Name:     v.VodName,
			Path:     DrpyItemPath(v.VodID),
			CoverURL: v.VodPic,
		})
	}
	return items, nil
}

// Resolve 调详情接口取第一集直链，并按目标站点域名补上 Referer。
// Referer 用直链自身的 origin：绝大多数防盗链只校验「是不是自家站点」。
func (s *DrpySource) Resolve(ctx context.Context, itemPath, _ string) (*PlaySource, error) {
	vodID := strings.TrimPrefix(itemPath, DrpyPathPrefix)
	if vodID == "" {
		return nil, fmt.Errorf("缺少 vod_id（%s）", itemPath)
	}

	s.mu.Lock()
	cached, ok := s.cache[vodID]
	s.mu.Unlock()
	if ok && time.Now().Before(cached.expire) {
		return s.playSource(cached.url), nil
	}

	episodes, err := s.client.GetPlayUrl(ctx, vodID)
	if err != nil {
		return nil, err
	}
	target := episodes[0].URL

	s.mu.Lock()
	s.cache[vodID] = cachedPlay{url: target, expire: time.Now().Add(drpyEpisodeTTL)}
	s.mu.Unlock()
	return s.playSource(target), nil
}

// Invalidate 丢掉某个条目的直链缓存，供上游返回 403 时刷新重试。
func (s *DrpySource) Invalidate(_ context.Context, itemPath string) {
	vodID := strings.TrimPrefix(itemPath, DrpyPathPrefix)
	s.mu.Lock()
	delete(s.cache, vodID)
	s.mu.Unlock()
}

// playSource 组装上游请求头：浏览器 UA + 目标站 origin 作为 Referer。
func (s *DrpySource) playSource(target string) *PlaySource {
	header := http.Header{}
	header.Set("User-Agent", drpyUA)
	header.Set("Accept", "*/*")
	if u, err := url.Parse(target); err == nil && u.Host != "" {
		header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	}
	return &PlaySource{URL: target, Header: header}
}
