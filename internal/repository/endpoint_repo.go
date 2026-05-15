// Package repository：数据访问层（DAO），只负责「用 Gorm 读写表」，不写 HTTP 与业务规则。
//
// 本文件：表「待巡检接口配置」Endpoint 的增删改查。
package repository

import (
	"github.com/example/health-audit/internal/model" // 表对应的 struct

	"gorm.io/gorm" // 数据库句柄类型
)

// EndpointRepository 持有 *gorm.DB，所有方法都用 r.db 操作。
type EndpointRepository struct {
	db *gorm.DB
}

// NewEndpointRepository 构造函数：谁要用仓储，就把 db 传进来。
func NewEndpointRepository(db *gorm.DB) *EndpointRepository {
	return &EndpointRepository{db: db}
}

// Create 插入一行 Endpoint；e.ID 若为零值则由数据库自增生成。
func (r *EndpointRepository) Create(e *model.Endpoint) error {
	return r.db.Create(e).Error // Error 非 nil 表示违反唯一约束等
}

// Update 按主键更新整行（Save 会更新所有字段，注意零值也会写入）。
func (r *EndpointRepository) Update(e *model.Endpoint) error {
	return r.db.Save(e).Error
}

// Delete 按主键删除一行。
func (r *EndpointRepository) Delete(id uint) error {
	return r.db.Delete(&model.Endpoint{}, id).Error
}

// GetByID 按主键查一行；找不到时 err 为 gorm.ErrRecordNotFound。
func (r *EndpointRepository) GetByID(id uint) (*model.Endpoint, error) {
	var e model.Endpoint
	if err := r.db.First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// ListAll 返回全部接口配置，按 id 升序（稳定顺序）。
func (r *EndpointRepository) ListAll() ([]model.Endpoint, error) {
	var list []model.Endpoint
	err := r.db.Order("id asc").Find(&list).Error
	return list, err
}

// ListEnabled 只返回 enabled=true 的接口，供巡检 RunBatch 使用。
func (r *EndpointRepository) ListEnabled() ([]model.Endpoint, error) {
	var list []model.Endpoint
	err := r.db.Where("enabled = ?", true).Order("id asc").Find(&list).Error
	return list, err
}
