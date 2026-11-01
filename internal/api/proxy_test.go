package api

import (
	"net/url"
	"strings"
	"testing"
)

func TestRewritePlaylist(t *testing.T) {
	const (
		base    = "https://cdn.example.com/hls/list.m3u8"
		referer = "https://cdn.example.com/"
	)
	in := strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-KEY:METHOD=AES-128,URI="key.bin"`,
		"#EXTINF:4.0,",
		"seg1.ts",
		"#EXTINF:4.0,",
		"https://other.example.com/seg2.mp4",
		"",
	}, "\n")

	out := rewritePlaylist(in, base, referer)
	lines := strings.Split(out, "\n")

	if lines[0] != "#EXTM3U" {
		t.Fatalf("头部被破坏: %q", lines[0])
	}
	if !strings.Contains(lines[1], `URI="/api/proxy?url=`) {
		t.Errorf("密钥 URI 没有被改写: %q", lines[1])
	}
	if !strings.HasPrefix(lines[3], "/api/proxy?url=") {
		t.Errorf("相对分片没有被改写: %q", lines[3])
	}
	if !strings.HasPrefix(lines[5], "/api/proxy?url=") {
		t.Errorf("绝对分片没有被改写: %q", lines[5])
	}

	// 解析出改写后的分片地址，确认相对路径解析正确且 referer 被带上
	u, err := url.Parse(lines[3])
	if err != nil {
		t.Fatalf("改写结果不是合法 URL: %v", err)
	}
	if got := u.Query().Get("url"); got != "https://cdn.example.com/hls/seg1.ts" {
		t.Errorf("相对路径解析错误: %q", got)
	}
	if got := u.Query().Get("referer"); got != referer {
		t.Errorf("referer 丢失: %q", got)
	}
}

func TestIsPlaylist(t *testing.T) {
	if !isPlaylist("https://cdn.example.com/a.m3u8?token=x", nil) {
		t.Error("按后缀应识别为播放列表")
	}
	h := map[string][]string{"Content-Type": {"application/vnd.apple.mpegurl"}}
	if !isPlaylist("https://cdn.example.com/play", h) {
		t.Error("按 Content-Type 应识别为播放列表")
	}
	if isPlaylist("https://cdn.example.com/a.mp4", nil) {
		t.Error("mp4 不应识别为播放列表")
	}
}
