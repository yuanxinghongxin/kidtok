package api

import (
	"context"
	"log/slog"
	"net/http"

	"qimeng-feed/internal/config"
	"qimeng-feed/internal/source"
)

// Handler 持有依赖，供各接口方法使用。
type Handler struct {
	alist *source.AlistProvider
	// store 负责读写 config.json。
	store *config.Store
	// primary 是 /api/feed 默认使用的数据源（由配置 source.primary 决定）。
	primary source.VideoProvider
	// providers 按名字保存所有可用数据源，取流时按路径前缀再选一次。
	providers map[string]source.VideoProvider
}

// NewHandler 创建接口处理器。primary 为 nil 表示没有可用数据源。
func NewHandler(alist *source.AlistProvider, store *config.Store, primary source.VideoProvider, providers map[string]source.VideoProvider) *Handler {
	return &Handler{alist: alist, store: store, primary: primary, providers: providers}
}

// alistClient 按最新配置返回客户端，未配置网盘时为 nil。
func (h *Handler) alistClient(ctx context.Context) *source.Alist {
	return h.alist.Client(ctx)
}

// AlistList 处理 GET /api/alist/list?path=/&password=：列出网盘目录。
func (h *Handler) AlistList(w http.ResponseWriter, r *http.Request) {
	client := h.alistClient(r.Context())
	if client == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "未配置 alist 网盘源")
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	entries, err := client.List(r.Context(), path, r.URL.Query().Get("password"), 1, 0)
	if err != nil {
		slog.Error("列出 alist 目录失败", "path", path, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "列出 alist 目录失败: "+err.Error())
		return
	}
	OK(w, entries)
}

// AlistPlay 处理 GET /api/alist/play?path=<文件路径>：返回含直链的播放信息。
// 直链有时效，每次播放前都应调用本接口重新获取。
func (h *Handler) AlistPlay(w http.ResponseWriter, r *http.Request) {
	client := h.alistClient(r.Context())
	if client == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "未配置 alist 网盘源")
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 path 参数")
		return
	}

	file, err := client.Get(r.Context(), path, r.URL.Query().Get("password"), true)
	if err != nil {
		slog.Error("获取 alist 播放直链失败", "path", path, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "获取播放直链失败: "+err.Error())
		return
	}
	OK(w, file)
}

// Feed 处理 GET /api/feed?path=/xxx：按配置的数据源返回短视频 Feed 流。
// path 对网盘源是目录，对 drpy 源可临时充当搜索关键词；不传时网盘源用服务器配置的视频路径。
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	if h.primary == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "未配置任何视频源")
		return
	}

	dir := r.URL.Query().Get("path")
	if dir == "" && h.primary.Name() == "alist" {
		dir = h.store.Get().BasePath
	}
	if dir == "" && h.primary.Name() == "alist" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 path 参数")
		return
	}

	items, err := h.primary.Feed(r.Context(), dir, r.URL.Query().Get("password"))
	if err != nil {
		slog.Error("生成 Feed 失败", "source", h.primary.Name(), "path", dir, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "生成 Feed 失败: "+err.Error())
		return
	}
	OK(w, items)
}

// VideoURL 处理 GET /api/video/url?path=<视频路径>：实时返回最新播放直链。
// 前端每滑到一个视频就调一次，不要缓存 RawURL。
func (h *Handler) VideoURL(w http.ResponseWriter, r *http.Request) {
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

	info, err := client.VideoURL(r.Context(), filePath, r.URL.Query().Get("password"))
	if err != nil {
		slog.Error("获取视频直链失败", "path", filePath, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "获取视频直链失败: "+err.Error())
		return
	}
	OK(w, info)
}

// NotFound 兜底路由，保证未匹配路径也返回统一 JSON。
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	Fail(w, http.StatusNotFound, CodeBadRequest, "接口不存在")
}
