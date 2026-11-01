package source

import (
	"context"
	"net/http"
	"strings"
)

// DrpyPathPrefix 是旧版单源 drpy 条目在 FeedItem.Path 里的前缀。
// 新版多数据源用 OnlinePathPrefix，并把源 ID 也编进路径里。
const DrpyPathPrefix = "drpy:"

// OnlinePathPrefix 是「在线数据源」条目在 FeedItem.Path 里的前缀。
// 格式：src:<源ID>:<条目键>，例如 src:s1:/play/1404。
const OnlinePathPrefix = "src:"

// DrpyItemPath 把 vod_id 包装成旧版单源 drpy 路径（保留给旧配置的兼容逻辑）。
func DrpyItemPath(vodID string) string {
	return DrpyPathPrefix + strings.TrimSpace(vodID)
}

// OnlineItemPath 把某个数据源的条目键包装成路径。
// 源 ID 里只允许字母数字与下划线，因此用第一个冒号切分即可，
// 后面的冒号都归属条目键（部分源的 vod_id 里带冒号或斜杠）。
func OnlineItemPath(sourceID, key string) string {
	return OnlinePathPrefix + sourceID + ":" + strings.TrimSpace(key)
}

// SplitOnlinePath 拆出源 ID 与条目键；不是在线源路径时 ok 为 false。
func SplitOnlinePath(itemPath string) (sourceID, key string, ok bool) {
	if !strings.HasPrefix(itemPath, OnlinePathPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(itemPath, OnlinePathPrefix)
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return rest, "", true
	}
	return rest[:idx], rest[idx+1:], true
}

// IsDrpyPath 判断一个 Feed 条目路径是否属于旧版 drpy 源。
func IsDrpyPath(itemPath string) bool {
	return strings.HasPrefix(itemPath, DrpyPathPrefix)
}

// ProviderOfPath 根据条目路径的前缀判断该由哪个数据源处理；
// 没有前缀的（如 /启蒙/xxx.mp4）一律按网盘文件处理。
func ProviderOfPath(itemPath string) string {
	switch {
	case strings.HasPrefix(itemPath, OnlinePathPrefix), strings.HasPrefix(itemPath, DrpyPathPrefix):
		return "online"
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
