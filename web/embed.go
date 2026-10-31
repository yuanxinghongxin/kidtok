// Package web 嵌入前端静态资源。
// go:embed 的路径不能跨出所在目录（不支持 ../），所以把嵌入声明放在 web 包内，
// 由 cmd/server 通过 fs.Sub 挂到根路径。
package web

import "embed"

// Files 是嵌入的前端资源，包含 index.html、style.css、app.js 以及 PWA 的 manifest.json。
//
//go:embed index.html style.css app.js mpegts.js manifest.json
var Files embed.FS
