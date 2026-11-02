package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// DrpyEpisode 是解析出来后的一集：名称 + 链接。
// 嗅探型源（一级链接）给的往往是一个网页地址，需要用 Flag 调 play 接口换出真实媒体链接。
type DrpyEpisode struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Flag string `json:"flag,omitempty"`
}

// drpyListResp 是 ?ac=search / ?ac=list 的响应外层。
// 不同源可能省略 code/msg，也可能带 page/total，这里只取需要的字段。
type drpyListResp struct {
	Code  int         `json:"code"`
	Msg   string      `json:"msg"`
	Total int         `json:"total"`
	// 部分源只给总页数和每页条数，此时用 pagecount*limit 还原总数。
	PageCount int         `json:"pagecount"`
	Limit     int         `json:"limit"`
	List      []DrpyVideo `json:"list"`
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

	return normalizeVideos(resp.List), nil
}

// DrpyCategory 是源支持的一个栏目（对应规则里的 class_name / class_url）。
type DrpyCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// flexString 兼容源把字段写成字符串或数字两种情况：
// 兔小贝的 type_id 是 "2"，贝乐虎的 type_id 是 65，直接按 string 解析会整份失败。
type flexString string

func (s *flexString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var v string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*s = flexString(n.String())
	return nil
}

// drpyClassResp 是 ?ac=home / ?ac=list 的分类列表外层。
type drpyClassResp struct {
	Class []struct {
		TypeID   flexString `json:"type_id"`
		TypeName string     `json:"type_name"`
	} `json:"class"`
	Msg string `json:"msg"`
}

// Categories 获取当前源支持的全部栏目。
// drpy-node 在 ?ac=home（部分源只有 ?ac=list）的响应里用 class 数组给出
// {type_id, type_name}，也就是规则里的 class_url / class_name。
func (d *Drpy) Categories(ctx context.Context) ([]DrpyCategory, error) {
	var lastErr error
	for _, ac := range []string{"home", "list"} {
		endpoint, err := d.endpoint(ac, nil)
		if err != nil {
			return nil, err
		}
		body, err := d.get(ctx, endpoint)
		if err != nil {
			lastErr = err
			continue
		}
		var resp drpyClassResp
		if err := json.Unmarshal(body, &resp); err != nil {
			lastErr = fmt.Errorf("解析栏目响应失败: %w（响应片段: %s）", err, snippet(body))
			continue
		}

		cats := make([]DrpyCategory, 0, len(resp.Class))
		for _, c := range resp.Class {
			id := strings.TrimSpace(string(c.TypeID))
			if id == "" {
				continue
			}
			name := strings.TrimSpace(c.TypeName)
			if name == "" {
				name = id
			}
			cats = append(cats, DrpyCategory{ID: id, Name: name})
		}
		if len(cats) > 0 {
			return cats, nil
		}
		lastErr = fmt.Errorf("drpy 源没有返回栏目（ac=%s）", ac)
	}
	if lastErr == nil {
		lastErr = errors.New("drpy 源没有返回栏目")
	}
	return nil, lastErr
}

// CategoryVideos 拉取某个栏目的第一页数据（?ac=category&t=栏目ID）。
func (d *Drpy) CategoryVideos(ctx context.Context, categoryID string) ([]DrpyVideo, int, error) {
	return d.CategoryVideosPage(ctx, categoryID, 1)
}

// drpyMaxAllPages 是「拉全栏目」的页数上限（每页约 30 条，上限即 1800 条），
// 防止个别源谎报超大 pagecount 时无限请求。
const drpyMaxAllPages = 60

// AllCategoryVideos 从第 1 页开始逐页拉取，直到源返回空页（部分源用 no_data 占位）为止。
// 合集面板要展示该栏目的全部视频，所以这里不做条数裁剪，只按 vod_id 去重。
func (d *Drpy) AllCategoryVideos(ctx context.Context, categoryID string) ([]DrpyVideo, error) {
	all := make([]DrpyVideo, 0, 60)
	seen := make(map[string]bool)

	for page := 1; page <= drpyMaxAllPages; page++ {
		videos, _, err := d.CategoryVideosPage(ctx, categoryID, page)
		if err != nil {
			// 第一页就失败说明栏目不可用；后续页失败时把已拿到的部分返回，
			// 让面板仍然能展示内容（比如第 20 页偶发超时）。
			if page == 1 {
				return nil, err
			}
			break
		}
		added := 0
		for _, v := range videos {
			if seen[v.VodID] {
				continue
			}
			seen[v.VodID] = true
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
	return all, nil
}

// CategoryVideosPage 拉取栏目第 page 页，并返回源声明的真实总条数。
// drpy 通常返回 total；只给 pagecount/limit 的源用二者相乘还原。
func (d *Drpy) CategoryVideosPage(ctx context.Context, categoryID string, page int) ([]DrpyVideo, int, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, 0, errors.New("栏目 ID 不能为空")
	}

	params := url.Values{"t": {categoryID}}
	if page > 1 {
		params.Set("pg", strconv.Itoa(page))
	}

	endpoint, err := d.endpoint("category", params)
	if err != nil {
		return nil, 0, err
	}
	body, err := d.get(ctx, endpoint)
	if err != nil {
		return nil, 0, err
	}

	var resp drpyListResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("解析栏目响应失败: %w（响应片段: %s）", err, snippet(body))
	}
	if resp.Code != 0 && resp.Code != 1 && resp.Msg != "" {
		return nil, 0, fmt.Errorf("栏目接口返回错误（code=%d）: %s", resp.Code, resp.Msg)
	}

	total := resp.Total
	if total <= 0 && resp.PageCount > 0 && resp.Limit > 0 {
		total = resp.PageCount * resp.Limit
	}
	return normalizeVideos(resp.List), total, nil
}

// normalizeVideos 去掉脏数据并 trim 字段，搜索与栏目接口共用。
func normalizeVideos(list []DrpyVideo) []DrpyVideo {
	videos := make([]DrpyVideo, 0, len(list))
	for _, v := range list {
		if strings.TrimSpace(v.VodID) == "" || strings.TrimSpace(v.VodName) == "" {
			continue // 脏数据直接跳过，不让它污染后续详情请求
		}
		// drpy 到底后用 no_data + 「无数据,防无限请求」占位，不能当成一集展示
		if v.VodID == "no_data" || strings.Contains(v.VodName, "防无限请求") {
			continue
		}
		v.VodID = strings.TrimSpace(v.VodID)
		v.VodName = strings.TrimSpace(v.VodName)
		videos = append(videos, v)
	}
	return videos
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

	episodes := ParseVodPlayURLWithFrom(resp.List[0].VodPlayURL, resp.List[0].VodPlayFrom)
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
	return ParseVodPlayURLWithFrom(raw, "")
}

// ParseVodPlayURLWithFrom 在解析剧集的同时，按 $$$ 线路顺序把 vod_play_from 写进 Flag。
// 嗅探型源播放时要带 flag，否则 drpy-node 不知道用哪条线路去解析网页。
func ParseVodPlayURLWithFrom(raw, playFrom string) []DrpyEpisode {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	flags := strings.Split(playFrom, "$$$")
	lines := strings.Split(raw, "$$$")
	episodes := make([]DrpyEpisode, 0, 32)
	for lineIdx, line := range lines {
		flag := ""
		if lineIdx < len(flags) {
			flag = strings.TrimSpace(flags[lineIdx])
		}
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
			episodes = append(episodes, DrpyEpisode{Name: name, URL: link, Flag: flag})
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
	if ac != "" {
		params.Set("ac", ac)
	}
	if d.Pwd != "" {
		params.Set("pwd", d.Pwd)
	}
	return base + "/api/" + url.PathEscape(sourceName) + "?" + params.Encode(), nil
}

// PlayResult 是 drpy play 动作的解析结果。
type PlayResult struct {
	// URL 是最终可播放的媒体地址（mp4/m3u8），个别源也可能是仍需嗅探的网页。
	URL    string
	Header http.Header
	// Parse 为 1 表示该地址还需要客户端 JS 嗅探才能出流。
	Parse int
}

// drpyPlayResp 是 ?play=xxx&flag=yyy 的响应外层。
type drpyPlayResp struct {
	Code   int             `json:"code"`
	Msg    string          `json:"msg"`
	Parse  int             `json:"parse"`
	URL    string          `json:"url"`
	Js     string          `json:"js"`
	Data   string          `json:"data"`
	Header json.RawMessage `json:"header"`
}

// PlayMedia 调用 drpy 源的播放动作，把详情里的网页地址换成真正的媒体直链。
//
// 一级/嗅探型源的 vod_play_url 给的是详情页（例如 https://www.tuxiaobei.com/play/1404），
// 直接丢给播放器只会拿到 HTML。drpy-node 的 ?play=<地址>&flag=<线路> 会跑源里的 play 逻辑，
// 返回 {"parse":0,"url":"https://.../xxx.mp4"}；部分源返回 302 跳转到 /mediaProxy。
// 这两种情况都归一到「可被本服务代理的 URL」，仍由 /api/video/stream 统一转发。
func (d *Drpy) PlayMedia(ctx context.Context, playURL, flag string) (*PlayResult, error) {
	if !isHTTPURL(playURL) {
		return nil, fmt.Errorf("play 地址不是合法 http(s) 链接: %s", playURL)
	}
	params := url.Values{"play": {playURL}}
	if strings.TrimSpace(flag) != "" {
		params.Set("flag", strings.TrimSpace(flag))
	}
	endpoint, err := d.endpoint("", params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造 play 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", drpyUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	// 有些源会 302 到 /mediaProxy：这里不跟随，直接拿 Location 交给后续代理。
	client := &http.Client{
		Timeout:       d.client.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 play 接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, fmt.Errorf("play 接口返回 %d 但缺少 Location", resp.StatusCode)
		}
		abs, err := resolveRef(endpoint, loc)
		if err != nil {
			return nil, fmt.Errorf("play 接口跳转地址不合法（%s）: %w", loc, err)
		}
		return &PlayResult{URL: abs}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 play 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("play 接口返回状态码 %d（响应片段: %s）", resp.StatusCode, snippet(body))
	}

	// 响应本身就是媒体流（未走 JSON 的源）：play 地址自己就是可持续拉流的入口。
	mediaType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(string(body), "#EXTM3U") {
		return &PlayResult{URL: endpoint}, nil
	}

	var pr drpyPlayResp
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("play 接口返回不是 JSON: %w（响应片段: %s）", err, snippet(body))
	}
	finalURL := strings.TrimSpace(pr.URL)
	if finalURL == "" && strings.TrimSpace(pr.Data) != "" {
		// 少数源把结果再套一层 JSON 字符串放在 data 里。
		var inner struct {
			URL    string          `json:"url"`
			Header json.RawMessage `json:"header"`
		}
		if json.Unmarshal([]byte(pr.Data), &inner) == nil {
			finalURL = strings.TrimSpace(inner.URL)
			if pr.Header == nil {
				pr.Header = inner.Header
			}
		} else if isHTTPURL(strings.TrimSpace(pr.Data)) {
			finalURL = strings.TrimSpace(pr.Data)
		}
	}
	if !isHTTPURL(finalURL) {
		return nil, fmt.Errorf("play 接口没有返回可播放地址（响应片段: %s）", snippet(body))
	}
	return &PlayResult{URL: finalURL, Header: parseDrpyHeader(pr.Header), Parse: pr.Parse}, nil
}

// parseDrpyHeader 解析源返回的 header 字段（可能是对象，也可能是 JSON 字符串）。
func parseDrpyHeader(raw json.RawMessage) http.Header {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		var s string
		if json.Unmarshal(raw, &s) != nil || json.Unmarshal([]byte(s), &m) != nil {
			return nil
		}
	}
	if len(m) == 0 {
		return nil
	}
	h := http.Header{}
	for k, v := range m {
		h.Set(k, v)
	}
	return h
}

// resolveRef 把 Location 里的相对地址补成绝对地址。
func resolveRef(baseURL, ref string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(r).String(), nil
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

// isMediaLink 判断链接本身是否已经是媒体文件（不用再走 play 接口嗅探）。
func isMediaLink(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	path := strings.ToLower(u.Path)
	for _, ext := range []string{".m3u8", ".mp4", ".m4v", ".ts", ".flv", ".mkv", ".webm", ".mov", ".mp3", ".m4a"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
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
