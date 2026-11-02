package source

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
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

// drpyCategoryTTL 是「栏目列表」的缓存时长。
// 设置页勾选栏目、首页每次刷新都会用到它，缓一小段时间既快又能避免上游限流。
const drpyCategoryTTL = 10 * time.Minute

// DrpySource 把本地 drpy-node 当作数据源：
// Feed = 勾选栏目的第一页合并后洗牌，播放 = detail 接口里的 vod_play_url 第一集。
type DrpySource struct {
	client  *Drpy
	keyword string
	// categories 是设置页勾选参与首页混排的栏目 ID；为空时用源返回的第一个栏目。
	categories []string
	// sourceID 是多数据源配置里的源 ID；为空表示旧版单源模式。
	sourceID string

	mu    sync.Mutex
	cache map[string]cachedPlay

	catMu      sync.Mutex
	cats       []DrpyCategory
	catsExpire time.Time
}

type cachedPlay struct {
	url    string
	expire time.Time
}

// NewDrpySource 创建 drpy 数据源；keyword 为空时用「儿歌」，categories 为空表示默认第一个栏目。
func NewDrpySource(baseURL, sourceName, pwd, keyword string, categories []string, timeout time.Duration) *DrpySource {
	if strings.TrimSpace(keyword) == "" {
		keyword = "儿歌"
	}
	return &DrpySource{
		client:     NewDrpy(baseURL, sourceName, pwd, timeout),
		keyword:    strings.TrimSpace(keyword),
		categories: categories,
		cache:      make(map[string]cachedPlay),
	}
}

// Name 实现 VideoProvider。
func (s *DrpySource) Name() string { return "drpy" }

// SetSourceID 设置多数据源配置里的源 ID；设置后 FeedItem.Path 会带上源 ID。
func (s *DrpySource) SetSourceID(id string) { s.sourceID = strings.TrimSpace(id) }

// Categories 返回当前源支持的全部栏目，供设置页勾选（带 10 分钟缓存）。
func (s *DrpySource) Categories(ctx context.Context) ([]DrpyCategory, error) {
	s.catMu.Lock()
	if len(s.cats) > 0 && time.Now().Before(s.catsExpire) {
		cats := s.cats
		s.catMu.Unlock()
		return cats, nil
	}
	s.catMu.Unlock()

	cats, err := s.client.Categories(ctx)
	if err != nil {
		return nil, err
	}
	s.catMu.Lock()
	s.cats = cats
	s.catsExpire = time.Now().Add(drpyCategoryTTL)
	s.catMu.Unlock()
	return cats, nil
}

// Feed 返回首页混合流：勾选栏目的第一页并发拉回、合并后随机打乱。
// dir 传的不是路径（不含 / 开头）时当作临时关键词搜索，方便手工调试；
// 以 / 开头的是网盘目录语义，drpy 忽略，走栏目混排。
func (s *DrpySource) Feed(ctx context.Context, dir, _ string) ([]FeedItem, error) {
	if dir != "" && !strings.HasPrefix(dir, "/") {
		return s.searchFeed(ctx, dir)
	}
	return s.categoryFeed(ctx)
}

// searchFeed 按关键词搜索一批视频（调试用，也兼容旧配置里的搜索词）。
func (s *DrpySource) searchFeed(ctx context.Context, keyword string) ([]FeedItem, error) {
	videos, err := s.client.SearchVideos(ctx, keyword)
	if err != nil {
		return nil, err
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("搜索「%s」没有结果", keyword)
	}
	return s.toFeedItems(videos, "", ""), nil
}

// categoryFeed 并发请求各栏目的第一页，合并成一个大切片后洗牌。
// 每条视频都带上栏目 ID 与栏目名，前端按栏目合成合集。
func (s *DrpySource) categoryFeed(ctx context.Context) ([]FeedItem, error) {
	return s.FeedCategories(ctx, s.categories)
}

// FeedCategories 拉取指定栏目的第一页并合并洗牌；ids 为空时用源返回的第一个栏目。
// 多数据源模式下，栏目范围由每个源自己的配置决定。
func (s *DrpySource) FeedCategories(ctx context.Context, ids []string) ([]FeedItem, error) {
	cats, err := s.Categories(ctx)
	if err != nil && len(ids) == 0 {
		return nil, err
	}
	if err != nil {
		slog.Warn("获取 drpy 栏目列表失败，栏目名用 ID 兜底", "err", err)
	}
	names := make(map[string]string, len(cats))
	for _, c := range cats {
		names[c.ID] = c.Name
	}

	if len(ids) == 0 {
		if len(cats) == 0 {
			return nil, errors.New("源没有可用的栏目")
		}
		ids = []string{cats[0].ID}
	}

	type result struct {
		id    string
		items []FeedItem
		err   error
	}
	// 勾选栏目多时每个栏目只取前几条，首页总滑屏数不会跟着线性膨胀
	limit := feedPerCategoryCount(len(ids))
	// 整体预算 + 并发上限：不让几十个栏目同时打满上游，超时后返回已拿到的部分
	ctx, cancel := context.WithTimeout(ctx, feedSourceTimeout)
	defer cancel()
	results := make(chan result, len(ids))
	sem := make(chan struct{}, feedCategoryWorkers)
	var wg sync.WaitGroup
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{id: id, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			videos, err := s.client.CategoryVideos(ctx, id)
			if err != nil {
				results <- result{id: id, err: err}
				return
			}
			results <- result{id: id, items: capFeedItems(s.toFeedItems(videos, id, names[id]), limit)}
		}(id)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]FeedItem, 0, 160)
	var firstErr error
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			slog.Warn("拉取 drpy 栏目失败", "category", r.id, "err", r.err)
			continue
		}
		all = append(all, r.items...)
	}
	if len(all) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("拉取栏目内容失败: %w", firstErr)
		}
		return nil, errors.New("勾选的栏目都没有内容")
	}

	rand.Shuffle(len(all), func(i, j int) {
		all[i], all[j] = all[j], all[i]
	})
	return all, nil
}

// AllFeedItems 拉取某个栏目的全部视频（不含播放直链），供前端合集面板完整展示。
// 首页 /api/feed 仍然只取第一页，避免一次构建上千个播放滑屏。
func (s *DrpySource) AllFeedItems(ctx context.Context, categoryID string) ([]FeedItem, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, errors.New("缺少 category_id")
	}

	videos, err := s.client.AllCategoryVideos(ctx, categoryID)
	if err != nil {
		return nil, err
	}

	name := categoryID
	if cats, err := s.Categories(ctx); err == nil {
		for _, c := range cats {
			if c.ID == categoryID {
				name = c.Name
				break
			}
		}
	}
	return s.toFeedItems(videos, categoryID, name), nil
}

// toFeedItems 把 drpy 视频映射成通用 Feed 条目，并带上所属栏目。
// 配了 sourceID 时路径会带源 ID，播放接口据此找到对应源实例。
func (s *DrpySource) toFeedItems(videos []DrpyVideo, categoryID, categoryName string) []FeedItem {
	items := make([]FeedItem, 0, len(videos))
	for _, v := range videos {
		itemPath := DrpyItemPath(v.VodID)
		if s.sourceID != "" {
			itemPath = OnlineItemPath(s.sourceID, v.VodID)
		}
		items = append(items, FeedItem{
			Name:       v.VodName,
			Path:       itemPath,
			CoverURL:   v.VodPic,
			CategoryID: categoryID,
			Category:   categoryName,
			SourceID:   s.sourceID,
		})
	}
	return items
}

// Resolve 调详情接口取第一集，再按需调 play 接口把网页地址解析成真实媒体链接。
// Referer 用目标站点自身的 origin：绝大多数防盗链只校验「是不是自家站点」。
func (s *DrpySource) Resolve(ctx context.Context, itemPath, _ string) (*PlaySource, error) {
	vodID := strings.TrimPrefix(itemPath, DrpyPathPrefix)
	if vodID == "" {
		return nil, fmt.Errorf("缺少 vod_id（%s）", itemPath)
	}

	s.mu.Lock()
	cached, ok := s.cache[vodID]
	s.mu.Unlock()
	if ok && time.Now().Before(cached.expire) {
		return s.playSource(cached.url, nil), nil
	}

	episodes, err := s.client.GetPlayUrl(ctx, vodID)
	if err != nil {
		return nil, err
	}
	first := episodes[0]
	target := first.URL
	var header http.Header

	// 一级/嗅探型源的第一集是详情网页（没有媒体后缀），必须换成 play 接口返回的直链，
	// 否则浏览器拿到的是整页 HTML，播放器只会报「视频加载不出来」。
	if !isMediaLink(target) {
		result, err := s.client.PlayMedia(ctx, first.URL, first.Flag)
		switch {
		case err != nil:
			slog.Warn("drpy play 接口解析失败，回退为原始地址", "vod_id", vodID, "url", first.URL, "err", err)
		case result.URL != "":
			target = result.URL
			header = result.Header
		}
	}

	s.mu.Lock()
	s.cache[vodID] = cachedPlay{url: target, expire: time.Now().Add(drpyEpisodeTTL)}
	s.mu.Unlock()
	return s.playSource(target, header), nil
}

// Invalidate 丢掉某个条目的直链缓存，供上游返回 403 时刷新重试。
func (s *DrpySource) Invalidate(_ context.Context, itemPath string) {
	vodID := strings.TrimPrefix(itemPath, DrpyPathPrefix)
	s.mu.Lock()
	delete(s.cache, vodID)
	s.mu.Unlock()
}

// playSource 组装上游请求头：浏览器 UA + 目标站 origin 作为 Referer；
// 源在 play 响应里给了 header 时以源为准（覆盖默认值）。
func (s *DrpySource) playSource(target string, extra http.Header) *PlaySource {
	header := http.Header{}
	header.Set("User-Agent", drpyUA)
	header.Set("Accept", "*/*")
	if u, err := url.Parse(target); err == nil && u.Host != "" {
		header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	}
	for key, values := range extra {
		for _, v := range values {
			header.Set(key, v)
		}
	}
	return &PlaySource{URL: target, Header: header}
}
