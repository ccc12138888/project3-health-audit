// Package worker：有界协程池。
//
// 是什么：固定 N 个 worker goroutine + 带缓冲的 Job 队列。
// 干什么：限制「同时有多少个巡检」在跑，避免一次巡检上千个 URL 时瞬间创建上千 goroutine。
// 怎么用：main 里 NewPool(n).Start()；业务里 pool.Submit(func(){ ... })；退出前 pool.Close()。
package worker

import (
	"sync" // WaitGroup、Mutex、Once
)

// Job 表示「一个要在池里执行的任务」。
// 类型是「无参无返回值函数」：需要传参时用闭包捕获外层变量。
type Job func()

// Pool 结构体保存池的配置与运行时状态。
type Pool struct {
	size   int           // worker 数量（同时执行 Job 的上限）
	jobs   chan Job      // 任务队列；带缓冲，Submit 在队列满时会阻塞
	wg     sync.WaitGroup // 等待所有 worker goroutine 退出
	once   sync.Once      // 保证 Start 只真正执行一次
	mu     sync.Mutex     // 保护 closed 字段的并发读写
	closed bool           // 为 true 时不再接受 Submit，Close 会关 channel
}

// NewPool 创建池；size 表示 worker 个数，至少为 1。
func NewPool(size int) *Pool {
	if size < 1 {
		size = 1
	}
	buf := size * 4 // 缓冲约为 worker 数的 4 倍，缓冲突发提交
	if buf < 16 {
		buf = 16
	}
	return &Pool{
		size: size,
		jobs: make(chan Job, buf), // 有缓冲 channel：cap(buf)
	}
}

// Start 启动 size 个 goroutine，每个都从 p.jobs 里 range 取任务执行。
// sync.Once 保证多次调用 Start 也只有第一次会起 worker。
func (p *Pool) Start() {
	p.once.Do(func() {
		for i := 0; i < p.size; i++ { // 起 size 个 worker
			p.wg.Add(1) // 每个 worker 计数 +1，Close 时 Wait 等它们结束
			go func() {
				defer p.wg.Done() // worker 退出时计数 -1
				// range 在 channel close 且排空后自动结束循环
				for job := range p.jobs {
					if job != nil { // 防御 nil 任务
						job() // 执行具体逻辑（例如一次 HTTP 巡检）
					}
				}
			}()
		}
	})
}

// Submit 把一个 Job 放进队列；若池已 Close 则返回 ErrPoolClosed。
// 注意：向已关闭的 channel 发送会 panic，故先用 closed 标志位判断。
func (p *Pool) Submit(job Job) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrPoolClosed
	}
	p.mu.Unlock()
	p.jobs <- job // 可能阻塞，直到某个 worker 取走
	return nil
}

// Close 标记关闭并 close(jobs)，worker 会消费完队列中剩余任务后退出，最后 wg.Wait 返回。
func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed { // 幂等：重复 Close 直接返回
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()
	close(p.jobs) // 关闭后 range 结束，所有 worker goroutine 返回
	p.wg.Wait()   // 阻塞直到所有 worker Done
}
