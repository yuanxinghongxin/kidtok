package api

import (
	"testing"

	"qimeng-feed/internal/config"
)

func TestNormalizeSettingsRejectsIncompletePrimary(t *testing.T) {
	cases := []struct {
		name string
		in   config.Settings
		want string
	}{
		{
			name: "alist 没填地址",
			in:   config.Settings{PrimarySource: "alist"},
			want: "选择 AList 作为默认数据源时，请填写网盘地址",
		},
		{
			name: "drpy 没填地址",
			in:   config.Settings{PrimarySource: "drpy", Drpy: config.DrpySettings{Name: "兔小贝[儿]"}},
			want: "选择 Drpy 作为默认数据源时，请填写服务端地址",
		},
		{
			name: "drpy 没填源名",
			in:   config.Settings{PrimarySource: "drpy", Drpy: config.DrpySettings{BaseURL: "http://192.168.0.201:5757"}},
			want: "选择 Drpy 作为默认数据源时，请填写源名称",
		},
		{
			name: "默认源非法",
			in:   config.Settings{PrimarySource: "local"},
			want: "默认数据源只能是 alist 或 drpy",
		},
	}
	for _, tc := range cases {
		if _, msg := normalizeSettings(tc.in); msg != tc.want {
			t.Errorf("%s: 期望 %q，实际 %q", tc.name, tc.want, msg)
		}
	}
}

func TestNormalizeSettingsNormalizesFields(t *testing.T) {
	in := config.Settings{
		PrimarySource: "DRPY",
		Alist:        config.AlistSettings{Host: "http://192.168.0.201/", Port: "5244", BasePath: "启蒙"},
		Drpy:         config.DrpySettings{BaseURL: "192.168.0.201:5757/", Name: " 兔小贝[儿] "},
	}
	out, msg := normalizeSettings(in)
	if msg != "" {
		t.Fatalf("不应报错，实际 %q", msg)
	}
	if out.PrimarySource != "drpy" {
		t.Errorf("primary_source 应转小写，实际 %q", out.PrimarySource)
	}
	if out.Alist.Host != "192.168.0.201" {
		t.Errorf("host 应去掉协议与斜杠，实际 %q", out.Alist.Host)
	}
	if out.Alist.BasePath != "/启蒙" {
		t.Errorf("base_path 应补前导斜杠，实际 %q", out.Alist.BasePath)
	}
	if out.Drpy.BaseURL != "http://192.168.0.201:5757" {
		t.Errorf("drpy 地址应补协议并去掉尾部斜杠，实际 %q", out.Drpy.BaseURL)
	}
	if out.Drpy.Name != "兔小贝[儿]" {
		t.Errorf("源名应去掉首尾空格，实际 %q", out.Drpy.Name)
	}
}
