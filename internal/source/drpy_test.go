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

func TestParseVodPlayURLWithFrom(t *testing.T) {
	raw := "第1集$https://www.tuxiaobei.com/play/1404#第2集$https://a.com/2.mp4$$$线路2$https://b.com/3.mp4"
	episodes := ParseVodPlayURLWithFrom(raw, "道长在线$$$备用线路")
	if len(episodes) != 3 {
		t.Fatalf("条数 = %d, 期望 3", len(episodes))
	}
	if episodes[0].Flag != "道长在线" || episodes[1].Flag != "道长在线" {
		t.Errorf("第一条线路的 flag 错误: %+v", episodes[:2])
	}
	if episodes[2].Flag != "备用线路" {
		t.Errorf("第二条线路的 flag = %q, 期望 备用线路", episodes[2].Flag)
	}
	if got := ParseVodPlayURLWithFrom(raw, ""); len(got) != 3 || got[0].Flag != "" {
		t.Errorf("play_from 为空时不应带 flag: %+v", got)
	}
}

// 一级/嗅探源：详情给网页地址，play 接口返回 JSON 直链。
func TestPlayMediaJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("play") == "" {
			t.Errorf("缺少 play 参数: %s", r.URL.RawQuery)
		}
		if q.Get("flag") != "道长在线" {
			t.Errorf("flag = %q, 期望 道长在线", q.Get("flag"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"parse":0,"url":"https://cdn.example.com/1.mp4","header":{"Referer":"https://cdn.example.com/"}}`))
	}))
	defer srv.Close()

	d := NewDrpy(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, 5*time.Second)
	res, err := d.PlayMedia(context.Background(), "https://www.tuxiaobei.com/play/1404", "道长在线")
	if err != nil {
		t.Fatalf("PlayMedia 失败: %v", err)
	}
	if res.URL != "https://cdn.example.com/1.mp4" {
		t.Errorf("URL = %q", res.URL)
	}
	if res.Header.Get("Referer") != "https://cdn.example.com/" {
		t.Errorf("header 未解析: %v", res.Header)
	}
}

// 部分源 play 接口 302 到 /mediaProxy，需要把相对 Location 补成绝对地址。
func TestPlayMediaRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/mediaProxy?url=https%3A%2F%2Fcdn.example.com%2F1.mp4")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	d := NewDrpy(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, 5*time.Second)
	res, err := d.PlayMedia(context.Background(), "https://www.tuxiaobei.com/play/1404", "")
	if err != nil {
		t.Fatalf("PlayMedia 失败: %v", err)
	}
	if res.URL != srv.URL+"/mediaProxy?url=https%3A%2F%2Fcdn.example.com%2F1.mp4" {
		t.Errorf("URL = %q", res.URL)
	}
}

// 栏目列表来自 ?ac=home 的 class 数组（type_id / type_name）。
func TestDrpyCategories(t *testing.T) {
	var gotAC string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAC = r.URL.Query().Get("ac")
		w.Header().Set("Content-Type", "application/json")
		// 数字型 type_id（贝乐虎等源）也要能解析，不能整份失败
		w.Write([]byte(`{"class":[{"type_id":"2","type_name":"儿歌"},{"type_id":"3","type_name":"故事"},{"type_id":"","type_name":"脏数据"},{"type_id":"25","type_name":""},{"type_id":66,"type_name":"数字栏目"}]}`))
	}))
	defer srv.Close()

	d := NewDrpy(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, 5*time.Second)
	cats, err := d.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories 失败: %v", err)
	}
	if gotAC != "home" {
		t.Errorf("ac = %q, 期望 home", gotAC)
	}
	if len(cats) != 4 {
		t.Fatalf("栏目数量 = %d, 期望 4: %+v", len(cats), cats)
	}
	if cats[0].ID != "2" || cats[0].Name != "儿歌" {
		t.Errorf("第一个栏目解析错误: %+v", cats[0])
	}
	// 没写名字时用 ID 兜底，前端不至于渲染出空复选框
	if cats[2].ID != "25" || cats[2].Name != "25" {
		t.Errorf("无名栏目应回退为 ID: %+v", cats[2])
	}
	if cats[3].ID != "66" || cats[3].Name != "数字栏目" {
		t.Errorf("数字型 type_id 应转成字符串: %+v", cats[3])
	}
}

func TestDrpyCategoryVideos(t *testing.T) {
	var gotAC, gotT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotAC, gotT = q.Get("ac"), q.Get("t")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"page":1,"list":[{"vod_id":"2$996@@丢手绢@@https://example.com/p.png","vod_name":"丢手绢","vod_pic":"https://example.com/p.png"},{"vod_id":"","vod_name":"脏数据"}]}`))
	}))
	defer srv.Close()

	d := NewDrpy(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, 5*time.Second)
	videos, _, err := d.CategoryVideos(context.Background(), "2")
	if err != nil {
		t.Fatalf("CategoryVideos 失败: %v", err)
	}
	if gotAC != "category" || gotT != "2" {
		t.Errorf("参数错误: ac=%q t=%q", gotAC, gotT)
	}
	if len(videos) != 1 || videos[0].VodName != "丢手绢" {
		t.Errorf("栏目视频解析错误: %+v", videos)
	}
}

// 合集面板要拉全栏目：逐页取到 no_data 为止，重复 vod_id 只保留一条。
func TestDrpyAllCategoryVideos(t *testing.T) {
	var pgCalls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		pg := q.Get("pg")
		pgCalls = append(pgCalls, pg)
		w.Header().Set("Content-Type", "application/json")
		switch pg {
		case "", "1":
			w.Write([]byte(`{"list":[{"vod_id":"2$1@@儿歌一@@https://e.com/1.png","vod_name":"儿歌一"},{"vod_id":"2$2@@儿歌二@@https://e.com/2.png","vod_name":"儿歌二"}]}`))
		case "2":
			w.Write([]byte(`{"list":[{"vod_id":"2$2@@儿歌二@@https://e.com/2.png","vod_name":"儿歌二"},{"vod_id":"2$3@@儿歌三@@https://e.com/3.png","vod_name":"儿歌三"}]}`))
		default:
			// drpy 到底后的占位数据，必须被过滤掉并停止翻页
			w.Write([]byte(`{"list":[{"vod_id":"no_data","vod_name":"无数据,防无限请求"}]}`))
		}
	}))
	defer srv.Close()

	d := NewDrpy(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, 5*time.Second)
	videos, err := d.AllCategoryVideos(context.Background(), "2")
	if err != nil {
		t.Fatalf("AllCategoryVideos 失败: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("条数 = %d, 期望 3（去重后）: %+v", len(videos), videos)
	}
	for i, want := range []string{"儿歌一", "儿歌二", "儿歌三"} {
		if videos[i].VodName != want {
			t.Errorf("第 %d 条 = %q, 期望 %q", i+1, videos[i].VodName, want)
		}
	}
	// 第 4 次请求应该是 pg=3（拿到 no_data 就停，不会继续翻页）
	if len(pgCalls) != 3 || pgCalls[2] != "3" {
		t.Errorf("翻页请求 = %v, 期望 [\"\" 2 3] 后停止", pgCalls)
	}
}

// 首页混合流：把勾选栏目的第一页合并后洗牌（顺序随机，只校验内容都在）。
func TestDrpySourceFeedShufflesCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch q.Get("ac") {
		case "home":
			w.Write([]byte(`{"class":[{"type_id":"2","type_name":"儿歌"},{"type_id":"3","type_name":"故事"}]}`))
		case "category":
			switch q.Get("t") {
			case "2":
				w.Write([]byte(`{"list":[{"vod_id":"2$1","vod_name":"儿歌一"},{"vod_id":"2$2","vod_name":"儿歌二"}]}`))
			case "3":
				w.Write([]byte(`{"list":[{"vod_id":"3$1","vod_name":"故事一"}]}`))
			default:
				t.Errorf("意外的栏目 t = %q", q.Get("t"))
			}
		default:
			t.Errorf("意外的 ac = %q", q.Get("ac"))
		}
	}))
	defer srv.Close()

	s := NewDrpySource(srv.URL, "兔小贝[儿]", DrpyDefaultPwd, "", []string{"2", "3"}, 5*time.Second)
	items, err := s.Feed(context.Background(), "/启蒙", "")
	if err != nil {
		t.Fatalf("Feed 失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("条数 = %d, 期望 3: %+v", len(items), items)
	}
	names := map[string]bool{}
	wantCategory := map[string]string{"儿歌一": "儿歌", "儿歌二": "儿歌", "故事一": "故事"}
	for _, it := range items {
		names[it.Name] = true
		if !IsDrpyPath(it.Path) {
			t.Errorf("path 缺少 drpy: 前缀: %+v", it)
		}
		// 条目要带栏目信息，前端才能按栏目合成合集
		if it.Category != wantCategory[it.Name] {
			t.Errorf("%q 的栏目 = %q, 期望 %q", it.Name, it.Category, wantCategory[it.Name])
		}
	}
	for _, want := range []string{"儿歌一", "儿歌二", "故事一"} {
		if !names[want] {
			t.Errorf("缺少栏目内容 %q: %+v", want, items)
		}
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
