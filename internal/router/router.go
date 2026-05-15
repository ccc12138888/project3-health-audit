// Package router：仅负责「URL + HTTP 方法 → 处理函数」的注册表。
//
// 是什么：对 gin.Engine 调用 GET/POST 等。
// 干什么：把 REST API 与仪表盘首页挂到路径上。
// 怎么用：main 里 router.Mount(engine, api)；改路由只改本文件。
package router

import (
	"github.com/example/health-audit/internal/handler"

	"github.com/gin-gonic/gin"
)

// Mount 在引擎 r 上注册全部路由；api 里挂好了各 Service/Repository。
func Mount(r *gin.Engine, api *handler.API) {
	// 存活探测：负载均衡或运维脚本常用 GET /healthz
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API 统一前缀 /api/v1，便于以后做 v2 并存
	v1 := r.Group("/api/v1")
	{
		// --- 接口配置 CRUD ---
		v1.POST("/endpoints", api.CreateEndpoint)       // 新增一条待巡检 URL
		v1.GET("/endpoints", api.ListEndpoints)       // 列表
		v1.GET("/endpoints/:id", api.GetEndpoint)     // :id 为路径参数，如 /endpoints/3
		v1.PUT("/endpoints/:id", api.UpdateEndpoint)  // 修改
		v1.DELETE("/endpoints/:id", api.DeleteEndpoint) // 删除
		v1.GET("/endpoints/:id/logs", api.ListEndpointLogs) // 某条配置的历史日志

		// --- 巡检控制 ---
		v1.POST("/check/run", api.RunCheck)       // 手动立即跑一轮全量巡检
		v1.GET("/check/state", api.CheckState)    // 是否处于「结束巡检」暂停
		v1.POST("/check/pause", api.PauseCheck)   // 暂停探活
		v1.POST("/check/resume", api.ResumeCheck) // 恢复探活

		// --- 日志查询 ---
		v1.GET("/logs/recent", api.ListRecentLogs)     // ?limit= 最近全局日志
		v1.GET("/logs/anomalies", api.ListAnomalies) // 只看 anomaly=true
	}

	// 首页：嵌入的 HTML 仪表盘（与 /api 不冲突）
	r.GET("/", handler.ServeDashboard)
}
