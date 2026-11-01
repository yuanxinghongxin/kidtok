package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDrpySearchAndDetail(t *testing.T) {
	var gotPath, gotAC, gotWD, gotIDs string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		q := r.URL.Query()
		gotAC = q.Get("ac")
		if q.Get("pwd") != DrpyDefaultPwd {
			t.Errorf("pwd = %q, 期望 %q", q.Get("pwd"), DrpyDefaultPwd)
		}
		switch q.Get("ac") {
		case "search":
			gotWD = q.Get("wd")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"list":[{"vod_id":"/play/1404","vod_name":"亲亲小兔子","vod_pic":"https://example.com/p.jpg"},{"vod_id":"","vod_name":"脏数据"}]}`))
		case "detail":
			gotIDs = q.Get("ids")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"list":[{"vod_id":"/play/1404","vod_play_url":"第1集$https://a.example.com/1.m3u8#第2集$https://b.example.com/2.mp4$$$线路2$https://c.example.com/3.m3u8"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := NewDrpy(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, 5*time.Second)
	ctx := context.Background()

	videos, err := d.SearchVideos(ctx, "小猪佩奇")
	if err != nil {
		t.Fatalf("SearchVideos 失败: %v", err)
	}
	if gotPath != "/api/"+"兔小贝[儿]" {
		t.Errorf("请求路径 = %q, 期望解码后的 /api/兔小贝[儿]", gotPath)
	}
	if gotAC != "search" || gotWD != "小猪佩奇" {
		t.Errorf("搜索参数错误: ac=%q wd=%q", gotAC, gotWD)
	}
	if len(videos) != 1 || videos[0].VodID != "/play/1404" || videos[0].VodName != "亲亲小兔子" {
		t.Errorf("搜索结果不符合预期: %+v", videos)
	}

	episodes, err := d.GetPlayUrl(ctx, videos[0].VodID)
	if err != nil {
		t.Fatalf("GetPlayUrl 失败: %v", err)
	}
	if gotAC != "detail" || gotIDs != "/play/1404" {
		t.Errorf("详情参数错误: ac=%q ids=%q", gotAC, gotIDs)
	}
	if len(episodes) != 3 {
		t.Fatalf("剧集数量 = %d, 期望 3: %+v", len(episodes), episodes)
	}
	if episodes[0].Name != "第1集" || episodes[0].URL != "https://a.example.com/1.m3u8" {
		t.Errorf("第 1 集解析错误: %+v", episodes[0])
	}
}

func TestParseVodPlayURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"空值", "", nil},
		{"单线路", "第1集$https://a.com/1.m3u8#第2集$https://a.com/2.mp4", []string{"https://a.com/1.m3u8", "https://a.com/2.mp4"}},
		{"多线路", "线路1$https://a.com/1.m3u8$$$线路2$https://b.com/1.m3u8", []string{"https://a.com/1.m3u8", "https://b.com/1.m3u8"}},
		{"非链接条目被跳过", "第1集$return [];#第2集$https://a.com/2.m3u8", []string{"https://a.com/2.m3u8"}},
		{"多条$只取第一个", "第1集$https://a.com/1.m3u8$https://a.com/1b.m3u8", []string{"https://a.com/1.m3u8"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseVodPlayURL(c.raw)
			if len(got) != len(c.want) {
				t.Fatalf("条数 = %d, 期望 %d: %+v", len(got), len(c.want), got)
			}
			for i, u := range c.want {
				if got[i].URL != u {
					t.Errorf("第 %d 条 = %q, 期望 %q", i+1, got[i].URL, u)
				}
			}
		})
	}
}
