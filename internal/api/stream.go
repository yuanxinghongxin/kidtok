package api

import (
	"context"
	"fmt"
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

// VideoStream 处理 GET /api/video/stream?path=<条目路径>：把上游直链以同源流的形式转发。
//
// 浏览器直连第三方直链会遇到两类问题：跨域（CORS）与防盗链（403）。
// 由本服务代理后视频与页面同源，同时 Referer / User-Agent 由服务端补齐，
// 这些头不会暴露给浏览器。
func (h *Handler) VideoStream(w http.ResponseWriter, r *http.Request) {
	itemPath := r.URL.Query().Get("path")
	if itemPath == "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 path 参数")
		return
	}

	provider := h.providerForPath(itemPath)
	if provider == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "对应的视频源未配置")
		return
	}

	src, err := provider.Resolve(r.Context(), itemPath, r.URL.Query().Get("password"))
	if err != nil {
		slog.Error("解析播放地址失败", "source", provider.Name(), "path", itemPath, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "解析播放地址失败: "+err.Error())
		return
	}

	rangeHeader := r.Header.Get("Range")
	resp, err := openUpstream(r.Context(), src, rangeHeader)
	if err != nil {
		slog.Error("请求上游直链失败", "source", provider.Name(), "path", itemPath, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "请求上游直链失败: "+err.Error())
		return
	}

	// 直链可能在上游提前作废（403）：丢掉缓存重取一次再试。
	if resp.StatusCode == http.StatusForbidden {
		slog.Warn("上游直链返回 403，刷新后重试", "path", itemPath)
		resp.Body.Close()
		if inv, ok := provider.(interface {
			Invalidate(context.Context, string)
		}); ok {
			inv.Invalidate(r.Context(), itemPath)
		}
		if src, err = provider.Resolve(r.Context(), itemPath, r.URL.Query().Get("password")); err != nil {
			Fail(w, http.StatusBadGateway, CodeUpstreamErr, "刷新播放地址失败: "+err.Error())
			return
		}
		if resp, err = openUpstream(r.Context(), src, rangeHeader); err != nil {
			Fail(w, http.StatusBadGateway, CodeUpstreamErr, "刷新后请求直链失败: "+err.Error())
			return
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		slog.Error("上游直链返回错误状态", "path", itemPath, "status", resp.StatusCode)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "上游直链返回状态码 "+strconv.Itoa(resp.StatusCode))
		return
	}

	// m3u8：把播放列表里的分片地址改写成走本服务代理，否则播放器会直连第三方域名再次被拦。
	if isPlaylist(src.URL, resp.Header) {
		h.writePlaylist(w, resp, src.URL, src.Header.Get("Referer"))
		return
	}
	copyMedia(w, resp)
}

// providerForPath 按条目路径挑选数据源：带前缀的走对应接口型源，其余按网盘文件处理。
func (h *Handler) providerForPath(itemPath string) source.VideoProvider {
	return h.providers[source.ProviderOfPath(itemPath)]
}

// openUpstream 按解析结果发起一次上游请求，带上源要求的防盗链请求头并透传 Range。
func openUpstream(ctx context.Context, src *source.PlaySource, rangeHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造上游请求失败: %w", err)
	}
	for key, values := range src.Header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	// 透传 Range，保证 mpegts 拉流和进度拖动都能按需取字节。
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	return streamClient.Do(req)
}

// copyMedia 把上游字节流透传给前端，并保留支持断点续传所需的响应头。
func copyMedia(w http.ResponseWriter, resp *http.Response) {
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
		slog.Warn("视频流转发中断", "err", err)
	}
}
