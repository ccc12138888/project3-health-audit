// Package model 定义「与数据库表对应的结构体」以及 JSON 字段名。
//
// 是什么：Gorm 用 struct + tag 映射表列；Gin 用 json tag 输出 API。
// 干什么：描述「要巡检谁」和「每次巡检记什么」。
// 怎么用：repository 里 Create/Find；handler 里绑定 JSON。
package model

import "time"

// Endpoint 表示「一条待巡检的 HTTP 接口配置」，对应表 endpoints（Gorm 默认复数表名）。
//
// 字段说明：
//   ID        主键，自增；POST 创建成功后由数据库生成。
//   Name      给人看的名称，便于在仪表盘区分。
//   URL       完整 http(s) 地址；uniqueIndex 表示同一 URL 不能登记两行。
//   Method    HTTP 方法，如 GET；空时业务层会默认 GET。
//   Enabled   是否参与巡检；false 时 RunBatch 不会请求它。
//   CreatedAt/UpdatedAt Gorm 自动维护的时间戳。
type Endpoint struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	URL       string    `gorm:"size:512;not null;uniqueIndex" json:"url"`
	Method    string    `gorm:"size:16;default:GET" json:"method"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CheckLog 表示「对某个 Endpoint 做了一次 HTTP 探活」的一条结果，对应表 check_logs。
//
// 字段说明：
//   EndpointID  外键，指向哪条 Endpoint。
//   StatusCode  HTTP 响应状态码；网络失败时多为 0。
//   LatencyMs   从发请求到收到响应头的大致耗时（毫秒）。
//   OK          业务上是否算成功（本项目中为 2xx）。
//   ErrorMessage 若连接失败等，这里存错误文本。
//   Anomaly     是否标为异常（非 2xx、5xx、或网络错误等）。
//   CreatedAt   这条日志插入时间。
type CheckLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	EndpointID   uint      `gorm:"index;not null" json:"endpoint_id"`
	StatusCode   int       `json:"status_code"`
	LatencyMs    int64     `json:"latency_ms"`
	OK           bool      `gorm:"index" json:"ok"`
	ErrorMessage string    `gorm:"size:1024" json:"error_message"`
	Anomaly      bool      `gorm:"index;default:false" json:"anomaly"`
	CreatedAt    time.Time `json:"created_at"`
}
