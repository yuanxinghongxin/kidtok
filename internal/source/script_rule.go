package source

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 这里实现的是 drpy「配表式」规则的最小解析器：
//   var rule = { host:'...', url:'/list?type=fyclass&page=fypage', detailUrl:'/play/fyid', ... }
//
// 只支持声明式字段（URL 模板 + class 列表 / class_parse），
// 规则里如果包含一级/二级/搜索 这类自定义 JS 函数，则明确告知暂不支持。
// 鸭子类型解析：不引入 JS 引擎，避免为了几个字符串字段把二进制体积做大。

func parseScriptRule(script string) (*scriptRule, error) {
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("规则文件为空")
	}
	if !ruleDeclRe.MatchString(script) {
		return nil, fmt.Errorf("文件里没有找到 `rule = {...}`（不是 drpy 规则文件？）")
	}

	r := &scriptRule{Headers: map[string]string{}}

	r.Title = jsString(script, "title")
	r.Host = strings.TrimRight(strings.TrimSpace(jsString(script, "host")), "/")
	r.HomeURL = strings.TrimSpace(jsString(script, "homeUrl"))
	r.URLTpl = strings.TrimSpace(jsString(script, "url"))
	r.DetailTpl = strings.TrimSpace(jsString(script, "detailUrl"))
	r.SearchTpl = strings.TrimSpace(jsString(script, "searchUrl"))
	r.ClassParse = strings.TrimSpace(jsString(script, "class_parse"))

	if raw, ok := jsRaw(script, "headers"); ok {
		r.Headers = parseJSHeaders(raw)
	}

	if sec := jsNumber(script, "timeout"); sec > 0 {
		r.Timeout = time.Duration(sec) * time.Second
	} else {
		r.Timeout = 10 * time.Second
	}

	// 静态栏目：class 数组优先，其次 class_name / class_url 两个逗号串。
	if raw, ok := jsRaw(script, "class"); ok {
		r.Classes = parseJSClassArray(raw)
	}
	if len(r.Classes) == 0 {
		r.Classes = parseJSClassLists(jsString(script, "class_name"), jsString(script, "class_url"))
	}

	// 函数型规则无法在 Go 里执行。
	for _, key := range []string{"一级", "二级", "搜索", "推荐", "播放", "class_parse_func"} {
		if raw, ok := jsRaw(script, key); ok && looksLikeFunction(raw) {
			r.HasFuncRule = true
		}
	}
	if looksLikeFunction(rawFieldOr(script, "url")) {
		r.HasFuncRule = true
	}
	if looksLikeFunction(rawFieldOr(script, "detailUrl")) {
		r.HasFuncRule = true
	}

	if r.HasFuncRule {
		return nil, fmt.Errorf("该规则包含自定义 JS 函数（一级/二级/搜索/播放），当前内置解析器暂不支持；请改用 drpy-node 源")
	}
	if r.Host == "" {
		return nil, fmt.Errorf("规则缺少 host")
	}
	if r.URLTpl == "" {
		return nil, fmt.Errorf("规则缺少列表模板 url")
	}
	return r, nil
}

// ValidateScript 校验上传的 JS 规则能否被内置解析器识别；
// 第二个返回值是规则里声明的源名（可能为空），供设置页自动填名称。
func ValidateScript(script string) (string, error) {
	r, err := parseScriptRule(script)
	if err != nil {
		return "", err
	}
	return r.Title, nil
}

var ruleDeclRe = regexp.MustCompile(`(?:var|let|const)?\s*rule\s*=`)

// fieldRe 匹配 `key:`，要求 key 位于对象成员起始位置，避免匹配到字符串内容。
func fieldRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?:^|[\s,{])` + regexp.QuoteMeta(key) + `\s*:\s*`)
}

// jsString 读取字符串字段的值；字段不存在或不是字符串时返回空串。
func jsString(script, key string) string {
	raw, ok := jsRaw(script, key)
	if !ok {
		return ""
	}
	return unquoteJS(raw)
}

// jsRaw 读取字段的原始值（字符串会保留引号，对象/数组保留花括号）。
func jsRaw(script, key string) (string, bool) {
	loc := fieldRe(key).FindStringIndex(script)
	if loc == nil {
		return "", false
	}
	return readJSValue(script[loc[1]:])
}

func rawFieldOr(script, key string) string {
	raw, _ := jsRaw(script, key)
	return raw
}

// readJSValue 从字段冒号后读取一个 JS 字面量：引号字符串、花括号块或裸值。
func readJSValue(s string) (string, bool) {
	s = strings.TrimLeft(s, " \t\r\n")
	if s == "" {
		return "", false
	}
	switch s[0] {
	case '\'', '"':
		quote := s[0]
		var b strings.Builder
		b.WriteByte(quote)
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				b.WriteByte(s[i])
				i++
				b.WriteByte(s[i])
				continue
			}
			b.WriteByte(s[i])
			if s[i] == quote {
				return b.String(), true
			}
		}
		return b.String(), true
	case '{', '[':
		return readBalanced(s)
	default:
		end := strings.IndexAny(s, ",\r\n")
		if end < 0 {
			end = len(s)
		}
		val := strings.TrimSpace(s[:end])
		val = strings.TrimSuffix(val, "}")
		return val, val != ""
	}
}

// readBalanced 读取配对的 {...} 或 [...]，跳过字符串里的括号。
func readBalanced(s string) (string, bool) {
	open := s[0]
	close := byte('}')
	if open == '[' {
		close = ']'
	}
	depth := 0
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inQuote = ch
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[:i+1], true
			}
		}
	}
	return s, true
}

// unquoteJS 去掉一层引号并处理常见转义。
func unquoteJS(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return raw
	}
	q := raw[0]
	if (q != '\'' && q != '"') || raw[len(raw)-1] != q {
		return raw
	}
	body := raw[1 : len(raw)-1]
	replacer := strings.NewReplacer(`\'`, `'`, `\"`, `"`, `\\`, `\`, `\n`, "\n", `\t`, "\t")
	return replacer.Replace(body)
}

func jsNumber(script, key string) int {
	raw, ok := jsRaw(script, key)
	if !ok {
		return 0
	}
	raw = strings.Trim(raw, `"' `)
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func looksLikeFunction(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "function") ||
		strings.HasPrefix(raw, "async") ||
		strings.Contains(raw, "=>")
}

// parseJSHeaders 解析 headers 对象里的字符串键值对。
var jsHeaderRe = regexp.MustCompile(`['"]?([A-Za-z0-9_-]+)['"]?\s*:\s*(['"])([^'"]*)['"]?`)

func parseJSHeaders(raw string) map[string]string {
	out := map[string]string{}
	for _, m := range jsHeaderRe.FindAllStringSubmatch(raw, -1) {
		key := m[1]
		val := expandJSHeaderValue(m[3])
		if key != "" && val != "" {
			out[key] = val
		}
	}
	return out
}

// expandJSHeaderValue 展开 drpy 常用的 UA 占位符。
func expandJSHeaderValue(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "MOBILE_UA":
		return drpyUA
	case "PC_UA":
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	default:
		return strings.TrimSpace(v)
	}
}

// parseJSClassArray 解析 class: [{type_id:'1',type_name:'儿歌'}, ...]。
var jsClassObjRe = regexp.MustCompile(`(?s)\{([^{}]*)\}`)
var jsTypeIDRe = regexp.MustCompile(`['"]?type_id['"]?\s*:\s*['"]?([^'",}\s]+)['"]?`)
var jsTypeNameRe = regexp.MustCompile(`['"]?type_name['"]?\s*:\s*(['"])([^'"]*)['"]?`)

func parseJSClassArray(raw string) []DrpyCategory {
	cats := []DrpyCategory{}
	for _, m := range jsClassObjRe.FindAllStringSubmatch(raw, -1) {
		body := m[1]
		idMatch := jsTypeIDRe.FindStringSubmatch(body)
		if len(idMatch) < 2 {
			continue
		}
		id := strings.TrimSpace(idMatch[1])
		if id == "" {
			continue
		}
		name := id
		if nm := jsTypeNameRe.FindStringSubmatch(body); len(nm) >= 3 && strings.TrimSpace(nm[2]) != "" {
			name = strings.TrimSpace(nm[2])
		}
		cats = append(cats, DrpyCategory{ID: id, Name: name})
	}
	return cats
}

// parseJSClassLists 解析 class_name / class_url 两个并列字符串。
func parseJSClassLists(names, ids string) []DrpyCategory {
	names = strings.TrimSpace(names)
	ids = strings.TrimSpace(ids)
	if names == "" || ids == "" {
		return nil
	}
	nameParts := splitList(names)
	idParts := splitList(ids)
	n := len(nameParts)
	if len(idParts) < n {
		n = len(idParts)
	}
	cats := make([]DrpyCategory, 0, n)
	for i := 0; i < n; i++ {
		id := strings.TrimSpace(idParts[i])
		if id == "" {
			continue
		}
		cats = append(cats, DrpyCategory{ID: id, Name: strings.TrimSpace(nameParts[i])})
	}
	return cats
}

func splitList(s string) []string {
	if strings.Contains(s, "&") {
		return strings.Split(s, "&")
	}
	return strings.Split(s, ",")
}
