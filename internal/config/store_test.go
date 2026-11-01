package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 旧版 config.json 是扁平的（protocol/host/... 直接在最外层），读取时要迁移到 alist 段。
func TestLegacySettingsUnmarshal(t *testing.T) {
	legacy := []byte(`{"protocol":"http","host":"192.168.0.201","port":"5244","username":"admin","password":"123456","base_path":"/启蒙"}`)
	var got Settings
	if err := json.Unmarshal(legacy, &got); err != nil {
		t.Fatalf("解析旧配置失败: %v", err)
	}
	if got.PrimarySource != "alist" {
		t.Errorf("旧配置默认源应为 alist，实际 %q", got.PrimarySource)
	}
	if got.Alist.Host != "192.168.0.201" || got.Alist.Port != "5244" || got.Alist.BasePath != "/启蒙" {
		t.Errorf("旧配置字段迁移不完整: %+v", got.Alist)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	seed := Settings{
		PrimarySource: "drpy",
		Alist:         AlistSettings{Protocol: "http", Host: "192.168.0.201", Port: "5244", BasePath: "/启蒙"},
		Drpy:          DrpySettings{BaseURL: "http://192.168.0.201:5757", Name: "兔小贝[儿]", Pwd: "dzyyds", Keyword: "儿歌"},
	}
	store, err := NewStore(path, seed)
	if err != nil {
		t.Fatalf("创建 Store 失败: %v", err)
	}

	got := store.Get()
	if got.PrimarySource != "drpy" || got.Drpy.Name != "兔小贝[儿]" || got.Alist.Host != "192.168.0.201" {
		t.Errorf("首次写入的配置与 seed 不一致: %+v", got)
	}

	next := seed
	next.PrimarySource = "alist"
	next.Drpy.Keyword = "动画"
	if err := store.Save(next); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || !json.Valid(data) {
		t.Errorf("落盘内容不是合法 JSON: %v", err)
	}
	if got = store.Get(); got.PrimarySource != "alist" || got.Drpy.Keyword != "动画" {
		t.Errorf("保存后读取不一致: %+v", got)
	}
}
