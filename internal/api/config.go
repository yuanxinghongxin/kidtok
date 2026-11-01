package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"qimeng-feed/internal/config"
	"qimeng-feed/internal/source"
)

// probeTimeout 是设置页点「检测」时等待上游的最长时间。
const probeTimeout = 15 * time.Second

// Config 处理 GET /api/config：读取 config.json 回显到配置页，文件不存在时是默认值。
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	OK(w, h.store.Get())
}

// Categories 处理 GET /api/categories[?source_id=]：返回已保存源的栏目，供设置页回显。
func (h *Handler) Categories(w http.ResponseWriter, r *http.Request) {
	o := h.onlineSourceOrFirst(r.URL.Query().Get("source_id"))
	if o == nil {
		Fail(w, http.StatusFailedDependency, CodeUpstreamErr, "没有已启用的在线数据源")
		return
	}

	cats, err := o.Categories(r.Context())
	if err != nil {
		slog.Error("获取栏目列表失败", "source_id", o.ID(), "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "获取栏目列表失败: "+err.Error())
		return
	}
	OK(w, cats)
}

// TestSource 处理 POST /api/source/test：用表单里还没保存的配置连接一次，
// 成功返回该源的全部栏目，失败返回「连接失败：原因」。
func (h *Handler) TestSource(w http.ResponseWriter, r *http.Request) {
	var req config.SourceSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}

	normalized, msg := normalizeSource(req)
	if msg != "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, msg)
		return
	}
	if !normalized.Configured() {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "请先填写完整的连接信息")
		return
	}

	if normalized.Kind == "js" {
		if _, err := source.ValidateScript(normalized.Script); err != nil {
			Fail(w, http.StatusBadRequest, CodeBadRequest, "规则解析失败: "+err.Error())
			return
		}
	}

	cats, err := source.ProbeOnlineSource(r.Context(), normalized, probeTimeout)
	if err != nil {
		slog.Warn("数据源连接检测失败", "kind", normalized.Kind, "name", normalized.Name, "err", err)
		Fail(w, http.StatusBadGateway, CodeUpstreamErr, "连接失败: "+err.Error())
		return
	}
	OK(w, cats)
}

// SaveConfig 处理 POST /api/config：校验 → 试登录 → 写入 config.json。
// 保存后不需要重启：数据源按 config.json 实时解析，下一次 /api/feed 就会用新的配置。
func (h *Handler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var req config.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}

	settings, msg := normalizeSettings(req)
	if msg != "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, msg)
		return
	}

	// 网盘填了地址就先真实登录一次，避免把不可用的地址存下去。
	if settings.Alist.Configured() {
		if err := h.alist.Apply(r.Context(), settings.Alist); err != nil {
			slog.Warn("网盘登录失败，配置仍然保存（在线源不受影响）", "host", settings.Alist.Host, "err", err)
		}
	}

	if err := h.store.Save(settings); err != nil {
		slog.Warn("写入 config.json 失败", "err", err)
		Fail(w, http.StatusInternalServerError, CodeUpstreamErr, "配置已生效，但写入文件失败: "+err.Error())
		return
	}

	enabled := 0
	for _, s := range settings.Sources {
		if s.Enabled {
			enabled++
		}
	}
	slog.Info("数据源配置已更新",
		"alist", settings.Alist.BaseURL(),
		"sources", len(settings.Sources),
		"enabled", enabled)

	OK(w, settings)
}

// normalizeSettings 校验并规范化表单字段，第二个返回值为空表示通过。
func normalizeSettings(s config.Settings) (config.Settings, string) {
	out := config.Settings{
		Alist: config.AlistSettings{
			Protocol: strings.ToLower(strings.TrimSpace(s.Alist.Protocol)),
			Host:     strings.TrimSpace(s.Alist.Host),
			Port:     strings.TrimSpace(s.Alist.Port),
			Username: strings.TrimSpace(s.Alist.Username),
			Password: s.Alist.Password,
			BasePath: strings.TrimSpace(s.Alist.BasePath),
			Disabled: s.Alist.Disabled,
		},
		Sources: make([]config.SourceSettings, 0, len(s.Sources)),
	}

	if out.Alist.Protocol == "" {
		out.Alist.Protocol = "http"
	}
	if out.Alist.Protocol != "http" && out.Alist.Protocol != "https" {
		return out, "网盘协议只能是 http 或 https"
	}

	out.Alist.Host = strings.TrimPrefix(out.Alist.Host, "http://")
	out.Alist.Host = strings.TrimPrefix(out.Alist.Host, "https://")
	out.Alist.Host = strings.TrimSuffix(out.Alist.Host, "/")
	if out.Alist.Host != "" {
		if strings.ContainsAny(out.Alist.Host, " /") {
			return out, "网盘地址只能是 IP 或域名，不要带路径"
		}
		n, err := strconv.Atoi(out.Alist.Port)
		if err != nil || n < 1 || n > 65535 {
			return out, "网盘端口必须是 1-65535 之间的数字"
		}
	}
	if out.Alist.BasePath != "" && !strings.HasPrefix(out.Alist.BasePath, "/") {
		out.Alist.BasePath = "/" + out.Alist.BasePath
	}

	seen := make(map[string]bool, len(s.Sources))
	for i, raw := range s.Sources {
		sc, msg := normalizeSource(raw)
		if msg != "" {
			return out, fmt.Sprintf("第 %d 个数据源：%s", i+1, msg)
		}
		if sc.ID == "" {
			sc.ID = fmt.Sprintf("s%d", i+1)
		}
		if seen[sc.ID] {
			return out, fmt.Sprintf("数据源 ID %q 重复", sc.ID)
		}
		seen[sc.ID] = true
		out.Sources = append(out.Sources, sc)
	}
	return out, ""
}

// normalizeSource 校验单个数据源配置。
func normalizeSource(s config.SourceSettings) (config.SourceSettings, string) {
	out := config.SourceSettings{
		ID:         strings.TrimSpace(s.ID),
		Kind:       strings.ToLower(strings.TrimSpace(s.Kind)),
		Name:       strings.TrimSpace(s.Name),
		Enabled:    s.Enabled,
		Categories: normalizeIDs(s.Categories),
		BaseURL:    strings.TrimSpace(s.BaseURL),
		SourceName: strings.TrimSpace(s.SourceName),
		Pwd:        strings.TrimSpace(s.Pwd),
		Script:     s.Script,
		ScriptName: strings.TrimSpace(s.ScriptName),
	}

	switch out.Kind {
	case "drpy":
		if out.Pwd == "" {
			out.Pwd = source.DrpyDefaultPwd
		}
		if out.BaseURL != "" {
			if !strings.Contains(out.BaseURL, "://") {
				out.BaseURL = "http://" + out.BaseURL
			}
			u, err := url.Parse(out.BaseURL)
			if err != nil || u.Host == "" {
				return out, "drpy 服务端地址格式不正确，示例 http://192.168.0.201:5757"
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				return out, "drpy 服务端地址只支持 http 或 https"
			}
			out.BaseURL = strings.TrimRight(out.BaseURL, "/")
		}
		if out.Name == "" {
			out.Name = out.SourceName
		}
		if out.Enabled && out.SourceName == "" {
			return out, "启用 drpy 数据源前请填写源名称"
		}
		if out.Enabled && out.BaseURL == "" {
			return out, "启用 drpy 数据源前请填写服务端地址"
		}
	case "js":
		out.Script = strings.TrimSpace(s.Script)
		if out.Name == "" {
			out.Name = out.ScriptName
		}
		if out.Script != "" {
			title, err := source.ValidateScript(out.Script)
			if err != nil {
				return out, err.Error()
			}
			if out.Name == "" {
				out.Name = title
			}
		}
		if out.Enabled && out.Script == "" {
			return out, "启用 JS 数据源前请上传或粘贴规则文件"
		}
		if out.Enabled && out.Name == "" {
			out.Name = "JS 数据源"
		}
	default:
		return out, "数据源类型只能是 drpy 或 js"
	}
	return out, ""
}

// normalizeIDs 去空格、去空值、去重，并保持用户勾选的顺序。
func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
