package api

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"qimeng-feed/internal/source"
)

// collectionCacheTTL 是合集全量列表的本地缓存有效期：12 小时内同一栏目直接读 JSON 文件。
const collectionCacheTTL = 12 * time.Hour

// collectionFetchTimeout 是后台预热单个栏目的整体预算。
const collectionFetchTimeout = 90 * time.Second

// collectionPrefetchWorkers 是同时预热的分组上限，避免一次打满上游。
const collectionPrefetchWorkers = 6

// collectionCacheEntry 是落盘结构。只存列表基础信息（标题/封面/栏目），
// 不存任何播放直链：直链有时效，每次播放仍走 Resolve 重新解析。
type collectionCacheEntry struct {
	// Key 是 source_id|category_id：文件名是哈希，启动时靠它把磁盘缓存重新索引回内存
	Key       string            `json:"key"`
	UpdatedAt time.Time         `json:"updated_at"`
	Items     []source.FeedItem `json:"items"`
}

// CollectionCache 是「栏目全量列表」的本地 JSON 缓存 + 后台预热器。
//
// 并发安全：
//   - inflight 用 sync.Map 记录正在拉取的 key，同一栏目绝不重复请求；
//   - 落盘用「临时文件 + rename」原子替换，多个栏目同时写也不会读到半个文件；
//   - valid 记录最近一次成功写入的时间，预热前先查内存，避免每次 /api/feed 都读磁盘。
type CollectionCache struct {
	dir string
	ttl time.Duration

	mu    sync.RWMutex
	valid map[string]time.Time
	// inflight 的 value 是 chan struct{}：关闭表示这次拉取结束（成功或失败）
	inflight sync.Map
	// sem 限制同时预热的分组数量：首页一次可能带上几十个栏目，
	// 全放出去会瞬间打满上游，反而拖慢紧接着的首页请求。
	sem chan struct{}
}

// NewCollectionCache 创建缓存器；dir 为空时禁用落盘（只保留预热去重）。
func NewCollectionCache(dir string, ttl time.Duration) *CollectionCache {
	if ttl <= 0 {
		ttl = collectionCacheTTL
	}
	c := &CollectionCache{
		dir:   dir,
		ttl:   ttl,
		valid: make(map[string]time.Time),
		sem:   make(chan struct{}, collectionPrefetchWorkers),
	}
	// 进程启动时把磁盘上仍在有效期内的缓存重新索引：
	// 重启后第一次点合集也能直接命中，不用再等一轮上游。
	c.loadIndex()
	return c
}

// loadIndex 扫描缓存目录，把有效期内条目的 key 灌回内存索引。
// 单个文件损坏只会导致该栏目重新拉取，不影响其它缓存。
func (c *CollectionCache) loadIndex() {
	if c.dir == "" {
		return
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, file := range entries {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.dir, file.Name()))
		if err != nil {
			continue
		}
		var meta collectionCacheEntry
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.Key == "" || time.Since(meta.UpdatedAt) >= c.ttl {
			continue
		}
		c.valid[meta.Key] = meta.UpdatedAt
	}
}

// collectionKey 由 source_id + category_id 组成，避免不同源的同名栏目串味。
func collectionKey(sourceID, categoryID string) string {
	return sourceID + "|" + categoryID
}

// filePath 用 sha1 生成文件名：category_id 可能带中文/斜杠，不能直接拼进路径。
func (c *CollectionCache) filePath(key string) string {
	sum := sha1.Sum([]byte(key))
	return filepath.Join(c.dir, "collection_"+hex.EncodeToString(sum[:])+".json")
}

func (c *CollectionCache) fresh(key string) bool {
	c.mu.RLock()
	at, ok := c.valid[key]
	c.mu.RUnlock()
	return ok && time.Since(at) < c.ttl
}

// Load 优先命中内存有效期，再读 JSON 文件；过期、损坏、不存在都算未命中。
func (c *CollectionCache) Load(key string) ([]source.FeedItem, bool) {
	if !c.fresh(key) {
		return nil, false
	}
	if c.dir == "" {
		return nil, false
	}

	data, err := os.ReadFile(c.filePath(key))
	if err != nil {
		c.drop(key)
		return nil, false
	}
	var entry collectionCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil || len(entry.Items) == 0 {
		c.drop(key)
		return nil, false
	}
	if time.Since(entry.UpdatedAt) >= c.ttl {
		c.drop(key)
		return nil, false
	}
	return entry.Items, true
}

// Save 原子落盘：先写同目录临时文件，再 rename 覆盖，读端永远看到完整 JSON。
func (c *CollectionCache) Save(key string, items []source.FeedItem) error {
	if c.dir == "" || len(items) == 0 {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}

	entry := collectionCacheEntry{Key: key, UpdatedAt: time.Now(), Items: items}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// CreateTemp 保证并发写同一栏目时临时文件名也不会撞车
	tmp, err := os.CreateTemp(c.dir, "collection-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后会删不到，但失败路径能清掉残留

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, c.filePath(key)); err != nil {
		return err
	}

	c.mu.Lock()
	c.valid[key] = entry.UpdatedAt
	c.mu.Unlock()
	return nil
}

func (c *CollectionCache) drop(key string) {
	c.mu.Lock()
	delete(c.valid, key)
	c.mu.Unlock()
}

// Warm 在后台异步预热某个栏目：已缓存或正在预热时直接返回，不重复打上游。
func (c *CollectionCache) Warm(key string, fetch func(context.Context) ([]source.FeedItem, error)) {
	if c.fresh(key) {
		return
	}
	v, loaded := c.inflight.LoadOrStore(key, make(chan struct{}))
	if loaded {
		return
	}

	go func() {
		ch := v.(chan struct{})
		defer func() {
			c.inflight.Delete(key)
			close(ch)
		}()

		// 全局限流：排队等一个空位，最多等一轮预热周期
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
		case <-time.After(collectionFetchTimeout):
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), collectionFetchTimeout)
		defer cancel()

		items, err := fetch(ctx)
		if err != nil {
			slog.Warn("预热合集失败", "key", key, "err", err)
			return
		}
		if err := c.Save(key, items); err != nil {
			slog.Warn("写入合集缓存失败", "key", key, "err", err)
		}
	}()
}

// Get 是 /api/feed/all 的入口：命中缓存直接返回；未命中时与后台预热共用同一次请求，
// 避免用户点得快时「预热一次 + 接口再拉一次」。
func (c *CollectionCache) Get(ctx context.Context, key string, fetch func(context.Context) ([]source.FeedItem, error)) ([]source.FeedItem, error) {
	if items, ok := c.Load(key); ok {
		return items, nil
	}

	if v, loaded := c.inflight.LoadOrStore(key, make(chan struct{})); loaded {
		ch := v.(chan struct{})
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if items, ok := c.Load(key); ok {
			return items, nil
		}
	} else {
		ch := v.(chan struct{})
		defer func() {
			c.inflight.Delete(key)
			close(ch)
		}()
	}

	items, err := fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("栏目没有返回任何视频")
	}
	if err := c.Save(key, items); err != nil {
		slog.Warn("写入合集缓存失败", "key", key, "err", err)
	}
	return items, nil
}
