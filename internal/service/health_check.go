// Package service：核心业务。本文件实现「HTTP 探活巡检」与「全局暂停」。
//
// 是什么：HealthCheckService 用标准库 http.Client 请求每个已启用 Endpoint，并把结果写入 CheckLog。
// 干什么：checkOne 测单个 URL；RunBatch 测一批；Pause/Resume 控制是否执行探活。
// 怎么用：handler 调 RunBatch；scheduler 定时调 RunBatch；网页「结束巡检」调 Pause。
package service

import (
	"context"   // 控制单次请求超时、取消（随 HTTP 请求走）
	"errors"    // 定义 ErrCheckPaused
	"io"        // Discard 丢弃响应体
	"log"       // 异常时打 [ALERT] 日志
	"net/http"  // 发 HTTP 请求
	"strings"   // 处理 Method 大小写、空格
	"sync"      // WaitGroup 等一批任务结束；Mutex 保护 firstErr
	"sync/atomic" // 暂停标志用原子布尔，避免加锁
	"time"      // 计时耗时

	"github.com/example/health-audit/internal/model"
	"github.com/example/health-audit/internal/repository"
	"github.com/example/health-audit/internal/worker"
)

// ErrCheckPaused 表示用户点了「结束巡检」：RunBatch 应立刻返回，不访问任何 URL。
// handler 与 scheduler 用 errors.Is(err, ErrCheckPaused) 区分「正常暂停」与「真错误」。
var ErrCheckPaused = errors.New("inspection paused")

// HealthCheckService 聚合：接口列表仓储、日志仓储、协程池、HTTP 客户端、暂停标志。
type HealthCheckService struct {
	endpoints *repository.EndpointRepository // 读哪些 URL 要测
	logs      *repository.CheckLogRepository // 写每次结果
	pool      *worker.Pool                   // 限制并发
	client    *http.Client                   // 实际发 HTTP
	paused    atomic.Bool                    // true=暂停探活（内存态，重启丢失）
}

// NewHealthCheckService 构造 HTTP 客户端：整次请求超时为 timeout；不重定向跟随（见 CheckRedirect）。
func NewHealthCheckService(
	ep *repository.EndpointRepository, // 接口配置仓库（查要巡检哪些URL）
	lg *repository.CheckLogRepository, // 巡检日志仓库（存巡检结果）
	pool *worker.Pool, // 协程池（控制并发，防止炸服务器）
	timeout time.Duration,  // HTTP请求超时时间
) *HealthCheckService {
	return &HealthCheckService{
		endpoints: ep,
		logs:      lg,
		pool:      pool,
		client: &http.Client{
			Timeout: timeout, // 包含连接+TLS+读响应头尾的整体上限
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				// ErrUseLastResponse 表示「不要自动跟 301/302」，保留第一个响应的状态码
				return http.ErrUseLastResponse
			},
		},
	}
}

// checkOne 对单个 Endpoint 发一次 HTTP，返回一条 CheckLog（此时尚未写入数据库）。
func (s *HealthCheckService) checkOne(ctx context.Context, ep model.Endpoint) model.CheckLog {
	// 规范化方法名：去空格、转大写；空则用 GET
	method := strings.ToUpper(strings.TrimSpace(ep.Method))
	if method == "" {
		method = http.MethodGet
	}
	start := time.Now() // 记录开始时间，用于算 LatencyMs

	// NewRequestWithContext：若 ctx 取消（例如客户端断开），请求可中止
	req, err := http.NewRequestWithContext(ctx, method, ep.URL, nil)
	if err != nil {
		// URL 非法等，构造请求就失败
		return model.CheckLog{
			EndpointID:   ep.ID,
			StatusCode:   0,
			LatencyMs:    time.Since(start).Milliseconds(),
			OK:           false,
			ErrorMessage: err.Error(),
			Anomaly:      true,
		}
	}

	// Do 真正发起网络请求
	resp, err := s.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		// 超时、连接拒绝、DNS 失败等
		return model.CheckLog{
			EndpointID:   ep.ID,
			StatusCode:   0,
			LatencyMs:    latency,
			OK:           false,
			ErrorMessage: err.Error(),
			Anomaly:      true,
		}
	}
	defer resp.Body.Close() // 必须关 Body，连接才能复用
	// 读最多 512KB 到垃圾桶，避免大页面占内存；不关心内容是否正确
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512<<10))

	// 2xx 认为 OK；非 2xx 或 5xx 标异常（业务可再调规则）
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	anomaly := !ok || resp.StatusCode >= 500

	return model.CheckLog{
		EndpointID:   ep.ID,
		StatusCode:   resp.StatusCode,
		LatencyMs:    latency,
		OK:           ok,
		ErrorMessage: "",
		Anomaly:      anomaly,
	}
}

// Pause 设置暂停标志为 true：之后 RunBatch 直接返回 ErrCheckPaused。
func (s *HealthCheckService) Pause() { s.paused.Store(true) }

// Resume 清除暂停标志。
func (s *HealthCheckService) Resume() { s.paused.Store(false) }

// Paused 查询当前是否暂停（给 GET /check/state 用）。
func (s *HealthCheckService) Paused() bool { return s.paused.Load() }

// RunBatch：查出所有已启用 Endpoint，对每个提交到协程池里执行 checkOne + 写库。
//
// ctx：用于取消「尚未提交到池里的循环」；已提交的任务仍会跑完（闭包捕获同一 ctx，请求也可能随 ctx 取消）。
// 返回：nil 表示本轮无错误或未启用接口；ErrCheckPaused 表示暂停；其它 error 多为数据库错误。
func (s *HealthCheckService) RunBatch(ctx context.Context) error {
	if s.Paused() {
		return ErrCheckPaused
	}
	list, err := s.endpoints.ListEnabled()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return nil // 没有要测的，直接成功
	}

	var (
		wg       sync.WaitGroup // 等待本批所有 pool 任务执行完
		mu       sync.Mutex     // 保护 firstErr 的并发写
		firstErr error          // 记录第一个写库错误（若有）
	)

	for _, ep := range list {
		// 若调用方取消 ctx，则不再投递新任务，等已投递的跑完后返回 ctx.Err()
		if ctx.Err() != nil {
			wg.Wait()
			if firstErr != nil {
				return firstErr
			}
			return ctx.Err()
		}
		ep := ep // 闭包经典写法：复制循环变量，避免 goroutine 里全是最后一个 ep
		wg.Add(1)
		if err := s.pool.Submit(func() {
			defer wg.Done()
			entry := s.checkOne(ctx, ep) // 真正 HTTP
			if err := s.logs.Create(&entry); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err // 只记第一个错误
				}
				mu.Unlock()
				return
			}
			if entry.Anomaly {
				// 控制台「报警」：运维可把日志接到告警系统
				log.Printf("[ALERT] anomaly endpoint_id=%d url=%s status=%d latency_ms=%d err=%q",
					entry.EndpointID, ep.URL, entry.StatusCode, entry.LatencyMs, entry.ErrorMessage)
			}
		}); err != nil {
			wg.Done() // Submit 失败则从未开始执行，抵消上面的 Add(1)
			return err
		}
	}

	wg.Wait() // 阻塞直到本批任务全部 Done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr // 可能为 nil
}
