package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DrpyDefaultPwd 是 drpy-node 网页端调用 WebAPI 时带的默认口令。
const DrpyDefaultPwd = "dzyyds"

const drpyUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"

// Drpy 访问本地 drpy-node 的 WebAPI：/api/{源名}?ac=search|detail&pwd=...
//
// 源名里可能带中文和 URL 特殊字符（例如「兔小贝[儿]」），所以拼路径时必须用
// url.PathEscape；查询参数（wd、ids）里也可能带 /、$ 等字符，统一交给
// url.Values.Encode 处理，避免手动拼接出现编码错位。
type Drpy struct {
	BaseURL string // 例：http://192.168.0.201:5757
	Source  string // 例：兔小贝[儿]
	Pwd     string // 例：dzyyds
	client  *http.Client
}

// NewDrpy 创建 drpy-node 客户端；pwd 为空时用 DrpyDefaultPwd，timeout<=0 时用 10s。
func NewDrpy(baseURL, sourceName, pwd string, timeout time.Duration) *Drpy {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if pwd == "" {
		pwd = DrpyDefaultPwd
	}
	return &Drpy{
		BaseURL: baseURL,
		Source:  sourceName,
		Pwd:     pwd,
		client:  &http.Client{Timeout: timeout},
	}
}

// DrpyVideo 是搜索/列表返回的一条记录，字段对应苹果 CMS（vod_*）命名。
type DrpyVideo struct {
	VodID      string `json:"vod_id"`
	VodName    string `json:"vod_name"`
	VodPic     string `json:"vod_pic,omitempty"`
	VodRemarks string `json:"vod_remarks,omitempty"`
	TypeName   string `json:"type_name,omitempty"`
}

// DrpyEpisode 是解析出来后的一集：名称 + 可直接播放的 http(s) 直链。
type DrpyEpisode struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// drpyListResp 是 ?ac=search / ?ac=list 的响应外层。
// 不同源可能省略 code/msg，也可能带 page/total，这里只取需要的字段。
type drpyListResp struct {
	Code  int         `json:"code"`
	Msg   string      `json:"msg"`
	Total int         `json:"total"`
	List  []DrpyVideo `json:"list"`
}

// drpyDetailResp 是 ?ac=detail 的响应外层。
type drpyDetailResp struct {
	Code int `json:"code"`
	Msg  string
	List []struct {
		VodID       string `json:"vod_id"`
		VodName     string `json:"vod_name"`
		VodPic      string `json:"vod_pic"`
		VodRemarks  string `json:"vod_remarks"`
		VodPlayFrom string `json:"vod_play_from"`
		VodPlayURL  string `json:"vod_play_url"`
	} `json:"list"`
}

// SearchVideos 调用 ?ac=search&wd=关键词，返回搜索到的节目列表。
func (d *Drpy) SearchVideos(ctx context.Context, keyword string) ([]DrpyVideo, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("搜索关键词不能为空")
	}

	endpoint, err := d.endpoint("search", url.Values{"wd": {keyword}})
	if err != nil {
		return nil, err
	}
	body, err := d.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var resp drpyListResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w（响应片段: %s）", err, snippet(body))
	}
	if resp.Code != 0 && resp.Code != 1 && resp.Msg != "" {
		return nil, fmt.Errorf("搜索接口返回错误（code=%d）: %s", resp.Code, resp.Msg)
	}

	videos := make([]DrpyVideo, 0, len(resp.List))
	for _, v := range resp.List {
		if strings.TrimSpace(v.VodID) == "" || strings.TrimSpace(v.VodName) == "" {
			continue // 脏数据直接跳过，不让它污染后续详情请求
		}
		v.VodID = strings.TrimSpace(v.VodID)
		v.VodName = strings.TrimSpace(v.VodName)
		videos = append(videos, v)
	}
	return videos, nil
}

// GetPlayUrl 调用 ?ac=detail&ids=vod_id，解析 vod_play_url 并返回可直接播放的剧集列表。
// vod_id 形如 "/play/1404..."，其中的 / 由 url.Values.Encode 编码成 %2F。
func (d *Drpy) GetPlayUrl(ctx context.Context, vodID string) ([]DrpyEpisode, error) {
	vodID = strings.TrimSpace(vodID)
	if vodID == "" {
		return nil, errors.New("vod_id 不能为空")
	}

	endpoint, err := d.endpoint("detail", url.Values{"ids": {vodID}})
	if err != nil {
		return nil, err
	}
	body, err := d.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var resp drpyDetailResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析详情响应失败: %w（响应片段: %s）", err, snippet(body))
	}
	if resp.Code != 0 && resp.Code != 1 && resp.Msg != "" {
		return nil, fmt.Errorf("详情接口返回错误（code=%d）: %s", resp.Code, resp.Msg)
	}
	if len(resp.List) == 0 {
		return nil, fmt.Errorf("详情接口没有返回数据（vod_id=%s）", vodID)
	}

	episodes := ParseVodPlayURL(resp.List[0].VodPlayURL)
	if len(episodes) == 0 {
		return nil, fmt.Errorf("vod_play_url 中没有可用的 http 直链（vod_id=%s，原始值: %s）", vodID, snippet([]byte(resp.List[0].VodPlayURL)))
	}
	return episodes, nil
}

// ParseVodPlayURL 解析苹果 CMS 的 vod_play_url。
//
// 常见格式：第1集$https://a.m3u8#第2集$https://b.mp4
// 多线路时线路之间用 $$$ 分隔：线路1$url1#...#$$$线路2$url2#...
// 少数源会在一个条目里塞多个地址（名称$url1$url2），这里只取第一个有效链接。
func ParseVodPlayURL(raw string) []DrpyEpisode {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	lines := strings.Split(raw, "$$$")
	episodes := make([]DrpyEpisode, 0, 32)
	for _, line := range lines {
		for i, entry := range strings.Split(line, "#") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}

			name, link := "", entry
			if idx := strings.Index(entry, "$"); idx >= 0 {
				name = strings.TrimSpace(entry[:idx])
				link = strings.TrimSpace(entry[idx+1:])
				if j := strings.Index(link, "$"); j >= 0 {
					link = strings.TrimSpace(link[:j])
				}
			}
			if !isHTTPURL(link) {
				continue // 例如 "第1集$return ..." 这类规则片段，不是真实链接
			}
			if name == "" {
				name = fmt.Sprintf("第%d集", i+1)
			}
			episodes = append(episodes, DrpyEpisode{Name: name, URL: link})
		}
	}
	return episodes
}

// endpoint 拼接 WebAPI 地址：{base}/api/{PathEscape(源名)}?ac=...&pwd=...
func (d *Drpy) endpoint(ac string, params url.Values) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(d.BaseURL), "/")
	if base == "" {
		return "", errors.New("drpy 服务地址为空")
	}
	if _, err := url.Parse(base); err != nil {
		return "", fmt.Errorf("drpy 服务地址不合法（%s）: %w", base, err)
	}
	sourceName := strings.TrimSpace(d.Source)
	if sourceName == "" {
		return "", errors.New("drpy 源名为空")
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("ac", ac)
	if d.Pwd != "" {
		params.Set("pwd", d.Pwd)
	}
	return base + "/api/" + url.PathEscape(sourceName) + "?" + params.Encode(), nil
}

// get 发起 GET 并读取响应体，统一处理状态码与读取错误。
func (d *Drpy) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", drpyUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", endpoint, err)
	}
	defer resp.Body.Close()

	// 响应是 JSON，正常不会很大；加个上限防止异常源把内存打爆
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求 %s 返回状态码 %d（响应片段: %s）", endpoint, resp.StatusCode, snippet(body))
	}
	return body, nil
}

// isHTTPURL 判断是不是可以直接交给播放器的绝对链接。
func isHTTPURL(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// snippet 截断用于错误信息的原文，避免把整段 HTML/JSON 打进日志。
func snippet(body []byte) string {
	const max = 200
	s := strings.TrimSpace(string(body))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
