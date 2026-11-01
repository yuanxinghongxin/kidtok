package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"qimeng-feed/internal/source"
)

// mediaUA 是代理第三方直链时使用的 UA：多数站点只对浏览器 UA 放行。
const mediaUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"

// Proxy 处理 GET /api/proxy?url=<直链>&referer=<可选>：通用透传代理。
//
// 服务端拉流再转发，Referer / User-Agent 由服务端补齐，浏览器只看到同源地址，
// 既绕开 CORS 也绕开防盗链。m3u8 会把分片地址改写成继续走本代理的地址，
// 保证上层播放列表里的相对路径不会让播放器又去直连第三方域名。
func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 url 参数")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "url 必须是 http/https 绝对地址")
		return
	}

	referer := r.URL.Query().Get("referer")
	header := http.Header{}
	header.Set("User-Agent", mediaUA)
	header.Set("Accept", "*/*")
	if referer != "" {
		header.Set("Referer", referer)
	}

	resp, err := openUpstream(r.Context(), &source.PlaySource{URL: raw, Header: header}, r.Header.Get("Range"))
	if err != nil {
		slog.Error("代理直链请求失败", "url", raw, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "代理直链失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		slog.Error("代理直链返回错误状态", "url", raw, "status", resp.StatusCode)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "代理直链返回状态码 "+resp.Status)
		return
	}

	if isPlaylist(raw, resp.Header) {
		h.writePlaylist(w, resp, raw, referer)
		return
	}
	copyMedia(w, resp)
}

// isPlaylist 判断响应是不是 m3u8 播放列表（按 Content-Type 或地址后缀）。
func isPlaylist(rawURL string, header http.Header) bool {
	ct := strings.ToLower(header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "application/x-mpeg") {
		return true
	}
	if u, err := url.Parse(rawURL); err == nil {
		return strings.HasSuffix(strings.ToLower(u.Path), ".m3u8")
	}
	return false
}

// uriAttr 匹配 EXT-X-KEY / EXT-X-MAP / EXT-X-MEDIA 里的 URI="..."。
var uriAttr = regexp.MustCompile(`URI="([^"]+)"`)

// writePlaylist 读取 m3u8 内容，把分片、密钥、子列表的地址全部改写成本服务代理地址。
func (h *Handler) writePlaylist(w http.ResponseWriter, resp *http.Response, baseURL, referer string) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "读取播放列表失败: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(rewritePlaylist(string(body), baseURL, referer))); err != nil {
		slog.Warn("写回播放列表失败", "err", err)
	}
}

// rewritePlaylist 把播放列表里的地址改写为 /api/proxy?url=...，相对路径按 m3u8 自身地址解析。
func rewritePlaylist(text, baseURL, referer string) string {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return text
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			// 标签行：只改写 URI="..." 属性（密钥、初始化分片、嵌套列表）
			lines[i] = uriAttr.ReplaceAllStringFunc(line, func(m string) string {
				sub := uriAttr.FindStringSubmatch(m)[1]
				abs := resolveRef(base, sub)
				if abs == "" {
					return m
				}
				return `URI="` + proxyPath(abs, referer) + `"`
			})
			continue
		}
		abs := resolveRef(base, trimmed)
		if abs == "" {
			continue
		}
		lines[i] = proxyPath(abs, referer)
	}
	return strings.Join(lines, "\n")
}

// resolveRef 把可能是相对路径的分片地址解析成绝对地址，只放行 http/https。
func resolveRef(base *url.URL, ref string) string {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ""
	}
	abs := base.ResolveReference(u)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}

// proxyPath 拼出本服务的代理地址，referer 一并带上，分片请求也能过防盗链。
func proxyPath(absURL, referer string) string {
	p := "/api/proxy?url=" + url.QueryEscape(absURL)
	if referer != "" {
		p += "&referer=" + url.QueryEscape(referer)
	}
	return p
}
