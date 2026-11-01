package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Settings 是前端「数据源配置」页编辑的全部内容，持久化在 config.json。
//
//	{
//	  "alist": { "protocol": "http", "host": "...", ... },
//	  "sources": [
//	    {"id": "s1", "kind": "drpy", "name": "兔小贝", "enabled": true,
//	     "categories": ["2"], "base_url": "...", "source_name": "兔小贝[儿]", "pwd": "dzyyds"},
//	    {"id": "s2", "kind": "js", "name": "贝乐虎", "enabled": true,
//	     "categories": ["65"], "script": "var rule = {...}"}
//	  ]
//	}
type Settings struct {
	// PrimarySource 是旧版单源配置的默认源标记，仅用于把旧配置迁移成 Sources。
	PrimarySource string `json:"primary_source,omitempty"`
	// EnabledCategories 是旧版单源配置勾选的栏目，仅用于迁移。
	EnabledCategories []string `json:"enabled_categories,omitempty"`
	Alist             AlistSettings `json:"alist"`
	// Drpy 是旧版单源配置，仅用于迁移；新配置请使用 Sources。
	Drpy DrpySettings `json:"drpy,omitempty"`
	// Sources 是可以保存多个的在线数据源（drpy-node 或上传的 JS 规则），
	// 首页会把所有「已启用」源里勾选的栏目合并后随机播放。
	Sources []SourceSettings `json:"sources"`
}

// SourceSettings 是一个已保存的在线数据源。
type SourceSettings struct {
	ID   string `json:"id"`
	// Kind 是数据源类型：drpy（本地 drpy-node）或 js（上传/粘贴的 drpy 规则文件）。
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Categories 是勾选参与首页混排的栏目 ID；为空时用源返回的第一个栏目。
	Categories []string `json:"categories"`

	// drpy-node 参数。
	BaseURL    string `json:"base_url,omitempty"`
	SourceName string `json:"source_name,omitempty"`
	Pwd        string `json:"pwd,omitempty"`

	// JS 规则源参数：直接保存规则文件内容，避免部署多个挂载目录。
	Script     string `json:"script,omitempty"`
	ScriptName string `json:"script_name,omitempty"`
}

// Configured 判断数据源参数是否完整。
func (s SourceSettings) Configured() bool {
	switch s.Kind {
	case "drpy":
		return strings.TrimSpace(s.BaseURL) != "" && strings.TrimSpace(s.SourceName) != ""
	case "js":
		return strings.TrimSpace(s.Script) != ""
	}
	return false
}

// AlistSettings 是网盘挂载参数。
type AlistSettings struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
	// Disabled 为 true 时网盘不参与首页混排（在线源仍然正常工作）。
	// 用「是否禁用」而不是「是否启用」，是为了让旧配置（没有这个字段）默认参与。
	Disabled bool `json:"disabled,omitempty"`
}

// Included 判断网盘是否参与首页混排：填了视频路径且没有被禁用。
func (s AlistSettings) Included() bool {
	return !s.Disabled && strings.TrimSpace(s.BasePath) != ""
}

// DrpySettings 是本地 drpy-node 服务参数。
type DrpySettings struct {
	// BaseURL 形如 http://192.168.0.201:5757，不带 /api。
	BaseURL string `json:"base_url"`
	Name    string `json:"name"`
	Pwd     string `json:"pwd"`
	Keyword string `json:"keyword"`
}

// BaseURL 拼成 http://host:port；未填 Host 时返回空串。
func (s AlistSettings) BaseURL() string {
	host := strings.TrimSpace(s.Host)
	if host == "" {
		return ""
	}
	protocol := strings.ToLower(strings.TrimSpace(s.Protocol))
	if protocol != "https" {
		protocol = "http"
	}
	port := strings.TrimSpace(s.Port)
	if port == "" {
		if protocol == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return protocol + "://" + host + ":" + port
}

// Configured 判断网盘是否填了地址。
func (s AlistSettings) Configured() bool {
	return strings.TrimSpace(s.Host) != ""
}

// Configured 判断 drpy 源是否配置完整（地址与源名都填了才能用）。
func (d DrpySettings) Configured() bool {
	return strings.TrimSpace(d.BaseURL) != "" && strings.TrimSpace(d.Name) != ""
}

// UnmarshalJSON 兼容旧版扁平 config.json（protocol/host/... 直接在最外层）。
func (s *Settings) UnmarshalJSON(data []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if _, ok := probe["alist"]; ok {
		type plain Settings
		var out plain
		if err := json.Unmarshal(data, &out); err != nil {
			return err
		}
		*s = Settings(out)
		return nil
	}
	var legacy AlistSettings
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*s = Settings{PrimarySource: "alist", Alist: legacy}
	return nil
}

// Seed 把 config.yaml 里的 alist / source 段转成初始配置，仅在 config.json 不存在时使用。
func Seed(a Alist, src Source) Settings {
	s := Settings{
		PrimarySource: strings.ToLower(strings.TrimSpace(src.Primary)),
		Alist: AlistSettings{
			Protocol: "http",
			Username: a.Username,
			Password: a.Password,
			BasePath: a.RootPath,
		},
		Drpy: DrpySettings{
			BaseURL: src.Drpy.BaseURL,
			Name:    src.Drpy.Name,
			Pwd:     src.Drpy.Pwd,
			Keyword: src.Drpy.Keyword,
		},
	}
	if s.PrimarySource == "" {
		s.PrimarySource = "alist"
	}

	if u, err := url.Parse(strings.TrimSpace(a.BaseURL)); err == nil && u.Host != "" {
		if strings.ToLower(u.Scheme) == "https" {
			s.Alist.Protocol = "https"
		}
		s.Alist.Host = u.Hostname()
		s.Alist.Port = u.Port()
	}
	return s
}

// Store 负责 config.json 的读写，可被多个请求并发使用。
type Store struct {
	mu   sync.Mutex
	path string
	cfg  Settings
}

// NewStore 读取 config.json；文件不存在时写入 seed（可为空配置）。
func NewStore(path string, seed Settings) (*Store, error) {
	s := &Store{path: path, cfg: seed}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var got Settings
		if err := json.Unmarshal(data, &got); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		s.cfg = withDefaults(got)
	case os.IsNotExist(err):
		if err := s.write(seed); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	return s, nil
}

// Get 每次都读文件，保证手工改了 config.json 也能立刻生效。
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if data, err := os.ReadFile(s.path); err == nil {
		var got Settings
		if json.Unmarshal(data, &got) == nil {
			s.cfg = withDefaults(got)
		}
	}
	return s.cfg
}

// GetAlist 只取网盘配置，供 AlistProvider 高频调用。
func (s *Store) GetAlist() AlistSettings {
	return s.Get().Alist
}

// Save 把配置写入 config.json。
func (s *Store) Save(cfg Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg = withDefaults(cfg)
	if err := s.write(cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

func (s *Store) write(cfg Settings) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	if err := os.WriteFile(s.path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", s.path, err)
	}
	return nil
}

// withDefaults 补齐缺省值：没写 primary_source 时按网盘；
// 旧版配置只有单个 drpy 时，自动迁移成一条 Sources 记录。
func withDefaults(s Settings) Settings {
	if s.PrimarySource == "" {
		s.PrimarySource = "alist"
	}
	if s.EnabledCategories == nil {
		s.EnabledCategories = []string{}
	}
	if s.Sources == nil {
		s.Sources = migrateSources(s)
	}
	return s
}

// migrateSources 把旧版单源 drpy 配置转成新版多数据源列表。
func migrateSources(s Settings) []SourceSettings {
	out := []SourceSettings{}
	if !s.Drpy.Configured() {
		return out
	}
	out = append(out, SourceSettings{
		ID:         "drpy",
		Kind:       "drpy",
		Name:       strings.TrimSpace(s.Drpy.Name),
		Enabled:    strings.EqualFold(strings.TrimSpace(s.PrimarySource), "drpy"),
		Categories: s.EnabledCategories,
		BaseURL:    strings.TrimSpace(s.Drpy.BaseURL),
		SourceName: strings.TrimSpace(s.Drpy.Name),
		Pwd:        strings.TrimSpace(s.Drpy.Pwd),
	})
	return out
}
