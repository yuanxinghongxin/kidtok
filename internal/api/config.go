package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"qimeng-feed/internal/config"
)

// Config 处理 GET /api/config：读取 config.json 回显到配置页，文件不存在时是默认值。
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	OK(w, h.store.Get())
}

// SaveConfig 处理 POST /api/config：校验 → 试登录 → 写入 config.json。
// 保存后不需要重启：数据源按 config.json 实时解析，下一次 /api/feed 就会用新的默认源。
func (h *Handler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var req config.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}

	settings, msg := normalizeSettings(req)
	if msg != "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, msg)
		return
	}

	// 网盘填了地址就先真实登录一次，避免把不可用的地址存下去。
	// 当前默认源是 drpy 时，网盘登录失败不阻断保存（只是网盘源不可用）。
	if settings.Alist.Configured() {
		if err := h.alist.Apply(r.Context(), settings.Alist); err != nil {
			if settings.PrimarySource == "alist" {
				slog.Error("保存网盘配置时登录失败", "host", settings.Alist.Host, "err", err)
				Fail(w, http.StatusBadRequest, CodeBadRequest, "登录失败，请检查地址、端口与账号密码: "+err.Error())
				return
			}
			slog.Warn("网盘登录失败，但默认源是 drpy，配置仍然保存", "host", settings.Alist.Host, "err", err)
		}
	}

	if err := h.store.Save(settings); err != nil {
		slog.Warn("写入 config.json 失败", "err", err)
		Fail(w, http.StatusInternalServerError, CodeUpstreamErr, "配置已生效，但写入文件失败: "+err.Error())
		return
	}
	slog.Info("配置已更新，数据源立即生效",
		"primary", settings.PrimarySource,
		"alist", settings.Alist.BaseURL(),
		"drpy", settings.Drpy.BaseURL,
		"drpy_source", settings.Drpy.Name)

	OK(w, settings)
}

// normalizeSettings 校验并规范化表单字段，第二个返回值为空表示通过。
func normalizeSettings(s config.Settings) (config.Settings, string) {
	out := config.Settings{
		PrimarySource: strings.ToLower(strings.TrimSpace(s.PrimarySource)),
		Alist: config.AlistSettings{
			Protocol: strings.ToLower(strings.TrimSpace(s.Alist.Protocol)),
			Host:     strings.TrimSpace(s.Alist.Host),
			Port:     strings.TrimSpace(s.Alist.Port),
			Username: strings.TrimSpace(s.Alist.Username),
			Password: s.Alist.Password,
			BasePath: strings.TrimSpace(s.Alist.BasePath),
		},
		Drpy: config.DrpySettings{
			BaseURL: strings.TrimSpace(s.Drpy.BaseURL),
			Name:    strings.TrimSpace(s.Drpy.Name),
			Pwd:     strings.TrimSpace(s.Drpy.Pwd),
			Keyword: strings.TrimSpace(s.Drpy.Keyword),
		},
	}

	if out.PrimarySource != "alist" && out.PrimarySource != "drpy" {
		return out, "默认数据源只能是 alist 或 drpy"
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
	if out.Alist.Host == "" {
		if out.PrimarySource == "alist" {
			return out, "选择 AList 作为默认数据源时，请填写网盘地址"
		}
	} else {
		if strings.ContainsAny(out.Alist.Host, " /") {
			return out, "网盘地址只能是 IP 或域名，不要带路径"
		}
		n, err := strconv.Atoi(out.Alist.Port)
		if err != nil || n < 1 || n > 65535 {
			return out, "网盘端口必须是 1-65535 之间的数字"
		}
		if out.Alist.BasePath != "" && !strings.HasPrefix(out.Alist.BasePath, "/") {
			out.Alist.BasePath = "/" + out.Alist.BasePath
		}
	}

	if out.Drpy.BaseURL != "" {
		if !strings.Contains(out.Drpy.BaseURL, "://") {
			out.Drpy.BaseURL = "http://" + out.Drpy.BaseURL
		}
		u, err := url.Parse(out.Drpy.BaseURL)
		if err != nil || u.Host == "" {
			return out, "Drpy 服务端地址格式不正确，示例 http://192.168.0.201:5757"
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return out, "Drpy 服务端地址只支持 http 或 https"
		}
		out.Drpy.BaseURL = strings.TrimRight(out.Drpy.BaseURL, "/")
	}
	if out.Drpy.Keyword == "" {
		out.Drpy.Keyword = "儿歌"
	}
	if out.PrimarySource == "drpy" {
		if out.Drpy.BaseURL == "" {
			return out, "选择 Drpy 作为默认数据源时，请填写服务端地址"
		}
		if out.Drpy.Name == "" {
			return out, "选择 Drpy 作为默认数据源时，请填写源名称"
		}
	}

	return out, ""
}
