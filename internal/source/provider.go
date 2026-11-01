package source

import (
	"context"
	"net/http"
	"strings"
)

// DrpyPathPrefix 是 drpy 条目在 FeedItem.Path 里的前缀。
//
// 前端只把 path 拼进 /api/video/stream?path=...，不认识数据源，
// 所以用前缀来区分「这个 path 属于 drpy 源、而不是网盘文件路径」，
// 这样接新数据源不需要改前端。
const DrpyPathPrefix = "drpy:"

// DrpyItemPath 把 vod_id 包装成 Feed 条目路径。
func DrpyItemPath(vodID string) string {
	return DrpyPathPrefix + strings.TrimSpace(vodID)
}

// IsDrpyPath 判断一个 Feed 条目路径是否属于 drpy 源。
func IsDrpyPath(itemPath string) bool {
	return strings.HasPrefix(itemPath, DrpyPathPrefix)
}

// ProviderOfPath 根据条目路径的前缀判断该由哪个数据源处理；
// 没有前缀的（如 /启蒙/xxx.mp4）一律按网盘文件处理。
func ProviderOfPath(itemPath string) string {
	switch {
	case strings.HasPrefix(itemPath, DrpyPathPrefix):
		return "drpy"
	case strings.HasPrefix(itemPath, TuxiaobeiPathPrefix):
		return "tuxiaobei"
	default:
		return "alist"
	}
}

// PlaySource 是解析出来的上游播放地址，连同取流时必需的请求头。
// 防盗链站点通常校验 Referer 和 User-Agent，这些头只在服务端代理时带，
// 不会透给浏览器。
type PlaySource struct {
	URL    string
	Header http.Header
}

// VideoProvider 是统一的数据源接口。三种源（网盘 alist、直连兔小贝、drpy-node）
// 都实现它，上层只依赖这个接口，不关心具体是哪种。
type VideoProvider interface {
	// Name 返回数据源标识，如 alist / drpy。
	Name() string
	// Feed 返回 Feed 列表。dir、password 只对网盘类源有意义，
	// 接口型源（drpy）会忽略它们，用自己的搜索关键词。
	Feed(ctx context.Context, dir, password string) ([]FeedItem, error)
	// Resolve 把 FeedItem.Path 解析成上游播放地址。
	Resolve(ctx context.Context, itemPath, password string) (*PlaySource, error)
}
