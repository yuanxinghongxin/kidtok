package api

import (
	"net/http"
	"strings"
)

// NewRouter 注册接口路由，并把其余路径交给嵌入的前端静态文件。
func NewRouter(h *Handler, static http.FileSystem) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", h.Config)
	mux.HandleFunc("POST /api/config", h.SaveConfig)
	mux.HandleFunc("GET /api/categories", h.Categories)
	mux.HandleFunc("POST /api/source/test", h.TestSource)
	mux.HandleFunc("GET /api/alist/list", h.AlistList)
	mux.HandleFunc("GET /api/alist/play", h.AlistPlay)
	mux.HandleFunc("GET /api/feed", h.Feed)
	mux.HandleFunc("GET /api/feed/all", h.FeedAll)
	mux.HandleFunc("GET /api/video/url", h.VideoURL)
	mux.HandleFunc("GET /api/video/stream", h.VideoStream)
	mux.HandleFunc("GET /api/proxy", h.Proxy)

	fileServer := http.FileServer(static)
	// manifest.json 用规范的 PWA MIME 类型；提前写入 Content-Type 后
	// http.FileServer 不会再按扩展名改成 application/json。
	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		fileServer.ServeHTTP(w, r)
	})
	// 未注册的 /api 路径仍返回统一 JSON，其余路径交给静态文件服务。
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	}))
	return mux
}
