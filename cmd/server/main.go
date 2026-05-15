// Package main 是程序入口：负责「组装依赖 → 启动 HTTP → 收到退出信号后优雅关机」。
//
// 怎么用（本机）：
//  1）配置 MySQL：环境变量 DB_DSN（CMD 示例：set "DB_DSN=root:密码@tcp(127.0.0.1:3306)/health_audit?charset=utf8mb4&parseTime=True&loc=Local"）
//  2）在项目根目录执行：go run ./cmd/server
//  3）浏览器打开 http://127.0.0.1:8080/ 看仪表盘；API 在 /api/v1/...
//  4）Ctrl+C 停止服务
package main

import (
	"context"   // 用于控制「应用级生命周期」与 HTTP Shutdown 的超时
	"log"       // 标准日志，打印到控制台
	"net/http"  // 标准 HTTP 服务器，包装 Gin 引擎
	"os"        // 操作系统信号（Ctrl+C）
	"os/signal" // 把 SIGINT/SIGTERM 转成 channel 接收
	"syscall"   // 具体信号常量
	"time"      // 超时、间隔

	// 以下为本项目内部包（按启动顺序阅读最容易懂）
	"github.com/example/health-audit/internal/config"     // 读环境变量，得到端口、数据库、间隔等
	"github.com/example/health-audit/internal/database" // 连接数据库并 AutoMigrate 建表
	"github.com/example/health-audit/internal/handler"  // HTTP 处理函数（Controller 层）
	"github.com/example/health-audit/internal/repository" // 数据库 CRUD（DAO 层）
	"github.com/example/health-audit/internal/router"   // 把 URL 绑到 handler
	"github.com/example/health-audit/internal/scheduler" // 定时触发巡检
	"github.com/example/health-audit/internal/service"  // 业务：巡检逻辑、接口配置业务
	"github.com/example/health-audit/internal/worker"   // 协程池，限制并发巡检数

	"github.com/gin-gonic/gin" // Web 框架，提供路由、JSON、中间件
)

// main 是唯一入口函数；Go 程序从这里的第一个语句开始执行。
func main() {
	// Load 读取环境变量（若未设置则用默认值），得到 Config 结构体。
	cfg := config.Load()

	// Open 根据 cfg 连接 MySQL/SQLite，并对 model 做自动建表/迁移。
	db, err := database.Open(cfg)
	if err != nil {
		// Fatalf 会打印错误并 os.Exit(1)，后面代码不再执行。
		log.Fatalf("database: %v", err)
	}

	// NewPool 创建「有界 worker 池」：同时最多 cfg.WorkerPoolSize 个巡检在跑。
	pool := worker.NewPool(cfg.WorkerPoolSize)
	// Start 启动固定数量的 goroutine，从 channel 里取 Job 执行（见 internal/worker）。
	pool.Start()

	// Repository 封装 Gorm：只关心 SQL/表，不写业务规则。
	epRepo := repository.NewEndpointRepository(db)   // 表：待巡检接口配置
	logRepo := repository.NewCheckLogRepository(db) // 表：每次巡检一条日志

	// Service 写业务：HealthCheckService 会发 HTTP、写 CheckLog；EndpointService 管 Endpoint CRUD。
	healthSvc := service.NewHealthCheckService(epRepo, logRepo, pool, cfg.RequestTimeout)
	epSvc := service.NewEndpointService(epRepo)

	// appCtx 用于通知「应用要退出了」；cancel() 会关闭 appCtx，让监听它的 goroutine 结束（例如停 cron）。
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel() // main 返回前一定会调用 cancel，释放 context 资源（即使后面正常走完也会执行）

	// 定时器：每隔 intervalSec 秒调用一次 healthSvc.RunBatch（全量巡检所有已启用接口）。
	sched := scheduler.New(healthSvc)
	intervalSec := int(cfg.CheckInterval / time.Second) // Duration 转成整数秒
	if intervalSec < 1 {
		intervalSec = 1 // 防止除出来为 0 导致 cron 异常
	}
	sched.Start(appCtx, intervalSec)

	// Gin 设为 ReleaseMode，减少调试输出；开发时若想更详细可改为 gin.DebugMode。
	gin.SetMode(gin.ReleaseMode)
	// New 创建引擎；不用 Default() 以免自带 Logger 与我们下面手动 Use 重复。
	engine := gin.New()
	// Recovery 捕获 panic 防止整个进程崩溃；Logger 打印每个 HTTP 请求一行访问日志。
	engine.Use(gin.Recovery(), gin.Logger())

	// API 结构体把「各层依赖」注入到 handler：handler 里通过 a.Endpoints / a.Health 调用。
	api := &handler.API{
		Endpoints: epSvc,
		Health:    healthSvc,
		Logs:      logRepo,
	}
	// Mount 注册所有路由：/healthz、/api/v1/...、首页仪表盘 / 。
	router.Mount(engine, api)

	// http.Server 是标准库服务器；Handler 指向 Gin（Gin 实现了 http.Handler 接口）。
	srv := &http.Server{
		Addr:              cfg.Addr,              // 监听地址，默认 ":8080"
		Handler:           engine,                // 所有 HTTP 交给 Gin 分发
		ReadHeaderTimeout: 10 * time.Second,      // 防止慢客户端占连接（只限制读请求头阶段）
	}

	// 在单独 goroutine 里 ListenAndServe，避免阻塞 main，这样 main 才能去 <-sig 等信号。
	go func() {
		log.Printf("listening on %s", cfg.Addr)
		// ListenAndServe 一般一直阻塞；Shutdown 后会返回 http.ErrServerClosed，这是正常退出，不 Fatalf。
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// 建一个缓冲为 1 的 channel，只关心「收到一次退出信号」。
	sig := make(chan os.Signal, 1)
	// Notify 把 Ctrl+C（SIGINT）和 kill（SIGTERM）转发到 sig channel。
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig // 阻塞直到用户按 Ctrl+C 或进程被结束

	// Shutdown：停止接受新连接，并等待已有请求处理完或超时（30 秒内）。
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}

	// cancel() 通知 scheduler 里监听 appCtx 的 goroutine：该停了（cron.Stop）。
	cancel()
	// Close 关闭任务 channel 并等 worker 把队列里剩余 Job 跑完（见 worker.Pool）。
	pool.Close()
}
