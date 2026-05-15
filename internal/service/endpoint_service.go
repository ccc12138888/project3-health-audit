// Package service：业务层。本文件封装「接口配置 Endpoint」的业务操作。
//
// 是什么：介于 handler 与 repository 之间，可在此加校验、默认值、组合多个 repo（当前较薄）。
// 干什么：创建时若 Method 为空则默认 GET。
// 怎么用：handler 调 a.Endpoints.Create/Get/List...
package service

import (
	"github.com/example/health-audit/internal/model"
	"github.com/example/health-audit/internal/repository"
)

// EndpointService 依赖 EndpointRepository。
type EndpointService struct {
	repo *repository.EndpointRepository
}

// NewEndpointService 构造函数。
func NewEndpointService(repo *repository.EndpointRepository) *EndpointService {
	return &EndpointService{repo: repo}
}

// Create 新建一条接口配置；若 Method 为空则设为 GET 再入库。
func (s *EndpointService) Create(e *model.Endpoint) error {
	if e.Method == "" {
		e.Method = "GET"
	}
	return s.repo.Create(e)
}

// Update 全量更新一行（ID 需在 e 里已设置）。
func (s *EndpointService) Update(e *model.Endpoint) error {
	return s.repo.Update(e)
}

// Delete 按 id 删除。
func (s *EndpointService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// Get 按 id 查询单条。
func (s *EndpointService) Get(id uint) (*model.Endpoint, error) {
	return s.repo.GetByID(id)
}

// List 列出全部配置（含停用的）。
func (s *EndpointService) List() ([]model.Endpoint, error) {
	return s.repo.ListAll()
}
