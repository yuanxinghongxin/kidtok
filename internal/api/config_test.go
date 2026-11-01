package api

import (
	"testing"

	"qimeng-feed/internal/config"
)

const testRule = `var rule = {
    title: '测试源',
    host: 'https://example.com',
    url: '/list?type=fc&page=fypage',
    detailUrl: '/play/fyid',
    class: [
        { type_id: '1', type_name: '儿歌' },
        { type_id: '2', type_name: '动画' }
    ]
}`

func TestNormalizeSettingsRejectsInvalidAlist(t *testing.T) {
	cases := []struct {
		name string
		in   config.Settings
		want string
	}{
		{
			name: "协议非法",
			in:   config.Settings{Alist: config.AlistSettings{Protocol: "ftp", Host: "192.168.0.201", Port: "5244"}},
			want: "网盘协议只能是 http 或 https",
		},
		{
			name: "地址带路径",
			in:   config.Settings{Alist: config.AlistSettings{Host: "192.168.0.201/alist", Port: "5244"}},
			want: "网盘地址只能是 IP 或域名，不要带路径",
		},
		{
			name: "端口非法",
			in:   config.Settings{Alist: config.AlistSettings{Host: "192.168.0.201", Port: "abc"}},
			want: "网盘端口必须是 1-65535 之间的数字",
		},
	}
	for _, tc := range cases {
		if _, msg := normalizeSettings(tc.in); msg != tc.want {
			t.Errorf("%s: 期望 %q，实际 %q", tc.name, tc.want, msg)
		}
	}
}

func TestNormalizeSettingsAllowsEmpty(t *testing.T) {
	if _, msg := normalizeSettings(config.Settings{}); msg != "" {
		t.Errorf("空配置应允许保存，实际 %q", msg)
	}
}

func TestNormalizeSettingsNormalizesAlist(t *testing.T) {
	in := config.Settings{
		Alist: config.AlistSettings{Protocol: "HTTP", Host: "http://192.168.0.201/", Port: " 5244 ", BasePath: "启蒙"},
	}
	out, msg := normalizeSettings(in)
	if msg != "" {
		t.Fatalf("不应报错，实际 %q", msg)
	}
	if out.Alist.Protocol != "http" {
		t.Errorf("protocol 应转小写，实际 %q", out.Alist.Protocol)
	}
	if out.Alist.Host != "192.168.0.201" {
		t.Errorf("host 应去掉协议与斜杠，实际 %q", out.Alist.Host)
	}
	if out.Alist.Port != "5244" {
		t.Errorf("port 应去掉空格，实际 %q", out.Alist.Port)
	}
	if out.Alist.BasePath != "/启蒙" {
		t.Errorf("base_path 应补前导斜杠，实际 %q", out.Alist.BasePath)
	}
}

func TestNormalizeSources(t *testing.T) {
	out, msg := normalizeSettings(config.Settings{
		Sources: []config.SourceSettings{
			{Kind: "DRPY", BaseURL: "192.168.0.201:5757/", SourceName: " 兔小贝[儿] ", Enabled: true, Categories: []string{"1", " 1 ", "", "2"}},
			{Kind: "js", Script: testRule, Enabled: true},
		},
	})
	if msg != "" {
		t.Fatalf("不应报错，实际 %q", msg)
	}
	if len(out.Sources) != 2 {
		t.Fatalf("应保留 2 个数据源，实际 %d", len(out.Sources))
	}

	drpy := out.Sources[0]
	if drpy.ID != "s1" {
		t.Errorf("空 ID 应生成 s1，实际 %q", drpy.ID)
	}
	if drpy.Kind != "drpy" || drpy.BaseURL != "http://192.168.0.201:5757" {
		t.Errorf("drpy 地址应补协议并去掉尾部斜杠，实际 %+v", drpy)
	}
	if drpy.Name != "兔小贝[儿]" {
		t.Errorf("源名应去掉首尾空格，实际 %q", drpy.Name)
	}
	if drpy.Pwd != "dzyyds" {
		t.Errorf("pwd 应有默认值，实际 %q", drpy.Pwd)
	}
	if len(drpy.Categories) != 2 {
		t.Errorf("栏目应去重去空，实际 %+v", drpy.Categories)
	}

	js := out.Sources[1]
	if js.ID != "s2" || js.Name != "测试源" {
		t.Errorf("JS 源应生成 ID 并用规则 title 兜底名称，实际 %+v", js)
	}
}

func TestNormalizeSourcesRejects(t *testing.T) {
	cases := []struct {
		name string
		in   config.SourceSettings
		want string
	}{
		{
			name: "类型非法",
			in:   config.SourceSettings{Kind: "alist"},
			want: "第 1 个数据源：数据源类型只能是 drpy 或 js",
		},
		{
			name: "启用的 drpy 缺地址",
			in:   config.SourceSettings{Kind: "drpy", SourceName: "兔小贝", Enabled: true},
			want: "第 1 个数据源：启用 drpy 数据源前请填写服务端地址",
		},
		{
			name: "启用的 drpy 缺源名",
			in:   config.SourceSettings{Kind: "drpy", BaseURL: "http://192.168.0.201:5757", Enabled: true},
			want: "第 1 个数据源：启用 drpy 数据源前请填写源名称",
		},
		{
			name: "启用的 js 缺规则",
			in:   config.SourceSettings{Kind: "js", Enabled: true},
			want: "第 1 个数据源：启用 JS 数据源前请上传或粘贴规则文件",
		},
		{
			name: "js 规则语法错误",
			in:   config.SourceSettings{Kind: "js", Script: `var rule = { title: 'x' }`},
			want: "第 1 个数据源：规则缺少 host",
		},
	}
	for _, tc := range cases {
		if _, msg := normalizeSettings(config.Settings{Sources: []config.SourceSettings{tc.in}}); msg != tc.want {
			t.Errorf("%s: 期望 %q，实际 %q", tc.name, tc.want, msg)
		}
	}
}

func TestNormalizeSourcesRejectsDuplicateID(t *testing.T) {
	_, msg := normalizeSettings(config.Settings{
		Sources: []config.SourceSettings{
			{ID: "a", Kind: "drpy"},
			{ID: "a", Kind: "js"},
		},
	})
	if msg == "" {
		t.Fatal("重复 ID 应该报错")
	}
}
