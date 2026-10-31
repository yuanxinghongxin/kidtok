package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TuxiaobeiHost 是兔小贝站点根地址，对应 drpy 规则里的 host 字段。
const TuxiaobeiHost = "https://www.tuxiaobei.com"

const tuxiaobeiUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"

// Tuxiaobei 按规则文件暴露的 HTTP 接口直接访问兔小贝站点，不依赖 JS 引擎。
type Tuxiaobei struct {
	client *http.Client
}

// NewTuxiaobei 创建站点客户端。
func NewTuxiaobei(timeout time.Duration) *Tuxiaobei {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Tuxiaobei{client: &http.Client{Timeout: timeout}}
}

// TuxiaobeiItem 是列表或搜索结果中的一个节目。
type TuxiaobeiItem struct {
	VideoID  int    `json:"video_id"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
	Image    string `json:"image,omitempty"`
	Duration string `json:"duration,omitempty"`
	PlayNum  string `json:"play_num,omitempty"`
	URL      string `json:"url,omitempty"`
}

// TuxiaobeiDetail 是详情页解析结果。
type TuxiaobeiDetail struct {
	VideoID int    `json:"video_id"`
	Name    string `json:"name"`
	Poster  string `json:"poster,omitempty"`
	PlayURL string `json:"play_url"`
}

// List 对应规则里的 url 字段：/list/mip-data?typeId=fyclass&page=fypage&callback=
// 返回值是 JSONP，形如 ({"status":0,"data":{"items":[...]}});
func (t *Tuxiaobei) List(ctx context.Context, typeID string, page int) ([]TuxiaobeiItem, error) {
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/list/mip-data?typeId=%s&page=%d&callback=", TuxiaobeiHost, typeID, page)
	body, err := t.get(ctx, u)
	if err != nil {
		return nil, err
	}

	payload := unwrapJSONP(body)
	var resp struct {
		Status int `json:"status"`
		Data   struct {
			Items []struct {
				VideoID        int    `json:"video_id"`
				Name           string `json:"name"`
				Image          string `json:"image"`
				CategoryName   string `json:"category_name"`
				DurationString string `json:"duration_string"`
				PlayNum        string `json:"play_num"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		return nil, fmt.Errorf("解析列表 JSONP 失败: %w", err)
	}
	if resp.Status != 0 {
		return nil, fmt.Errorf("站点返回非 0 状态: %d", resp.Status)
	}

	items := make([]TuxiaobeiItem, 0, len(resp.Data.Items))
	for _, it := range resp.Data.Items {
		items = append(items, TuxiaobeiItem{
			VideoID:  it.VideoID,
			Name:     it.Name,
			Category: it.CategoryName,
			Image:    it.Image,
			Duration: it.DurationString,
			PlayNum:  it.PlayNum,
			URL:      fmt.Sprintf("%s/play/%d", TuxiaobeiHost, it.VideoID),
		})
	}
	return items, nil
}

var (
	reSearchItem = regexp.MustCompile(`<div class="items">([\s\S]*?)</div>\s*</div>`)
	rePlayID     = regexp.MustCompile(`href="/play/(\d+)"`)
	reMipImg     = regexp.MustCompile(`<mip-img src="([^"]+)"`)
	reTitle      = regexp.MustCompile(`<p class="title">([^<]+)</p>`)
	reSort       = regexp.MustCompile(`<p class="sort">([^<]+)</p>`)
	reDuration   = regexp.MustCompile(`<p class="time">([^<]+)</p>`)
	reVideoSrc   = regexp.MustCompile(`video-src="([^"]+)"`)
	rePageTitle  = regexp.MustCompile(`<title>([^<]+)</title>`)
)

// Search 对应规则里的 searchUrl：/search/index?key=** （** 就是关键词占位符）。
// 返回 HTML，用规则里的「搜索」选择器语义解析：.list-con .items -> 标题/封面/时长/链接。
func (t *Tuxiaobei) Search(ctx context.Context, keyword string) ([]TuxiaobeiItem, error) {
	u := TuxiaobeiHost + "/search/index?key=" + escapeQuery(keyword)
	body, err := t.get(ctx, u)
	if err != nil {
		return nil, err
	}

	matches := reSearchItem.FindAllStringSubmatch(body, -1)
	items := make([]TuxiaobeiItem, 0, len(matches))
	for _, m := range matches {
		block := m[1]
		idPart := rePlayID.FindStringSubmatch(block)
		titlePart := reTitle.FindStringSubmatch(block)
		if idPart == nil || titlePart == nil {
			continue
		}
		id, _ := strconv.Atoi(idPart[1])
		item := TuxiaobeiItem{VideoID: id, Name: strings.TrimSpace(titlePart[1]), URL: fmt.Sprintf("%s/play/%d", TuxiaobeiHost, id)}
		if v := reMipImg.FindStringSubmatch(block); v != nil {
			item.Image = v[1]
		}
		if v := reSort.FindStringSubmatch(block); v != nil {
			item.Category = strings.TrimSpace(v[1])
		}
		if v := reDuration.FindStringSubmatch(block); v != nil {
			item.Duration = strings.TrimSpace(v[1])
		}
		items = append(items, item)
	}
	return items, nil
}

// Detail 对应规则里的 detailUrl：/play/fyid，页面内 #videoWrap 的 video-src 即直链。
func (t *Tuxiaobei) Detail(ctx context.Context, videoID int) (*TuxiaobeiDetail, error) {
	u := fmt.Sprintf("%s/play/%d", TuxiaobeiHost, videoID)
	body, err := t.get(ctx, u)
	if err != nil {
		return nil, err
	}

	src := reVideoSrc.FindStringSubmatch(body)
	if src == nil {
		return nil, fmt.Errorf("详情页 %d 中没有找到 video-src", videoID)
	}
	detail := &TuxiaobeiDetail{VideoID: videoID, PlayURL: src[1]}
	if v := rePageTitle.FindStringSubmatch(body); v != nil {
		detail.Name = strings.TrimSpace(v[1])
	}
	if v := regexp.MustCompile(`poster="([^"]+)"`).FindStringSubmatch(body); v != nil {
		detail.Poster = v[1]
	}
	return detail, nil
}

func (t *Tuxiaobei) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", tuxiaobeiUA)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 %s 失败: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("请求 %s 返回状态码 %d", url, resp.StatusCode)
	}
	return string(body), nil
}

// unwrapJSONP 去掉 JSONP 的外层 ( )
func unwrapJSONP(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "(") {
		s = s[1:]
	}
	if strings.HasSuffix(s, ");") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, ")") {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s)
}

func escapeQuery(s string) string {
	return url.QueryEscape(s)
}
