package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 贝乐虎（vd.ubestkid.com）是「全函数」drpy 规则：栏目来自 class_parse 里的
// gzip 数据，列表是 POST JSON，播放地址直接取 URL。规则无法用通用解析器执行，
// 这里按它公开的接口做内置直连，上传这份 JS 时自动识别并走这里。

const (
	ubestkidHostFrag = "ubestkid.com"
	iosUA            = "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"
	ubestkidMaxPage  = 60
)

// UbestkidSource 实现贝乐虎数据源。
type UbestkidSource struct {
	rule     *scriptRule
	client   *http.Client
	sourceID string
}

type ubestkidVideo struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Image string `json:"image"`
}

type ubestkidResp struct {
	Result struct {
		Items []ubestkidVideo `json:"items"`
	} `json:"result"`
}

func isUbestkidHost(host string) bool {
	return strings.Contains(strings.ToLower(host), ubestkidHostFrag)
}

func NewUbestkidSource(rule *scriptRule, timeout time.Duration) (*UbestkidSource, error) {
	if timeout <= 0 {
		timeout = rule.Timeout
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &UbestkidSource{
		rule:   rule,
		client: &http.Client{Timeout: timeout},
	}, nil
}

func (u *UbestkidSource) SetSourceID(id string) { u.sourceID = strings.TrimSpace(id) }

// Categories 直接返回规则里 gzip 解码出来的静态栏目。
func (u *UbestkidSource) Categories(ctx context.Context) ([]DrpyCategory, error) {
	if len(u.rule.Classes) == 0 {
		return nil, errors.New("规则里没有解析出贝乐虎栏目")
	}
	return u.rule.Classes, nil
}

func (u *UbestkidSource) FeedCategories(ctx context.Context, ids []string) ([]FeedItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cats, _ := u.Categories(ctx)
	names := make(map[string]string, len(cats))
	for _, c := range cats {
		names[c.ID] = c.Name
	}

	// 勾选栏目多时每个栏目只取前几条，首页总滑屏数不会跟着线性膨胀
	limit := feedPerCategoryCount(len(ids))
	// 并发上限 + 整体预算：栏目多时不再一个个串行等，也不会一次打满上游
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

			videos, err := u.page(ctx, id, 1)
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{items: capFeedItems(u.toFeedItems(videos, id, names[id]), limit)}
		}(id)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]FeedItem, 0, limit*len(ids))
	var firstErr error
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

func (u *UbestkidSource) AllFeedItems(ctx context.Context, categoryID string) ([]FeedItem, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, errors.New("缺少 category_id")
	}
	name := categoryID
	if cats, err := u.Categories(ctx); err == nil {
		for _, c := range cats {
			if c.ID == categoryID {
				name = c.Name
				break
			}
		}
	}

	all := make([]ubestkidVideo, 0, 60)
	seen := make(map[string]bool)
	for p := 1; p <= ubestkidMaxPage; p++ {
		videos, err := u.page(ctx, categoryID, p)
		if err != nil {
			if p == 1 {
				return nil, err
			}
			break
		}
		added := 0
		for _, v := range videos {
			if v.URL == "" || seen[v.URL] {
				continue
			}
			seen[v.URL] = true
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
	return u.toFeedItems(all, categoryID, name), nil
}

// Resolve 的条目键既可能是纯视频地址，也可能是 drpy 的 `url@@标题@@封面`。
func (u *UbestkidSource) Resolve(ctx context.Context, itemKey string) (*PlaySource, error) {
	itemKey = strings.TrimSpace(itemKey)
	if itemKey == "" {
		return nil, errors.New("缺少视频 ID")
	}
	media := strings.Split(itemKey, "@@")[0]
	if !isHTTPURL(media) {
		return nil, fmt.Errorf("贝乐虎条目不是可播放地址（%s）", itemKey)
	}
	header := http.Header{}
	header.Set("User-Agent", iosUA)
	header.Set("Accept", "*/*")
	header.Set("Referer", strings.TrimRight(u.rule.Host, "/")+"/")
	return &PlaySource{URL: media, Header: header}, nil
}

func (u *UbestkidSource) Invalidate(string) {}

func (u *UbestkidSource) toFeedItems(videos []ubestkidVideo, categoryID, categoryName string) []FeedItem {
	items := make([]FeedItem, 0, len(videos))
	for _, v := range videos {
		if v.URL == "" {
			continue
		}
		items = append(items, FeedItem{
			Name:       v.Title,
			Path:       OnlineItemPath(u.sourceID, v.URL),
			CoverURL:   v.Image,
			CategoryID: categoryID,
			Category:   categoryName,
			SourceID:   u.sourceID,
		})
	}
	return items
}

// page 取某栏目第 p 页。上游接口是 POST JSON，分页字段为 p。
func (u *UbestkidSource) page(ctx context.Context, categoryID string, p int) ([]ubestkidVideo, error) {
	var subcate any = categoryID
	if n, err := strconv.Atoi(categoryID); err == nil {
		subcate = n
	}
	payload := map[string]any{
		"age":         1,
		"appver":      "6.1.9",
		"egvip_status": 0,
		"svip_status": 0,
		"vps":         60,
		"subcateId":   subcate,
		"p":           p,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	target := ubestkidListURL(u.rule)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	ua := u.rule.Headers["User-Agent"]
	if ua == "" {
		ua = iosUA
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", strings.TrimRight(u.rule.Host, "/")+"/")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求贝乐虎列表失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("贝乐虎列表返回状态码 %d（响应片段: %s）", resp.StatusCode, snippet(raw))
	}

	var out ubestkidResp
	if err := json.Unmarshal(unwrapJSONPBytes(raw), &out); err != nil {
		return nil, fmt.Errorf("贝乐虎列表解析失败: %w（响应片段: %s）", err, snippet(raw))
	}
	return out.Result.Items, nil
}

// ubestkidListURL 取规则 url 中 `#` 之前的部分，补成完整地址。
func ubestkidListURL(rule *scriptRule) string {
	tpl := rule.URLTpl
	if i := strings.Index(tpl, "#"); i >= 0 {
		tpl = tpl[:i]
	}
	tpl = strings.TrimSpace(tpl)
	if strings.HasPrefix(tpl, "http://") || strings.HasPrefix(tpl, "https://") {
		return tpl
	}
	return strings.TrimRight(rule.Host, "/") + "/" + strings.TrimLeft(tpl, "/")
}
