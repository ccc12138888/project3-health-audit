// Package config 负责「从环境变量读取运行参数」。
//
// 是什么：把分散的环境变量收敛成一个 Config 结构体，供 main 与其它包使用。
// 干什么：端口、数据库驱动与 DSN、巡检间隔、HTTP 超时、协程池大小等。
// 怎么用：在启动前设置环境变量（Windows CMD 注意整段 DSN 用引号包住，避免 & 被当成命令分隔符）。
package config

import (
	"os"       // 读环境变量 os.Getenv
	"strconv"  // 把字符串转成整数
	"time"     // 表示「间隔」「超时」的 Duration 类型
)

// Config 保存进程运行所需的全部「可配置项」。
// 各字段含义见 Load() 里每一行的注释。
type Config struct {
	Addr           string        // HTTP 监听地址，如 ":8080"
	DBDriver       string        // "mysql" 或 "sqlite"
	DSN            string        // 数据库连接串（MySQL 见 Go mysql driver 文档）
	CheckInterval  time.Duration // 定时巡检周期间隔（由 CHECK_INTERVAL_SEC 转成）
	RequestTimeout time.Duration // 单次 HTTP 探活请求的超时
	WorkerPoolSize int           // 协程池 worker 数量（同时最多几个巡检）
}

// Load 读取环境变量并填充 Config；若某变量未设置，则使用第二个参数里的默认值。
//
// 环境变量一览（名称 → 作用）：
//   HTTP_ADDR           监听地址，默认 ":8080"
//   DB_DRIVER           mysql 或 sqlite，默认 mysql
//   DB_DSN              连接串；MySQL 默认连本机 health_audit 库（无密码 root，生产务必覆盖）
//   CHECK_INTERVAL_SEC  定时巡检间隔秒数，默认 60
//   REQUEST_TIMEOUT_SEC 单次 HTTP 请求超时秒数，默认 10
//   WORKER_POOL_SIZE    协程池大小，默认 8
func Load() Config {
	// 默认 MySQL DSN：无密码；有密码请不要写死在代码里，用环境变量 DB_DSN。
	defaultMySQL := "root:root@tcp(127.0.0.1:3306)/health_audit?charset=utf8mb4&parseTime=True&loc=Local"
	return Config{
		Addr:           getenv("HTTP_ADDR", ":8080"), // 浏览器访问 http://127.0.0.1:8080
		DBDriver:       getenv("DB_DRIVER", "mysql"),   // 与 internal/database 里 switch 一致
		DSN:            getenv("DB_DSN", defaultMySQL), // 含库名、时区、字符集
		CheckInterval:  durationEnv("CHECK_INTERVAL_SEC", 60*time.Second), // 60 秒一轮定时巡检
		RequestTimeout: durationEnv("REQUEST_TIMEOUT_SEC", 10*time.Second), // 探活超过 10 秒算失败
		WorkerPoolSize: intEnv("WORKER_POOL_SIZE", 8), // 8 个 worker 从队列取任务
	}
}

// getenv 若环境变量 key 存在且非空则返回其值，否则返回 def。
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// intEnv 把环境变量解析为正整数；解析失败或 ≤0 则返回 def。
func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// durationEnv 把环境变量里的「秒数」转成 time.Duration；无效则用 def。
func durationEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second // 整数秒 × 一秒
		}
	}
	return def
}
