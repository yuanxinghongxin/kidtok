package source

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Site 是配置 JSON 中 sites 数组的一个条目。
// 上游字段类型不稳定（如 ext 可能是字符串也可能是对象，playerType 可能是数字也可能是字符串），
// 因此非核心字段统一用 json.RawMessage 原样透传。
type Site struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Type        int             `json:"type"`
	API         string          `json:"api,omitempty"`
	Searchable  int             `json:"searchable"`
	QuickSearch int             `json:"quickSearch"`
	Indexs      int             `json:"indexs,omitempty"`
	Changeable  int             `json:"changeable,omitempty"`
	Timeout     int             `json:"timeout,omitempty"`
	PlayerType  json.RawMessage `json:"playerType,omitempty"`
	Filter      json.RawMessage `json:"filter,omitempty"`
	Style       json.RawMessage `json:"style,omitempty"`
	Ext         json.RawMessage `json:"ext,omitempty"`
}

// FilterSitesByKeyword 解析配置 JSON，返回 name 或 key 包含 keyword 的站点。
// keyword 为空时返回全部站点；没有匹配时返回空数组（而非 null）。
func FilterSitesByKeyword(configJSON string, keyword string) ([]Site, error) {
	var payload struct {
		Sites []Site `json:"sites"`
	}
	if err := json.Unmarshal([]byte(stripLineComments(configJSON)), &payload); err != nil {
		return nil, fmt.Errorf("解析配置 JSON 失败: %w", err)
	}

	kw := strings.ToLower(strings.TrimSpace(keyword))
	result := make([]Site, 0, len(payload.Sites))
	for _, site := range payload.Sites {
		if kw == "" ||
			strings.Contains(strings.ToLower(site.Name), kw) ||
			strings.Contains(strings.ToLower(site.Key), kw) {
			result = append(result, site)
		}
	}
	return result, nil
}
