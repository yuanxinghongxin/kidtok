package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Alist 是 alist 的 Web API 客户端（v3.x）。
type Alist struct {
	baseURL string
	token   string
	client  *http.Client

	// 直链缓存：mpegts 对同一个视频会发多次 Range 请求，命中缓存可省掉每次 fs/get 的几百毫秒。
	urlMu    sync.Mutex
	urlCache map[string]cachedVideoInfo
}

// cachedVideoInfo 是带过期时间的直链缓存项。
type cachedVideoInfo struct {
	info   *VideoInfo
	expire time.Time
}

// NewAlist 创建客户端；token 可为空，之后用 Login 获取。
func NewAlist(baseURL, token string, timeout time.Duration) *Alist {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Alist{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:    strings.TrimSpace(token),
		client:   &http.Client{Timeout: timeout},
		urlCache: map[string]cachedVideoInfo{},
	}
}

// AlistEntry 是 /api/fs/list 返回的目录项。
type AlistEntry struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified string `json:"modified"`
	Thumb    string `json:"thumb"`
	Type     int    `json:"type"`
}

// AlistFile 是 /api/fs/get 返回的文件信息，RawURL 有有效期，用完即弃。
type AlistFile struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	IsDir  bool   `json:"is_dir"`
	RawURL string `json:"raw_url"`
	Thumb  string `json:"thumb"`
	Sign   string `json:"sign"`
}

// Login 调用 /api/auth/login 换取 token。
func (a *Alist) Login(ctx context.Context, username, password string) (string, error) {
	var data struct {
		Token string `json:"token"`
	}
	payload := map[string]string{"username": username, "password": password}
	if err := a.post(ctx, "/api/auth/login", payload, &data); err != nil {
		return "", err
	}
	if data.Token == "" {
		return "", fmt.Errorf("alist 登录成功但未返回 token")
	}
	return data.Token, nil
}

// SetToken 设置后续请求使用的 token。
func (a *Alist) SetToken(token string) {
	a.token = strings.TrimSpace(token)
}

// List 列出目录内容。perPage 为 0 时由 alist 使用默认值。
// password 是 alist 目录级访问密码，未设置时传空字符串。
func (a *Alist) List(ctx context.Context, path, password string, page, perPage int) ([]AlistEntry, error) {
	if page <= 0 {
		page = 1
	}
	payload := map[string]any{
		"path":     path,
		"password": password,
		"page":     page,
		"per_page": perPage,
		"refresh":  false,
	}
	var data struct {
		Content []AlistEntry `json:"content"`
		Total   int          `json:"total"`
	}
	if err := a.post(ctx, "/api/fs/list", payload, &data); err != nil {
		return nil, err
	}
	return data.Content, nil
}

// Get 获取单个路径的文件信息（含播放直链）。password 是目录级访问密码；
// refresh 为 true 时强制 alist 重新生成直链，避免拿到缓存里已过期的 RawURL。
func (a *Alist) Get(ctx context.Context, path, password string, refresh bool) (*AlistFile, error) {
	var file AlistFile
	payload := map[string]any{"path": path, "password": password, "refresh": refresh}
	if err := a.post(ctx, "/api/fs/get", payload, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

// post 发送 JSON 请求并解析 alist 统一信封 {code,message,data}。
func (a *Alist) post(ctx context.Context, apiPath string, payload any, out any) error {
	if a.baseURL == "" {
		return fmt.Errorf("未配置 alist 地址")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+apiPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求 alist 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 alist 响应失败: %w", err)
	}

	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("解析 alist 响应失败(HTTP %d): %.120s", resp.StatusCode, raw)
	}
	if envelope.Code != http.StatusOK {
		return fmt.Errorf("alist 返回 code=%d: %s", envelope.Code, envelope.Message)
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("解析 alist data 失败: %w", err)
	}
	return nil
}
