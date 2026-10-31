package source

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// videoExts 是 Feed 流里视为视频的扩展名。
var videoExts = map[string]struct{}{
	".mp4": {}, ".mkv": {}, ".avi": {}, ".mov": {}, ".m4v": {},
	".webm": {}, ".ts": {}, ".flv": {}, ".wmv": {}, ".m3u8": {},
}

// FeedItem 是 Feed 流的一集。
// CoverURL 直接取 alist 列表项自带的 thumb（网盘缩略图，带时效）；
// 这里刻意不调 fs/get，避免为每个文件都生成一次会过期的播放直链。
type FeedItem struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	CoverURL string `json:"cover_url"`
}

// Feed 列出目录下的视频文件，按路径升序返回。
// 如果选中的目录本身没有视频（例如根目录「启蒙」下面全是子文件夹），
// 会逐级向下收集各个子目录里的视频，汇总成一个 Feed。
//
// 配额分配以「根目录下的一级子目录」为单位：每个子目录单独收集成一桶，
// 各自有独立的扫描预算，最后各桶轮流取一条直到用满 maxFeedItems。
// 旧的深度优先写法会让排在前面的一个大目录把预算全吃掉，
// 后面的同级目录一条都进不来（前端再怎么随机也刷不到）。
func (a *Alist) Feed(ctx context.Context, dir, password string) ([]FeedItem, error) {
	entries, err := a.List(ctx, dir, password, 1, 0)
	if err != nil {
		return nil, err
	}

	buckets := make([][]FeedItem, 0, 8)
	rootVideos := make([]FeedItem, 0, len(entries))
	subdirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir {
			subdirs = append(subdirs, path.Join(dir, entry.Name))
			continue
		}
		if !isVideo(entry.Name) {
			continue
		}
		rootVideos = append(rootVideos, FeedItem{
			Name:     entry.Name,
			Path:     path.Join(dir, entry.Name),
			CoverURL: entry.Thumb,
		})
	}

	if len(rootVideos) > 0 {
		// 选中的目录本身就是一个视频合集：不再下钻
		buckets = append(buckets, rootVideos)
	} else {
		// 每个一级子目录的扫描预算按目录数平均分，避免某个目录把全程预算吃光
		dirBudget := maxFeedDirs / len(subdirs)
		if dirBudget < 4 {
			dirBudget = 4
		}
		for _, sub := range subdirs {
			videos, err := a.collectSubtree(ctx, sub, password, dirBudget)
			if err != nil {
				if ctx.Err() != nil {
					return nil, err
				}
				continue // 单个目录失败只跳过该目录，不拖垮整个 Feed
			}
			if len(videos) > 0 {
				buckets = append(buckets, videos)
			}
		}
	}

	// 各桶轮流取一条：第 i 轮取每个目录的第 i 条。预算被打断时，
	// 截断的只是各目录「靠后的部分」，不会有整个目录被饿死。
	items := make([]FeedItem, 0, 64)
	for i := 0; len(items) < maxFeedItems; i++ {
		added := false
		for _, bucket := range buckets {
			if i >= len(bucket) || len(items) >= maxFeedItems {
				continue
			}
			items = append(items, bucket[i])
			added = true
		}
		if !added {
			break
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
	})
	return items, nil
}

// collectSubtree 收集一个一级子目录下的视频。本目录已有视频就不再下钻，
// 保持「一个视频文件夹 = 一个合集」的语义；本目录没有视频则继续往子目录找。
// dirBudget 限制本次最多列多少个目录，避免选中超大目录树时请求过多。
func (a *Alist) collectSubtree(ctx context.Context, dir, password string, dirBudget int) ([]FeedItem, error) {
	videos := make([]FeedItem, 0, 64)
	scanned := 0

	var walk func(current string, depth int)
	walk = func(current string, depth int) {
		if scanned >= dirBudget || len(videos) >= maxFeedItems || depth > maxFeedDepth {
			return
		}
		if ctx.Err() != nil {
			return
		}
		scanned++

		entries, err := a.List(ctx, current, password, 1, 0)
		if err != nil {
			return // 子目录失败只跳过该分支
		}

		subdirs := make([]string, 0, len(entries))
		found := false
		for _, entry := range entries {
			if entry.IsDir {
				subdirs = append(subdirs, path.Join(current, entry.Name))
				continue
			}
			if !isVideo(entry.Name) {
				continue
			}
			found = true
			videos = append(videos, FeedItem{
				Name:     entry.Name,
				Path:     path.Join(current, entry.Name),
				CoverURL: entry.Thumb,
			})
		}

		if found {
			return
		}
		for _, sub := range subdirs {
			walk(sub, depth+1)
			if scanned >= dirBudget || len(videos) >= maxFeedItems {
				return
			}
		}
	}

	walk(dir, 1)
	return videos, ctx.Err()
}

// 递归上限：避免选到超大目录树时一次请求扫几万个文件。
const (
	maxFeedDepth = 4
	maxFeedItems = 500
	maxFeedDirs  = 60
)

// isVideo 判断文件名是否为视频。
func isVideo(name string) bool {
	_, ok := videoExts[strings.ToLower(path.Ext(name))]
	return ok
}

// VideoInfo 是播放时实时查询的结果，RawURL 有有效期，只应即取即用。
type VideoInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	RawURL   string `json:"raw_url"`
	CoverURL string `json:"cover_url"`
	Size     int64  `json:"size"`
}

// rawURLTTL 是直链缓存时长。直链本身的有效期远大于此，20 分钟足够覆盖一次连续播放
// （mpegts 对同一文件会发多次 Range 请求），刷新页面重播同一批视频时也能直接命中，
// 省掉每次都要等 fs/get 的那 1 秒多。链接提前失效时上游返回 403，会自动作废重取。
const rawURLTTL = 20 * time.Minute

// VideoURL 返回指定视频的直链。命中进程内缓存时直接返回，避免每个 Range 请求都调一次
// fs/get；缓存未命中或已过期时才向网盘取最新直链。
func (a *Alist) VideoURL(ctx context.Context, filePath, password string) (*VideoInfo, error) {
	a.urlMu.Lock()
	cached, ok := a.urlCache[filePath]
	a.urlMu.Unlock()
	if ok && time.Now().Before(cached.expire) {
		return cached.info, nil
	}

	file, err := a.Get(ctx, filePath, password, true)
	if err != nil {
		return nil, err
	}
	if file.IsDir {
		return nil, fmt.Errorf("%s 是目录，不是视频文件", filePath)
	}
	if file.RawURL == "" {
		return nil, fmt.Errorf("%s 未返回播放直链，可能是网盘驱动不支持或文件已失效", filePath)
	}

	info := &VideoInfo{
		Name:     file.Name,
		Path:     file.Path,
		RawURL:   file.RawURL,
		CoverURL: file.Thumb,
		Size:     file.Size,
	}
	a.urlMu.Lock()
	a.urlCache[filePath] = cachedVideoInfo{info: info, expire: time.Now().Add(rawURLTTL)}
	a.urlMu.Unlock()
	return info, nil
}

// InvalidateVideoURL 清掉某个文件的直链缓存，供上游返回 403 时刷新重试。
func (a *Alist) InvalidateVideoURL(filePath string) {
	a.urlMu.Lock()
	delete(a.urlCache, filePath)
	a.urlMu.Unlock()
}
