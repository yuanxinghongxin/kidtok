package source

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"
)

func gzipBase64(t *testing.T, text string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(text)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// 贝乐虎规则把栏目藏在 class_parse 的 ungzip('base64') 里，
// 解析器要能解出来，并且不再因为 一级/推荐 是函数而拒绝整份规则。
func TestParseUbestkidRule(t *testing.T) {
	packed := gzipBase64(t, `[{"type_id":56,"type_name":"🐯经典童谣"},{"type_id":65,"type_name":"🐯最新上架"}]`)
	script := `var rule = {
  title: '贝乐虎[儿]',
  host: 'https://vd.ubestkid.com',
  url: '/api/v1/bv/video#pg=fypage',
  class_parse: async function () { let input = JSON.parse(ungzip('` + packed + `')); return {class: input} },
  一级: async function () { return [] },
  推荐: async function () { return [] },
}`

	r, err := parseScriptRule(script)
	if err != nil {
		t.Fatalf("parse ubestkid rule: %v", err)
	}
	if len(r.Classes) != 2 {
		t.Fatalf("classes = %d, want 2", len(r.Classes))
	}
	if r.Classes[0].ID != "56" || r.Classes[0].Name != "🐯经典童谣" {
		t.Fatalf("first class = %+v", r.Classes[0])
	}
	if !isUbestkidHost(r.Host) {
		t.Fatalf("host %q should be recognized as ubestkid", r.Host)
	}
}

// 同样含自定义函数的其它源，仍然明确拒绝。
func TestParseFuncRuleRejected(t *testing.T) {
	script := `var rule = {
  title: '某源',
  host: 'https://example.com',
  url: '/list?type=fyclass&page=fypage',
  一级: async function () { return [] },
}`
	if _, err := parseScriptRule(script); err == nil {
		t.Fatal("function rule on unsupported host should be rejected")
	}
}
