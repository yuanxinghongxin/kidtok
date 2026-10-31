// Package source 负责对接上游影视源，并解析其返回的加密配置。
package source

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultUserAgent 是请求饭太硬接口时必须带的 User-Agent。
	DefaultUserAgent = "okhttp/3.15"
	// separator 是图片字节流与 Base64 载荷的分隔符。
	separator = "**"
)

// ErrSourceURLMissing 表示没有配置上游接口地址。
var ErrSourceURLMissing = errors.New("未配置饭太硬接口地址（config.yaml 的 source.fantaiying_url 或环境变量 QIMENG_FANTAIYING_URL）")

// Fetcher 拉取并解密饭太硬配置。
type Fetcher struct {
	url       string
	userAgent string
	client    *http.Client
}

// NewFetcher 创建 Fetcher；userAgent 为空时使用 okhttp/3.15。
func NewFetcher(url, userAgent string, timeout time.Duration) *Fetcher {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = DefaultUserAgent
	}
	return &Fetcher{
		url:       strings.TrimSpace(url),
		userAgent: userAgent,
		client:    &http.Client{Timeout: timeout},
	}
}

// FetchConfig 请求上游接口，返回解密后的 JSON 字符串。
func (f *Fetcher) FetchConfig(ctx context.Context) (string, error) {
	if f.url == "" {
		return "", ErrSourceURLMissing
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return "", fmt.Errorf("构造上游请求失败: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求上游接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("上游接口返回状态码 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return DecodeConfig(resp.Body)
}

// DecodeConfig 从饭太硬响应中解出 JSON：
// 响应体是「图片字节流 + ** + Base64(JSON)」，取最后一个 ** 之后的内容做 Base64 解码。
func DecodeConfig(reader io.Reader) (string, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取上游响应失败: %w", err)
	}

	idx := bytes.LastIndex(raw, []byte(separator))
	if idx < 0 {
		return "", fmt.Errorf("响应中没有找到分隔符 %q（长度为 %d 字节）", separator, len(raw))
	}

	payload := string(raw[idx+len(separator):])
	payload = strings.TrimSpace(payload)
	payload = strings.TrimRight(payload, "*") // 兼容尾部多余的 ** 标记
	// Base64 载荷可能被换行/空格折行，统一去掉所有空白字符。
	payload = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, payload)
	if payload == "" {
		return "", errors.New("分隔符之后没有 Base64 内容")
	}

	decoded, err := decodeBase64(payload)
	if err != nil {
		return "", err
	}

	out := strings.TrimPrefix(strings.TrimSpace(string(decoded)), "\ufeff")
	// 上游配置里会用 // 注释掉部分条目，标准 JSON 解析器不认识，因此校验前先剥离。
	if !json.Valid([]byte(stripLineComments(out))) {
		return "", errors.New("Base64 解码结果不是合法的 JSON")
	}
	return out, nil
}

// stripLineComments 丢弃整行以 // 开头的注释行，与 TVBox 对配置的宽松解析保持一致。
// 只按行首判断，不会破坏值里的 "https://..." 这类内容。
func stripLineComments(s string) string {
	if !strings.Contains(s, "//") {
		return s
	}
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// decodeBase64 依次尝试标准/无填充/URL 安全编码，兼容上游填充差异。
func decodeBase64(s string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(s)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("Base64 解码失败: %w", lastErr)
}
