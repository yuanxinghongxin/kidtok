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

// Settings 是前端「服务器配置」页编辑的网盘挂载参数，持久化在 config.json。
type Settings struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
}

// BaseURL 拼成 http://host:port；未填 Host 时返回空串。
func (s Settings) BaseURL() string {
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

// SeedFromAlist 把 config.yaml 里的 alist 段转成初始配置，仅在 config.json 不存在时使用。
func SeedFromAlist(a Alist) Settings {
	s := Settings{
		Protocol: "http",
		Username: a.Username,
		Password: a.Password,
		BasePath: a.RootPath,
	}
	u, err := url.Parse(strings.TrimSpace(a.BaseURL))
	if err != nil || u.Host == "" {
		return s
	}
	if strings.ToLower(u.Scheme) == "https" {
		s.Protocol = "https"
	}
	s.Host = u.Hostname()
	s.Port = u.Port()
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
		s.cfg = got
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
			s.cfg = got
		}
	}
	return s.cfg
}

// Save 把配置写入 config.json。
func (s *Store) Save(cfg Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
