package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"qimeng-feed/internal/source"
)

// streamClient 专门用于视频流转发：不设总超时（大文件要读很久），只限制响应头等待时间。
var streamClient = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   8,
	},
}

// 访问网盘直链必须使用非浏览器 UA：百度 dlink 对浏览器 UA 直接返回 403，
// 只有客户端类 UA（如 okhttp）才会放行（实测 Chrome UA 一律 403）。

// VideoStream 处理 GET /api/video/stream?path=<视频路径>：把网盘直链以同源流的形式转发。
// 浏览器直连 baidupcs.com 会被 Chrome 的 ORB（Opaque Response Blocking）拦截，
// 由本服务代理后视频与页面同源，同时规避跨域与防盗链。
func (h *Handler) VideoStream(w http.ResponseWriter, r *http.Request) {
	client := h.alistClient(r.Context())
	if client == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "未配置 alist 网盘源")
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 path 参数")
		return
	}
	password := r.URL.Query().Get("password")
	rangeHeader := r.Header.Get("Range")

	resp, err := h.openVideoStream(client, r.Context(), filePath, password, rangeHeader)
	if err != nil {
		slog.Error("请求网盘直链失败", "path", filePath, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "请求网盘直链失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	// 缓存中的直链可能在有效期内被网盘提前作废（403）：丢掉缓存，用最新直链重试一次。
	if resp.StatusCode == http.StatusForbidden {
		slog.Warn("直链返回 403，刷新后重试", "path", filePath)
		resp.Body.Close()
		client.InvalidateVideoURL(filePath)
		if resp, err = h.openVideoStream(client, r.Context(), filePath, password, rangeHeader); err != nil {
			slog.Error("刷新直链后仍失败", "path", filePath, "err", err)
			Fail(w, http.StatusBadGateway, CodeUpstreamErr, "请求网盘直链失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode >= http.StatusBadRequest {
		slog.Error("网盘直链返回错误状态", "path", filePath, "status", resp.StatusCode)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "网盘直链返回状态码 "+strconv.Itoa(resp.StatusCode))
		return
	}

	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "video/mp4")
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("视频流转发中断", "path", filePath, "err", err)
	}
}

// openVideoStream 取直链（优先走缓存）并按客户端 Range 发起一次上游请求。
func (h *Handler) openVideoStream(client *source.Alist, ctx context.Context, filePath, password, rangeHeader string) (*http.Response, error) {
	info, err := client.VideoURL(ctx, filePath, password)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.RawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", source.DefaultUserAgent)
	// 透传 Range，保证 mpegts 拉流和进度拖动都能按需取字节。
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	return streamClient.Do(req)
}
