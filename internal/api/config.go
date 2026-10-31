package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"qimeng-feed/internal/config"
)

// Config 处理 GET /api/config：读取 config.json 回显到配置页，文件不存在时是默认值。
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	OK(w, h.store.Get())
}

// SaveConfig 处理 POST /api/config：校验 → 试登录 → 写入 config.json。
// 保存后不需要重启，/api/feed 与 /api/video/url 下一次请求就会用新配置。
func (h *Handler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var req config.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}

	settings, msg := normalizeSettings(req)
	if msg != "" {
		Fail(w, http.StatusBadRequest, CodeBadRequest, msg)
		return
	}

	// 先用新配置真实登录一次，失败就不写入，避免把不可用的地址存下去。
	if err := h.alist.Apply(r.Context(), settings); err != nil {
		slog.Error("保存网盘配置时登录失败", "host", settings.Host, "err", err)
		Fail(w, http.StatusBadRequest, CodeBadRequest, "登录失败，请检查地址、端口与账号密码: "+err.Error())
		return
	}

	if err := h.store.Save(settings); err != nil {
		slog.Warn("写入 config.json 失败", "err", err)
		Fail(w, http.StatusInternalServerError, CodeUpstreamErr, "配置已生效，但写入文件失败: "+err.Error())
		return
	}
	slog.Info("网盘配置已更新", "host", settings.Host, "port", settings.Port, "base_path", settings.BasePath)

	OK(w, settings)
}

// normalizeSettings 校验并规范化表单字段，第二个返回值为空表示通过。
func normalizeSettings(s config.Settings) (config.Settings, string) {
	out := config.Settings{
		Protocol: strings.ToLower(strings.TrimSpace(s.Protocol)),
		Host:     strings.TrimSpace(s.Host),
		Port:     strings.TrimSpace(s.Port),
		Username: strings.TrimSpace(s.Username),
		Password: s.Password,
		BasePath: strings.TrimSpace(s.BasePath),
	}

	if out.Protocol != "http" && out.Protocol != "https" {
		return out, "协议只能是 http 或 https"
	}

	out.Host = strings.TrimPrefix(out.Host, "http://")
	out.Host = strings.TrimPrefix(out.Host, "https://")
	out.Host = strings.TrimSuffix(out.Host, "/")
	if out.Host == "" {
		return out, "地址不能为空"
	}
	if strings.ContainsAny(out.Host, " /") {
		return out, "地址只能是 IP 或域名，不要带路径"
	}

	n, err := strconv.Atoi(out.Port)
	if err != nil || n < 1 || n > 65535 {
		return out, "端口必须是 1-65535 之间的数字"
	}

	if out.BasePath != "" && !strings.HasPrefix(out.BasePath, "/") {
		out.BasePath = "/" + out.BasePath
	}
	return out, ""
}
