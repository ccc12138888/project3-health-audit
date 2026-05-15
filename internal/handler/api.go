// Package handler：HTTP 入口（Controller）。把 JSON 请求转成 service/repository 调用，再写回 JSON。
//
// 是什么：Gin 的 HandlerFunc 方法集合，挂在 router 上。
// 干什么：参数校验、HTTP 状态码、错误转 JSON。
// 怎么用：用 curl/Postman/浏览器调 /api/v1/...；或打开首页用内置表单。
package handler

import (
	"errors"   // errors.Is 判断 Gorm 未找到、业务 ErrCheckPaused
	"net/http" // 标准 HTTP 状态码常量
	"strconv"  // 路径里的 id 字符串转 uint

	"github.com/example/health-audit/internal/model"
	"github.com/example/health-audit/internal/repository"
	"github.com/example/health-audit/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// API 把各业务依赖注入进来；router 创建时一次性填好。
type API struct {
	Endpoints *service.EndpointService              // 接口配置业务
	Health    *service.HealthCheckService          // 巡检业务
	Logs      *repository.CheckLogRepository       // 日志直接走仓储（也可再包一层 service）
}

// createEndpointReq 是 POST /endpoints 的 JSON 体结构；binding 标签由 Gin 校验。
type createEndpointReq struct {
	Name    string `json:"name" binding:"required"` // 必填
	URL     string `json:"url" binding:"required"`  // 必填
	Method  string `json:"method"`                  // 可选
	Enabled *bool  `json:"enabled"`                 // 指针：nil 表示「未传」，用默认 true
}

// CreateEndpoint POST /api/v1/endpoints：登记一条待巡检 URL。
func (a *API) CreateEndpoint(c *gin.Context) {
	var req createEndpointReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) // 400 客户端参数错
		return
	}
	ep := model.Endpoint{
		Name:    req.Name,
		URL:     req.URL,
		Method:  req.Method,
		Enabled: true, // 默认启用
	}
	if req.Enabled != nil {
		ep.Enabled = *req.Enabled // 客户端显式传了 enabled 则覆盖
	}
	if err := a.Endpoints.Create(&ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}) // 500 如唯一键冲突
		return
	}
	c.JSON(http.StatusCreated, ep) // 201 并返回含自增 ID 的完整对象
}

// ListEndpoints GET /api/v1/endpoints：返回全部配置（含停用）。
func (a *API) ListEndpoints(c *gin.Context) {
	list, err := a.Endpoints.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GetEndpoint GET /api/v1/endpoints/:id：查单条配置。
func (a *API) GetEndpoint(c *gin.Context) {
	id, err := parseID(c.Param("id")) // Gin 把 :id 匹配到的片段交给 Param
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ep, err := a.Endpoints.Get(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"}) // 404
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ep)
}

// UpdateEndpoint PUT /api/v1/endpoints/:id：整体更新一行（body 里带各字段）。
func (a *API) UpdateEndpoint(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body model.Endpoint
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.ID = id // 防止客户端篡改 URL 里的 id，以路径为准
	if err := a.Endpoints.Update(&body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, body)
}

// DeleteEndpoint DELETE /api/v1/endpoints/:id：删除配置。
func (a *API) DeleteEndpoint(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := a.Endpoints.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent) // 204 无 body
}

// RunCheck POST /api/v1/check/run：立刻跑一轮全量巡检（受「暂停」影响）。
func (a *API) RunCheck(c *gin.Context) {
	// 使用请求的 Context：浏览器断开时可取消本轮（已投递到池里的仍会跑）
	if err := a.Health.RunBatch(c.Request.Context()); err != nil {
		if errors.Is(err, service.ErrCheckPaused) {
			// 409 Conflict：资源状态与操作冲突（此处为「已暂停仍要巡检」）
			c.JSON(http.StatusConflict, gin.H{"error": "巡检已暂停，请先点击「继续巡检」", "paused": true})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "paused": false})
}

// CheckState GET /api/v1/check/state：给前端轮询当前是否暂停。
func (a *API) CheckState(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"paused": a.Health.Paused()})
}

// PauseCheck POST /api/v1/check/pause：结束巡检（内存标志，不写库）。
func (a *API) PauseCheck(c *gin.Context) {
	a.Health.Pause()
	c.JSON(http.StatusOK, gin.H{"paused": true, "message": "已结束巡检（暂停），定时与手动探活均不会执行"})
}

// ResumeCheck POST /api/v1/check/resume：继续巡检。
func (a *API) ResumeCheck(c *gin.Context) {
	a.Health.Resume()
	c.JSON(http.StatusOK, gin.H{"paused": false, "message": "已恢复巡检"})
}

// ListEndpointLogs GET /api/v1/endpoints/:id/logs：某接口最近 100 条巡检记录。
func (a *API) ListEndpointLogs(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	logs, err := a.Logs.ListByEndpoint(id, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// ListAnomalies GET /api/v1/logs/anomalies：全局异常记录，最多 200 条。
func (a *API) ListAnomalies(c *gin.Context) {
	logs, err := a.Logs.ListAnomalies(200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// ListRecentLogs GET /api/v1/logs/recent?limit=50：全局最近日志，limit 可改。
func (a *API) ListRecentLogs(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50")) // Query 参数，默认 50
	if err != nil || limit < 1 {
		limit = 50
	}
	logs, err := a.Logs.ListRecent(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// parseID 把路径里的十进制字符串转成 uint；失败返回 error。
func parseID(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 32) // 基数 10，位宽 32（再转 uint）
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}
