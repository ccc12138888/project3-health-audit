// Package handler：HTTP 层。本文件只负责「返回嵌入的仪表盘 HTML」。
//
// 是什么：把编译进二进制的 index.html 字节流以 text/html 返回给浏览器。
// 干什么：用户访问 GET / 时看到可视化页面。
// 怎么用：浏览器打开 http://127.0.0.1:8080/ （端口以你配置为准）。
package handler

import (
	"net/http"

	"github.com/example/health-audit/internal/webui" // init 里填充 IndexHTML

	"github.com/gin-gonic/gin"
)

// ServeDashboard Gin 处理函数：禁止缓存，返回 UTF-8 HTML。
func ServeDashboard(c *gin.Context) {
	c.Header("Cache-Control", "no-store") // 避免浏览器一直用旧版嵌入页
	c.Data(http.StatusOK, "text/html; charset=utf-8", webui.IndexHTML)
}
