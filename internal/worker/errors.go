// Package worker 提供「有界并发」执行异步任务的能力（协程池）。
// 本文件仅定义池在关闭后仍 Submit 时会返回的错误。
package worker

import "errors"

// ErrPoolClosed 在 Pool.Close() 之后再调用 Submit 时返回。
// 含义：不要再往池里丢任务了，worker 即将或已经退出。
var ErrPoolClosed = errors.New("worker pool closed")
