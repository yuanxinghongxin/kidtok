package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScriptSource 用「上传/粘贴的 drpy 规则 JS」当数据源，不依赖 drpy-node。
//
// drpy 的规则文件本质是一个配置对象（var rule = {...}），常见字段是
// host / url（列表模板）/ detailUrl / searchUrl / class 或 class_parse。
// 这里只解析这些声明式字段，用它们的 URL 模板直接请求上游并解析返回的
// JSON(P) 或 HTML；规则里带一级/二级自定义函数时无法执行，会明确报错。
type ScriptSource struct {
	rule     *scriptRule
	client   *http.Client
	sourceID string
	// 栏目真实总条数缓存：接口不返回 total 时靠翻页探测得到，探测一次缓存半小时
	totalMu sync.Mutex
	totals  map[string]scriptTotal
}

type scriptTotal struct {
	total int
	at    time.Time
}

type scriptRule struct {
	Title     string
	Host      string
	HomeURL   string
	URLTpl    string
	DetailTpl string
	SearchTpl string
	Headers   map[string]string
	Timeout   time.Duration
	// ClassParse 是 drpy 的 `class_parse` 原始值：
	// "容器选择器;名称选择器;链接选择器;ID 正则"。
	ClassParse string
	Classes    []DrpyCategory
	// HasFuncRule 表示规则里带自定义函数（一级/二级/搜索），目前无法在 Go 里执行。
	HasFuncRule bool
}

// NewScriptSource 解析规则 JS 并创建数据源；规则不完整时返回错误。
func NewScriptSource(script string, timeout time.Duration) (*ScriptSource, error) {
	rule, err := parseScriptRule(script)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = rule.Timeout
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ScriptSource{
		rule:   rule,
		client: &http.Client{Timeout: timeout},
	}, nil
}

// SetSourceID 设置多数据源配置里的源 ID。
func (s *ScriptSource) SetSourceID(id string) { s.sourceID = strings.TrimSpace(id) }

// Name 实现数据源标识。
func (s *ScriptSource) Name() string { return "js" }

// Title 返回规则里的源名，供设置页显示。
func (s *ScriptSource) Title() string { return s.rule.Title }

// Categories 返回规则声明的栏目：静态 class 数组优先，其次用 class_parse 抓首页。
func (s *ScriptSource) Categories(ctx context.Context) ([]DrpyCategory, error) {
	if len(s.rule.Classes) > 0 {
		return s.rule.Classes, nil
	}
	if s.rule.ClassParse != "" {
		cats, err := s.parseClasses(ctx)
		if err == nil && len(cats) > 0 {
			return cats, nil
		}
		if err != nil {
			return nil, fmt.Errorf("解析规则栏目失败: %w", err)
		}
	}
	return nil, errors.New("规则里没有可识别的分类（缺少静态 class 或 class_parse）")
}

// FeedCategories 拉取勾选栏目的第一页；ids 为空时用第一个栏目。
func (s *ScriptSource) FeedCategories(ctx context.Context, ids []string) ([]FeedItem, error) {
	ids = append([]string(nil), ids...)
	if len(ids) == 0 {
		cats, err := s.Categories(ctx)
		if err != nil {
			return nil, err
		}
		ids = []string{cats[0].ID}
	}

	cats, _ := s.Categories(ctx)
	names := make(map[string]string, len(cats))
	for _, c := range cats {
		names[c.ID] = c.Name
	}

	limit := feedPerCategoryCount(len(ids))
	all := make([]FeedItem, 0, limit*len(ids))
	var firstErr error

	// 栏目并发拉取：接口不返回总数时还要翻页探测，串行会明显拖慢首页
	ctx, cancel := context.WithTimeout(ctx, feedSourceTimeout)
	defer cancel()
	type result struct {
		items []FeedItem
		err   error
	}
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
				results <- result{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			videos, total, err := s.categoryPage(ctx, id, 1)
			if err != nil {
				results <- result{err: err}
				return
			}
			// 接口不返回 total 的源（如兔小贝）用翻页探测算真实总条数，
			// 否则前端只能按已加载的 30 条显示「共 30 集」
			if total <= 0 {
				total = s.discoverTotal(ctx, id, len(videos))
			}
			results <- result{items: capFeedItems(s.toFeedItems(videos, id, names[id], total), limit)}
		}(id)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		all = append(all, r.items...)
	}
	if len(all) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, errors.New("勾选的栏目都没有内容")
	}
	return all, nil
}

// AllFeedItems 逐页拉取某个栏目的全部视频，供合集面板完整展示。
func (s *ScriptSource) AllFeedItems(ctx context.Context, categoryID string) ([]FeedItem, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, errors.New("缺少 category_id")
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

	all := make([]scriptVideo, 0, 60)
	seen := make(map[string]bool)
	total := 0
	for page := 1; page <= drpyMaxAllPages; page++ {
		videos, pageTotal, err := s.categoryPage(ctx, categoryID, page)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if pageTotal > total {
			total = pageTotal
		}
		added := 0
		for _, v := range videos {
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			all = append(all, v)
			added++
		}
		if added == 0 {
			break
		}
	}
	if len(all) == 0 {
		return nil, errors.New("栏目没有返回任何视频")
	}
	if total <= 0 {
		total = len(all)
	}
	s.storeTotal(categoryID, total)
	return s.toFeedItems(all, categoryID, name, total), nil
}

// Resolve 打开详情页/详情接口，找出第一集可播放的媒体地址。
func (s *ScriptSource) Resolve(ctx context.Context, itemKey string) (*PlaySource, error) {
	itemKey = strings.TrimSpace(itemKey)
	if itemKey == "" {
		return nil, errors.New("缺少视频 ID")
	}

	// 列表里直接给媒体链接的源，不需要再进详情页。
	if isMediaLink(itemKey) && isHTTPURL(itemKey) {
		return s.playSource(itemKey), nil
	}
	if isHTTPURL(itemKey) && !strings.Contains(itemKey, "/play/") {
		// 已经是完整的网页地址：直接抓页面找媒体链接。
		if media := s.sniffPage(ctx, itemKey); media != "" {
			return s.playSource(media), nil
		}
		return nil, fmt.Errorf("详情页里没有找到可播放的媒体地址（%s）", itemKey)
	}

	if s.rule.DetailTpl == "" {
		return nil, fmt.Errorf("规则没有 detailUrl，无法解析 %s", itemKey)
	}
	tpl := strings.ReplaceAll(s.rule.DetailTpl, "fyid", itemKey)
	detailURL := tpl
	if !strings.HasPrefix(tpl, "http://") && !strings.HasPrefix(tpl, "https://") {
		detailURL = strings.TrimRight(s.rule.Host, "/") + "/" + strings.TrimLeft(tpl, "/")
	}

	body, err := s.get(ctx, detailURL)
	if err != nil {
		return nil, err
	}

	if media := findPlayURLInJSON(body); media != "" {
		return s.playSource(media), nil
	}
	if media := findMediaURLInText(string(body)); media != "" {
		return s.playSource(media), nil
	}
	return nil, fmt.Errorf("详情页里没有找到可播放的媒体地址（%s）", detailURL)
}

// Invalidate 目前 JS 源不做直链缓存，留空以兼容视频流的 403 重试流程。
func (s *ScriptSource) Invalidate(string) {}

func (s *ScriptSource) toFeedItems(videos []scriptVideo, categoryID, categoryName string, total int) []FeedItem {
	items := make([]FeedItem, 0, len(videos))
	for _, v := range videos {
		itemPath := OnlineItemPath(s.sourceID, v.ID)
		items = append(items, FeedItem{
			Name:       v.Name,
			Path:       itemPath,
			CoverURL:   v.Pic,
			CategoryID: categoryID,
			Category:   categoryName,
			SourceID:   s.sourceID,
			TotalCount: total,
		})
	}
	return items
}

// scriptVideo 是从规则列表里解析出的一条视频。
type scriptVideo struct {
	ID   string
	Name string
	Pic  string
}

// categoryPage 拉取某栏目第 page 页并解析出视频列表，同时尽量带回真实总条数。
func (s *ScriptSource) categoryPage(ctx context.Context, categoryID string, page int) ([]scriptVideo, int, error) {
	if s.rule.URLTpl == "" {
		return nil, 0, errors.New("规则没有 url 列表模板")
	}
	target := s.fillTemplate(s.rule.URLTpl, categoryID, page)
	body, err := s.get(ctx, target)
	if err != nil {
		return nil, 0, err
	}

	total := jsonTotal(body)
	if videos := parseJSONVideos(body); len(videos) > 0 {
		return videos, total, nil
	}
	if videos := parseHTMLVideos(body); len(videos) > 0 {
		return videos, total, nil
	}
	// 合法 JSON 但没有条目 = 越界页（如兔小贝翻到最后一页返回空 items），
	// 这是翻页探测判断结尾的依据，不能当作解析失败
	if json.Valid(body) {
		return nil, total, nil
	}
	return nil, 0, fmt.Errorf("列表接口没有解析出视频（%s，响应片段: %s）", target, snippet(body))
}

// jsonTotal 从列表响应里找真实总数（total / totalCount / count 等常见键）。
// 找不到时返回 0，前端会退回按已加载条数展示，不会显示错误的数字。
func jsonTotal(body []byte) int {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return 0
	}
	return findTotal(v, 0)
}

var totalKeys = []string{"total", "totalcount", "total_count"}

func findTotal(v any, depth int) int {
	if depth > 4 {
		return 0
	}
	switch val := v.(type) {
	case map[string]any:
		for _, key := range totalKeys {
			for k, raw := range val {
				if !strings.EqualFold(k, key) {
					continue
				}
				if n := toInt(raw); n > 0 {
					return n
				}
			}
		}
		// count 只认响应最外层，避免把某条视频的播放数/评论数当成总数
		if depth <= 1 {
			for k, raw := range val {
				if strings.EqualFold(k, "count") {
					if n := toInt(raw); n > 0 {
						return n
					}
				}
			}
		}
		for _, raw := range val {
			if n := findTotal(raw, depth+1); n > 0 {
				return n
			}
		}
	case []any:
		for _, raw := range val {
			if n := findTotal(raw, depth+1); n > 0 {
				return n
			}
		}
	}
	return 0
}

func toInt(raw any) int {
	switch n := raw.(type) {
	case float64:
		return int(n)
	case string:
		if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return v
		}
	}
	return 0
}

// scriptTotalTTL 是「翻页探测出来的总条数」缓存时长。
const scriptTotalTTL = 30 * time.Minute

// discoverTotal 在列表接口没有 total/pagecount 时，靠翻页探测真实总条数：
// 先按 2、4、8… 指数翻页找到第一个空页，再二分定位最后一页，最后按
// 「(最后一页-1) * 每页条数 + 最后一页条数」还原总数。
// 探测最多十几次请求且整体限时 8 秒；失败返回 0，前端退回按已加载条数显示。
func (s *ScriptSource) discoverTotal(ctx context.Context, categoryID string, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	if total, ok := s.cachedTotal(categoryID); ok {
		return total
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	probes := 0
	lo, hi := 1, 0
	counts := map[int]int{}
	for step := 2; step <= 64 && probes < 10; step *= 2 {
		if ctx.Err() != nil {
			return 0
		}
		videos, _, err := s.categoryPage(ctx, categoryID, step)
		if err != nil {
			return 0
		}
		probes++
		counts[step] = len(videos)
		if len(videos) == 0 {
			hi = step
			break
		}
		lo = step
		if len(videos) < pageSize {
			// 这一页没满，就是最后一页
			total := (step-1)*pageSize + len(videos)
			s.storeTotal(categoryID, total)
			return total
		}
	}
	if hi == 0 {
		hi = lo + 1
	}
	for hi-lo > 1 && probes < 16 {
		mid := (lo + hi) / 2
		if ctx.Err() != nil {
			return 0
		}
		videos, _, err := s.categoryPage(ctx, categoryID, mid)
		if err != nil {
			return 0
		}
		probes++
		counts[mid] = len(videos)
		if len(videos) == 0 {
			hi = mid
		} else {
			lo = mid
		}
	}

	last, ok := counts[lo]
	if !ok || last == 0 {
		videos, _, err := s.categoryPage(ctx, categoryID, lo)
		if err != nil {
			return 0
		}
		last = len(videos)
	}
	if last <= 0 {
		return 0
	}
	total := (lo-1)*pageSize + last
	s.storeTotal(categoryID, total)
	return total
}

func (s *ScriptSource) cachedTotal(categoryID string) (int, bool) {
	s.totalMu.Lock()
	defer s.totalMu.Unlock()
	e, ok := s.totals[categoryID]
	if !ok || time.Since(e.at) > scriptTotalTTL {
		return 0, false
	}
	return e.total, true
}

func (s *ScriptSource) storeTotal(categoryID string, total int) {
	if total <= 0 {
		return
	}
	s.totalMu.Lock()
	if s.totals == nil {
		s.totals = make(map[string]scriptTotal)
	}
	s.totals[categoryID] = scriptTotal{total: total, at: time.Now()}
	s.totalMu.Unlock()
}

// fillTemplate 把 drpy 规则模板里的占位符替换成实际值。
func (s *ScriptSource) fillTemplate(tpl, categoryID string, page int) string {
	tpl = strings.ReplaceAll(tpl, "fyclass", categoryID)
	tpl = strings.ReplaceAll(tpl, "fypage", strconv.Itoa(page))
	if strings.HasPrefix(tpl, "http://") || strings.HasPrefix(tpl, "https://") {
		return tpl
	}
	return strings.TrimRight(s.rule.Host, "/") + "/" + strings.TrimLeft(tpl, "/")
}

// get 发起带规则请求头的 GET，并统一解开 JSONP 外壳。
func (s *ScriptSource) get(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	ua := s.rule.Headers["User-Agent"]
	if ua == "" {
		ua = drpyUA
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if ref := s.rule.Headers["Referer"]; ref != "" {
		req.Header.Set("Referer", ref)
	} else if s.rule.Host != "" {
		req.Header.Set("Referer", strings.TrimRight(s.rule.Host, "/")+"/")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", target, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求 %s 返回状态码 %d（响应片段: %s）", target, resp.StatusCode, snippet(body))
	}
	return unwrapJSONPBytes(body), nil
}

// sniffPage 抓一个网页，在 HTML / 内嵌 JSON 里找媒体直链。
func (s *ScriptSource) sniffPage(ctx context.Context, target string) string {
	body, err := s.get(ctx, target)
	if err != nil {
		return ""
	}
	if media := findPlayURLInJSON(body); media != "" {
		return media
	}
	return findMediaURLInText(string(body))
}

// playSource 按目标站点 origin 补 Referer，并带上规则里的请求头。
func (s *ScriptSource) playSource(target string) *PlaySource {
	header := http.Header{}
	ua := s.rule.Headers["User-Agent"]
	if ua == "" {
		ua = drpyUA
	}
	header.Set("User-Agent", ua)
	header.Set("Accept", "*/*")
	ref := s.rule.Headers["Referer"]
	if ref == "" {
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			ref = u.Scheme + "://" + u.Host + "/"
		} else if s.rule.Host != "" {
			ref = strings.TrimRight(s.rule.Host, "/") + "/"
		}
	}
	if ref != "" {
		header.Set("Referer", ref)
	}
	return &PlaySource{URL: target, Header: header}
}

// parseClasses 用 class_parse 抓首页栏目。
func (s *ScriptSource) parseClasses(ctx context.Context) ([]DrpyCategory, error) {
	target := strings.TrimRight(s.rule.Host, "/")
	if s.rule.HomeURL != "" {
		if strings.HasPrefix(s.rule.HomeURL, "http") {
			target = s.rule.HomeURL
		} else {
			target += "/" + strings.TrimLeft(s.rule.HomeURL, "/")
		}
	}
	body, err := s.get(ctx, target)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(s.rule.ClassParse, ";")
	idRe := `(?:[^\s/"']+)`
	if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
		idRe = strings.TrimSpace(parts[3])
	}
	re, err := regexp.Compile(idRe)
	if err != nil {
		return nil, fmt.Errorf("class_parse 里的 ID 正则不合法: %w", err)
	}

	anchors := hrefAnchorRe.FindAllStringSubmatch(string(body), -1)
	cats := make([]DrpyCategory, 0, 16)
	seen := make(map[string]bool)
	for _, m := range anchors {
		href := html.UnescapeString(strings.TrimSpace(m[1]))
		text := strings.TrimSpace(stripTags(m[2]))
		if href == "" || text == "" {
			continue
		}
		match := re.FindStringSubmatch(href)
		if len(match) == 0 {
			continue
		}
		id := match[1]
		if len(match) == 1 {
			id = match[0]
		}
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cats = append(cats, DrpyCategory{ID: id, Name: text})
	}
	if len(cats) == 0 {
		return nil, errors.New("class_parse 没有匹配到任何栏目")
	}
	return cats, nil
}

var hrefAnchorRe = regexp.MustCompile(`(?is)<a\b[^>]*href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a>`)

var tagRe = regexp.MustCompile(`(?s)<[^>]+>`)

// stripTags 去掉 HTML 标签并解码常见实体。
func stripTags(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// parseJSONVideos 从 JSON / JSONP 响应里找出视频数组。
func parseJSONVideos(body []byte) []scriptVideo {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil
	}
	arrays := collectArrays(v, 0)
	best := make([]scriptVideo, 0)
	for _, arr := range arrays {
		items := mapJSONArray(arr)
		if len(items) > len(best) {
			best = items
		}
	}
	return best
}

// collectArrays 递归收集 JSON 里所有「至少两个对象元素」的数组。
func collectArrays(v any, depth int) [][]any {
	if depth > 8 {
		return nil
	}
	out := make([][]any, 0)
	switch val := v.(type) {
	case []any:
		if len(val) >= 2 {
			out = append(out, val)
		}
		for _, item := range val {
			out = append(out, collectArrays(item, depth+1)...)
		}
	case map[string]any:
		for _, item := range val {
			out = append(out, collectArrays(item, depth+1)...)
		}
	}
	return out
}

// mapJSONArray 把 JSON 数组里的对象映射成视频；字段名兼容苹果 CMS 与常见别名。
func mapJSONArray(arr []any) []scriptVideo {
	out := make([]scriptVideo, 0, len(arr))
	for _, raw := range arr {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(m, "vod_id", "id", "video_id", "mip_id", "vid", "url", "link", "href")
		name := firstString(m, "vod_name", "name", "title", "mip_title")
		pic := firstString(m, "vod_pic", "pic", "cover", "img", "image", "thumb")
		if id == "" {
			continue
		}
		if name == "" {
			name = id
		}
		out = append(out, scriptVideo{ID: id, Name: name, Pic: pic})
	}
	return out
}

// firstString 返回 map 里第一个存在的字符串值。
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch val := v.(type) {
			case string:
				if strings.TrimSpace(val) != "" {
					return strings.TrimSpace(val)
				}
			case float64:
				return strconv.FormatInt(int64(val), 10)
			case json.Number:
				return val.String()
			}
		}
	}
	return ""
}

// parseHTMLVideos 从 HTML 页面里抓播放链接与标题。
func parseHTMLVideos(body []byte) []scriptVideo {
	anchors := hrefAnchorRe.FindAllStringSubmatch(string(body), -1)
	out := make([]scriptVideo, 0, len(anchors))
	seen := make(map[string]bool)
	for _, m := range anchors {
		href := html.UnescapeString(strings.TrimSpace(m[1]))
		text := strings.TrimSpace(stripTags(m[2]))
		if href == "" || text == "" || !looksLikePlayLink(href) {
			continue
		}
		if seen[href] {
			continue
		}
		seen[href] = true
		out = append(out, scriptVideo{ID: href, Name: text})
	}
	return out
}

// looksLikePlayLink 判断链接像不像一条视频详情/播放地址。
func looksLikePlayLink(href string) bool {
	lower := strings.ToLower(href)
	if isMediaLink(lower) {
		return true
	}
	for _, seg := range []string{"/play/", "/video/", "/detail", "/mv/", "/vod/"} {
		if strings.Contains(lower, seg) {
			return true
		}
	}
	return false
}

// findPlayURLInJSON 在 JSON 里递归找 vod_play_url（或 play_url）并解析第一集。
func findPlayURLInJSON(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	if raw := findJSONPlayURL(v, 0); raw != "" {
		if eps := ParseVodPlayURL(raw); len(eps) > 0 {
			return eps[0].URL
		}
		if isHTTPURL(raw) {
			return raw
		}
	}
	return ""
}

func findJSONPlayURL(v any, depth int) string {
	if depth > 8 {
		return ""
	}
	switch val := v.(type) {
	case map[string]any:
		for _, key := range []string{"vod_play_url", "play_url", "playUrl", "url"} {
			if raw, ok := val[key]; ok {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					if key != "url" || strings.Contains(s, "$") || isMediaLink(s) {
						return s
					}
				}
			}
		}
		for _, item := range val {
			if got := findJSONPlayURL(item, depth+1); got != "" {
				return got
			}
		}
	case []any:
		for _, item := range val {
			if got := findJSONPlayURL(item, depth+1); got != "" {
				return got
			}
		}
	}
	return ""
}

var mediaURLRe = regexp.MustCompile(`(?i)https?://[^\s"'<>\\]+?\.(?:m3u8|mp4|m4v|flv|mkv|webm|mov|mp3|m4a)(?:\?[^\s"'<>\\]*)?`)

// findMediaURLInText 在 HTML / 内嵌 JS 里找第一个媒体地址。
func findMediaURLInText(text string) string {
	text = strings.ReplaceAll(text, `\/`, "/")
	text = html.UnescapeString(text)
	if m := mediaURLRe.FindString(text); m != "" {
		return m
	}
	return ""
}

// unwrapJSONPBytes 去掉 callback(...) 外壳；不是 JSONP 时原样返回。
func unwrapJSONPBytes(body []byte) []byte {
	s := strings.TrimSpace(string(body))
	// 部分接口在 JSONP 末尾带一个分号，例如 `({...});`
	for strings.HasSuffix(s, ";") {
		s = strings.TrimSpace(strings.TrimSuffix(s, ";"))
	}
	if s == "" {
		return body
	}
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return body
	}
	open := strings.Index(s, "(")
	if open < 0 || !strings.HasSuffix(s, ")") {
		return body
	}
	inner := strings.TrimSpace(s[open+1 : len(s)-1])
	if !strings.HasPrefix(inner, "{") && !strings.HasPrefix(inner, "[") {
		return body
	}
	return []byte(inner)
}
