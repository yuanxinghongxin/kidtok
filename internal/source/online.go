package source

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"qimeng-feed/internal/config"
)

// OnlineSource 是一条已启用的在线数据源：drpy-node 转发或本机执行的 JS 规则。
// 两种源都归一到同样的「栏目 / 列表 / 全部集数 / 解析直链」四个动作，
// 上层聚合时不需要关心它是哪种类型。
type OnlineSource struct {
	cfg    config.SourceSettings
	drpy   *DrpySource
	script *ScriptSource
}

// newOnlineSource 按配置创建在线源实例。
func newOnlineSource(cfg config.SourceSettings, timeout time.Duration) (*OnlineSource, error) {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	switch cfg.Kind {
	case "drpy":
		d := NewDrpySource(cfg.BaseURL, cfg.SourceName, cfg.Pwd, "", cfg.Categories, timeout)
		d.SetSourceID(cfg.ID)
		return &OnlineSource{cfg: cfg, drpy: d}, nil
	case "js":
		s, err := NewScriptSource(cfg.Script, timeout)
		if err != nil {
			return nil, err
		}
		s.SetSourceID(cfg.ID)
		return &OnlineSource{cfg: cfg, script: s}, nil
	default:
		return nil, fmt.Errorf("未知的数据源类型 %q", cfg.Kind)
	}
}

// ID 返回配置里的源 ID。
func (o *OnlineSource) ID() string { return o.cfg.ID }

// Name 返回给用户看的源名（例如「兔小贝」）。
func (o *OnlineSource) Name() string {
	if n := strings.TrimSpace(o.cfg.Name); n != "" {
		return n
	}
	return strings.TrimSpace(o.cfg.SourceName)
}

// Categories 返回该源的全部栏目。
func (o *OnlineSource) Categories(ctx context.Context) ([]DrpyCategory, error) {
	if o.drpy != nil {
		return o.drpy.Categories(ctx)
	}
	return o.script.Categories(ctx)
}

// Feed 拉取该源勾选栏目的第一页，并给每条视频补上源信息。
func (o *OnlineSource) Feed(ctx context.Context) ([]FeedItem, error) {
	var (
		items []FeedItem
		err   error
	)
	if o.drpy != nil {
		items, err = o.drpy.FeedCategories(ctx, o.cfg.Categories)
	} else {
		items, err = o.script.FeedCategories(ctx, o.cfg.Categories)
	}
	if err != nil {
		return nil, err
	}
	name := o.Name()
	for i := range items {
		items[i].SourceID = o.cfg.ID
		items[i].SourceName = name
		// 前端合集名显示「源名 · 栏目名」，不同源的同名栏目也分得开。
		if items[i].Category != "" {
			items[i].Category = name + " · " + items[i].Category
		}
	}
	return items, nil
}

// AllFeedItems 拉取某个栏目的全部视频（只有标题与封面，不含播放直链）。
func (o *OnlineSource) AllFeedItems(ctx context.Context, categoryID string) ([]FeedItem, error) {
	var (
		items []FeedItem
		err   error
	)
	if o.drpy != nil {
		items, err = o.drpy.AllFeedItems(ctx, categoryID)
	} else {
		items, err = o.script.AllFeedItems(ctx, categoryID)
	}
	if err != nil {
		return nil, err
	}
	name := o.Name()
	for i := range items {
		items[i].SourceID = o.cfg.ID
		items[i].SourceName = name
		if items[i].Category != "" {
			items[i].Category = name + " · " + items[i].Category
		}
	}
	return items, nil
}

// Resolve 把条目键解析成可播放的上游地址。
func (o *OnlineSource) Resolve(ctx context.Context, itemKey string) (*PlaySource, error) {
	if o.drpy != nil {
		return o.drpy.Resolve(ctx, DrpyItemPath(itemKey), "")
	}
	return o.script.Resolve(ctx, itemKey)
}

// Invalidate 丢掉某个条目的直链缓存，供上游 403 时刷新重试。
func (o *OnlineSource) Invalidate(itemKey string) {
	if o.drpy != nil {
		o.drpy.Invalidate(context.Background(), DrpyItemPath(itemKey))
		return
	}
	o.script.Invalidate(itemKey)
}

// ProbeOnlineSource 用一份还没保存的源配置连接一次并返回栏目，
// 供设置页点「检测」时立刻验证地址/源名/JS 规则是否可用。
func ProbeOnlineSource(ctx context.Context, cfg config.SourceSettings, timeout time.Duration) ([]DrpyCategory, error) {
	cfg.ID = "probe"
	o, err := newOnlineSource(cfg, timeout)
	if err != nil {
		return nil, err
	}
	return o.Categories(ctx)
}

// onlineRouter 实现 VideoProvider：按路径里的源 ID 找对应的在线源解析直链。
// 前端只认识 path，不关心它是 drpy 还是上传的 JS。
type onlineRouter struct {
	sources *Sources
}

// Name 实现 VideoProvider。
func (r *onlineRouter) Name() string { return "online" }

// Feed 实现 VideoProvider：返回所有已启用源的随机混合流。
func (r *onlineRouter) Feed(ctx context.Context, dir, password string) ([]FeedItem, error) {
	return r.sources.MixedFeed(ctx, dir)
}

// Resolve 实现 VideoProvider：从 src:<源ID>:<条目键> 里拆出源并解析。
func (r *onlineRouter) Resolve(ctx context.Context, itemPath, password string) (*PlaySource, error) {
	sourceID, key, ok := SplitOnlinePath(itemPath)
	if !ok {
		// 旧版单源路径 drpy:<vod_id>：交给第一个启用的源，保证老数据还能播。
		if strings.HasPrefix(itemPath, DrpyPathPrefix) {
			sourceID = r.sources.firstOnlineID()
			key = strings.TrimPrefix(itemPath, DrpyPathPrefix)
		} else {
			return nil, errors.New("不是在线数据源路径")
		}
	}
	if sourceID == "" {
		return nil, errors.New("在线数据源路径缺少源 ID")
	}
	o := r.sources.onlineByID(sourceID)
	if o == nil {
		return nil, fmt.Errorf("数据源 %s 不存在或未启用", sourceID)
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("在线条目缺少视频 ID")
	}
	return o.Resolve(ctx, key)
}
