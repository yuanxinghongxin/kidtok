// Package config 负责加载服务配置：内置默认值 ← config.yaml ← 环境变量。
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是服务的全部运行时配置。
type Config struct {
	Server Server
	Source Source
	Alist  Alist
}

// Server 是 HTTP 服务相关配置。
type Server struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

// Source 是上游影视源（饭太硬）相关配置。
type Source struct {
	FantaiyingURL string
	UserAgent     string
	Timeout       time.Duration
}

// Alist 是自建 alist 网盘服务相关配置。
type Alist struct {
	BaseURL  string
	Username string
	Password string
	Token    string
	// RootPath 是前端默认打开的视频目录，可在配置页里修改。
	RootPath string
	Timeout  time.Duration
}

// Default 返回内置默认配置。
func Default() *Config {
	return &Config{
		Server: Server{
			Addr:              ":8080",
			ReadHeaderTimeout: 10 * time.Second,
			ShutdownTimeout:   5 * time.Second,
		},
		Source: Source{
			UserAgent: "okhttp/3.15",
			Timeout:   15 * time.Second,
		},
		Alist: Alist{
			Timeout: 15 * time.Second,
		},
	}
}

// Load 读取配置文件（不存在则跳过），再叠加环境变量。
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		kv, perr := parseYAML(data)
		if perr != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, perr)
		}
		if err := cfg.applyFile(kv); err != nil {
			return nil, fmt.Errorf("配置文件 %s 取值非法: %w", path, err)
		}
	case os.IsNotExist(err):
		// 允许只靠环境变量运行。
	default:
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

	if err := cfg.applyEnv(); err != nil {
		return nil, fmt.Errorf("环境变量取值非法: %w", err)
	}
	return cfg, nil
}

// applyFile 把 "a.b" 形式的扁平键值写入配置。
func (c *Config) applyFile(kv map[string]string) error {
	for key, val := range kv {
		switch key {
		case "server.addr":
			c.Server.Addr = val
		case "server.read_header_timeout":
			d, err := parseDuration(val)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			c.Server.ReadHeaderTimeout = d
		case "server.shutdown_timeout":
			d, err := parseDuration(val)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			c.Server.ShutdownTimeout = d
		case "source.fantaiying_url":
			c.Source.FantaiyingURL = val
		case "source.user_agent":
			c.Source.UserAgent = val
		case "source.timeout":
			d, err := parseDuration(val)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			c.Source.Timeout = d
		case "alist.base_url":
			c.Alist.BaseURL = val
		case "alist.username":
			c.Alist.Username = val
		case "alist.password":
			c.Alist.Password = val
		case "alist.token":
			c.Alist.Token = val
		case "alist.root_path":
			c.Alist.RootPath = val
		case "alist.timeout":
			d, err := parseDuration(val)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			c.Alist.Timeout = d
		}
	}
	return nil
}

// applyEnv 用 QIMENG_ 前缀的环境变量覆盖配置。
func (c *Config) applyEnv() error {
	if v := os.Getenv("QIMENG_SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("QIMENG_FANTAIYING_URL"); v != "" {
		c.Source.FantaiyingURL = v
	}
	if v := os.Getenv("QIMENG_USER_AGENT"); v != "" {
		c.Source.UserAgent = v
	}
	if v := os.Getenv("QIMENG_SOURCE_TIMEOUT"); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return fmt.Errorf("QIMENG_SOURCE_TIMEOUT: %w", err)
		}
		c.Source.Timeout = d
	}
	if v := os.Getenv("QIMENG_ALIST_URL"); v != "" {
		c.Alist.BaseURL = v
	}
	if v := os.Getenv("QIMENG_ALIST_USERNAME"); v != "" {
		c.Alist.Username = v
	}
	if v := os.Getenv("QIMENG_ALIST_PASSWORD"); v != "" {
		c.Alist.Password = v
	}
	if v := os.Getenv("QIMENG_ALIST_TOKEN"); v != "" {
		c.Alist.Token = v
	}
	if v := os.Getenv("QIMENG_ALIST_TIMEOUT"); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return fmt.Errorf("QIMENG_ALIST_TIMEOUT: %w", err)
		}
		c.Alist.Timeout = d
	}
	return nil
}

// parseDuration 支持 "15s" 与纯数字（按秒理解）两种写法。
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("空的时间值")
	}
	if secs, err := strconv.Atoi(s); err == nil {
		if secs <= 0 {
			return 0, fmt.Errorf("时间值必须为正数: %q", s)
		}
		return time.Duration(secs) * time.Second, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("无法解析时间值 %q", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("时间值必须为正数: %q", s)
	}
	return d, nil
}

// parseYAML 解析本配置用到的两级 YAML 子集，输出 "server.addr" 形式的键值。
// 只支持 key: value、两级缩进（2 空格）、整行 # 注释，足够覆盖 config.yaml 的结构。
func parseYAML(data []byte) (map[string]string, error) {
	kv := make(map[string]string)
	var parents []string

	sc := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()

		if strings.ContainsRune(line, '\t') {
			return nil, fmt.Errorf("第 %d 行使用了制表符，请改用空格缩进", lineNo)
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent%2 != 0 {
			return nil, fmt.Errorf("第 %d 行缩进不是 2 的倍数", lineNo)
		}
		level := indent / 2
		if level > len(parents) {
			return nil, fmt.Errorf("第 %d 行缩进层级跳跃", lineNo)
		}
		parents = parents[:level]

		idx := strings.Index(trimmed, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("第 %d 行不是合法的 key: value 形式", lineNo)
		}
		key := strings.TrimSpace(trimmed[:idx])
		value := strings.TrimSpace(trimmed[idx+1:])
		if value == "" {
			parents = append(parents, key)
			continue
		}
		kv[strings.Join(append(parents, key), ".")] = strings.Trim(value, `"'`)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return kv, nil
}
