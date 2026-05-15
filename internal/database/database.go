// Package database 负责「打开数据库连接 + 根据 model 自动建表/迁移」。
//
// 是什么：对 Gorm 的一层薄封装。
// 干什么：按配置选择 MySQL 或 SQLite，执行 AutoMigrate。
// 怎么用：main 里 database.Open(cfg)，cfg 来自 config.Load()。
package database

import (
	"fmt" // 格式化错误信息

	"github.com/example/health-audit/internal/config" // 取 DBDriver、DSN
	"github.com/example/health-audit/internal/model"  // 要迁移的表对应结构体

	"gorm.io/driver/mysql"  // MySQL 驱动（底层 github.com/go-sql-driver/mysql）
	"gorm.io/driver/sqlite" // SQLite 驱动（文件库）
	"gorm.io/gorm"          // ORM 核心
	"gorm.io/gorm/logger"   // Gorm 日志级别
)

// Open 根据 cfg 连接数据库，并对 Endpoint、CheckLog 两张「逻辑表」做 AutoMigrate。
//
// 返回 *gorm.DB 供 repository 注入；若驱动不支持或连接失败则返回 error。
func Open(cfg config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector // Dialector 是 Gorm 对「哪种数据库」的抽象接口
	switch cfg.DBDriver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN) // DSN 格式见 MySQL driver 文档
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN) // 常为文件路径如 "health.db"
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER: %s (use sqlite or mysql)", cfg.DBDriver)
	}

	// Open 真正建立连接池；Config 里 Logger 设为 Warn，减少 SQL 刷屏。
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	// AutoMigrate：表不存在则创建；若 struct tag 变化会尽量 ALTER（注意生产环境迁移策略）。
	if err := db.AutoMigrate(&model.Endpoint{}, &model.CheckLog{}); err != nil {
		return nil, err
	}
	return db, nil
}
