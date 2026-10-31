// Package api 提供 HTTP 接口层：统一响应格式与路由。
package api

import (
	"encoding/json"
	"net/http"
)

// 业务状态码：0 表示成功，其余为失败。
const (
	CodeOK          = 0
	CodeBadRequest  = 4001
	CodeUpstreamErr = 5001
)

// Envelope 是所有接口的统一响应结构。
type Envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

// WriteJSON 写出统一格式的 JSON 响应。
func WriteJSON(w http.ResponseWriter, status int, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Code: code, Msg: msg, Data: data})
}

// OK 写出成功响应。
func OK(w http.ResponseWriter, data any) {
	WriteJSON(w, http.StatusOK, CodeOK, "ok", data)
}

// Fail 写出失败响应，data 固定为 null。
func Fail(w http.ResponseWriter, status int, code int, msg string) {
	WriteJSON(w, status, code, msg, nil)
}
