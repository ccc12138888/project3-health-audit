// Package scheduler：用 cron 定时触发「全量巡检」。
//
// 是什么：robfig/cron 的薄封装。
// 干什么：每 N 秒调用一次 HealthCheckService.RunBatch。
// 怎么用：main 里 sched.Start(appCtx, intervalSec)；退出时 cancel appCtx 会停 cron。
package scheduler

import (
	"context" // 用于在进程退出时停止 cron
	"errors"  // 判断 ErrCheckPaused，避免暂停时打错误日志
	"log"     // 定时任务里若 RunBatch 真失败则打印

	"github.com/example/health-audit/internal/service"

	"github.com/robfig/cron/v3" // 定时任务库
)

// Runner 持有 cron 实例与 HealthCheckService 指针。
type Runner struct {
	cron   *cron.Cron
	health *service.HealthCheckService
}

// New 创建 Runner；WithSeconds 表示 cron 表达式支持「秒」字段（6 段）。
func New(health *service.HealthCheckService) *Runner {
	return &Runner{
		cron:   cron.New(cron.WithSeconds()),
		health: health,
	}
}

// Start 注册周期任务并启动 cron；intervalSec 为两次 RunBatch 之间的间隔秒数。
func (r *Runner) Start(ctx context.Context, intervalSec int) {
	if intervalSec < 1 {
		intervalSec = 60
	}
	spec := formatEverySeconds(intervalSec) // 转成 cron 字符串
	_, err := r.cron.AddFunc(spec, func() {
		// 定时任务用 Background，避免与用户 HTTP 请求的 ctx 生命周期绑死
		if err := r.health.RunBatch(context.Background()); err != nil {
			if errors.Is(err, service.ErrCheckPaused) {
				return // 用户主动暂停：静默跳过，不打 error 日志
			}
			log.Printf("scheduled health check: %v", err)
		}
	})
	if err != nil {
		log.Printf("cron add: %v", err)
		return
	}
	r.cron.Start()

	// 监听 ctx 取消：main 在 Shutdown HTTP 后会 cancel，这里负责停 cron
	go func() {
		<-ctx.Done()
		stopCtx := r.cron.Stop() // 返回的 context 在 cron 完全停止后 Done
		<-stopCtx.Done()
	}()
}

// formatEverySeconds 生成「每 sec 秒执行一次」的 cron 表达式（秒 分 时 日 月 周）。
func formatEverySeconds(sec int) string {
	return "*/" + itoa(sec) + " * * * * *"
}

// itoa 把正整数转成字符串（无额外依赖）；cron 里用数字字符串即可。
func itoa(n int) string {
	if n < 1 {
		return "1"
	}
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	s := ""
	for n > 0 {
		s = string(digits[n%10]) + s
		n /= 10
	}
	return s
}
