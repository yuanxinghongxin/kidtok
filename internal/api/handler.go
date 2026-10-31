package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"qimeng-feed/internal/config"
	"qimeng-feed/internal/source"
)

// Handler 持有依赖，供各接口方法使用。
type Handler struct {
	fetcher *source.Fetcher
	alist   *source.AlistProvider
	// store 负责读写 config.json。
	store *config.Store
}

// NewHandler 创建接口处理器。
func NewHandler(fetcher *source.Fetcher, alist *source.AlistProvider, store *config.Store) *Handler {
	return &Handler{fetcher: fetcher, alist: alist, store: store}
}

// alistClient 按最新配置返回客户端，未配置网盘时为 nil。
func (h *Handler) alistClient(ctx context.Context) *source.Alist {
	return h.alist.Client(ctx)
}

// RawConfig 处理 GET /api/config/raw：返回解密后的配置 JSON 字符串。
func (h *Handler) RawConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.fetcher.FetchConfig(r.Context())
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}
	OK(w, config)
}

// Sites 处理 GET /api/sites?keyword=启蒙：返回按关键词过滤后的站点列表。
func (h *Handler) Sites(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")

	config, err := h.fetcher.FetchConfig(r.Context())
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}

	sites, err := source.FilterSitesByKeyword(config, keyword)
	if err != nil {
		slog.Error("解析配置 JSON 失败", "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "上游配置解析失败: "+err.Error())
		return
	}
	OK(w, sites)
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

// Feed 处理 GET /api/feed?path=/xxx：返回短视频 Feed 流（name/path/cover_url）。
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	client := h.alistClient(r.Context())
	if client == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "未配置 alist 网盘源")
		return
	}

	dir := r.URL.Query().Get("path")
	if dir == "" {
		// 未显式传目录时直接取服务器配置里的视频路径
		dir = h.store.Get().BasePath
	}
	if dir == "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 path 参数")
		return
	}

	items, err := client.Feed(r.Context(), dir, r.URL.Query().Get("password"))
	if err != nil {
		slog.Error("生成 Feed 失败", "path", dir, "err", err)
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

func (h *Handler) writeUpstreamError(w http.ResponseWriter, err error) {
	if errors.Is(err, source.ErrSourceURLMissing) {
		slog.Warn("上游接口地址未配置", "err", err)
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, err.Error())
		return
	}
	slog.Error("拉取上游配置失败", "err", err)
	Fail(w, http.StatusBadGateway, CodeUpstreamErr, "拉取上游配置失败: "+err.Error())
}
