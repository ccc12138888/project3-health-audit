// Package repository：CheckLog 表的读写（巡检结果日志）。
package repository

import (
	"github.com/example/health-audit/internal/model"

	"gorm.io/gorm"
)

// CheckLogRepository 巡检日志仓储。
type CheckLogRepository struct {
	db *gorm.DB
}

// NewCheckLogRepository 构造函数。
func NewCheckLogRepository(db *gorm.DB) *CheckLogRepository {
	return &CheckLogRepository{db: db}
}

// Create 插入一条巡检结果。
func (r *CheckLogRepository) Create(log *model.CheckLog) error {
	return r.db.Create(log).Error
}

// ListByEndpoint 查某个 endpoint_id 的最近 limit 条日志，按 id 倒序（新的在前）。
func (r *CheckLogRepository) ListByEndpoint(endpointID uint, limit int) ([]model.CheckLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var logs []model.CheckLog
	err := r.db.Where("endpoint_id = ?", endpointID).
		Order("id desc").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// ListAnomalies 只查 anomaly=true 的日志，用于「异常列表」接口与仪表盘。
func (r *CheckLogRepository) ListAnomalies(limit int) ([]model.CheckLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var logs []model.CheckLog
	err := r.db.Where("anomaly = ?", true).
		Order("id desc").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// ListRecent 不区分接口，按全局 id 倒序取最近 limit 条，用于仪表盘「最近巡检」。
func (r *CheckLogRepository) ListRecent(limit int) ([]model.CheckLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 { // 防止一次查太大拖慢数据库
		limit = 500
	}
	var logs []model.CheckLog
	err := r.db.Order("id desc").Limit(limit).Find(&logs).Error
	return logs, err
}
