package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"qimeng-feed/internal/config"
	"qimeng-feed/internal/source"
)

// Handler 持有依赖，供各接口方法使用。
type Handler struct {
	alist *source.AlistProvider
	// store 负责读写 config.json。
	store *config.Store
	// sources 按 config.json 的最新内容提供默认源与各前缀对应的源，
	// 保存配置后立即生效，不需要重启进程。
	sources *source.Sources
}

// NewHandler 创建接口处理器。
func NewHandler(alist *source.AlistProvider, store *config.Store, sources *source.Sources) *Handler {
	return &Handler{alist: alist, store: store, sources: sources}
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

// Feed 处理 GET /api/feed：按配置返回 Feed 流。
// 有启用中的在线源时，把所有源勾选栏目的第一页合并后随机混排；
// 一个在线源都没有时，回退到网盘目录（path 默认用配置里的视频路径）。
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = h.store.GetAlist().BasePath
	}

	items, err := h.sources.MixedFeed(r.Context(), dir)
	if err != nil {
		slog.Error("生成 Feed 失败", "path", dir, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "生成 Feed 失败: "+err.Error())
		return
	}
	OK(w, items)
}

// FeedAll 处理 GET /api/feed/all?source_id=xxx&category_id=yyy：
// 返回某个在线源某个栏目下的全部视频（只有标题与封面）。
// 首页 /api/feed 仍只取第一页；合集面板按需调本接口。
func (h *Handler) FeedAll(w http.ResponseWriter, r *http.Request) {
	categoryID := strings.TrimSpace(r.URL.Query().Get("category_id"))
	if categoryID == "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "缺少 category_id 参数")
		return
	}

	o := h.onlineSourceOrFirst(r.URL.Query().Get("source_id"))
	if o == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "该视频对应的数据源未启用")
		return
	}

	items, err := o.AllFeedItems(r.Context(), categoryID)
	if err != nil {
		slog.Error("拉取栏目全部视频失败", "source_id", o.ID(), "category_id", categoryID, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "拉取栏目全部视频失败: "+err.Error())
		return
	}
	OK(w, items)
}

// onlineSourceOrFirst 按 ID 找在线源；没传 ID 时用第一个已启用的源。
func (h *Handler) onlineSourceOrFirst(sourceID string) *source.OnlineSource {
	if strings.TrimSpace(sourceID) != "" {
		return h.sources.Online(sourceID)
	}
	if list := h.sources.OnlineList(); len(list) > 0 {
		return list[0]
	}
	return nil
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
