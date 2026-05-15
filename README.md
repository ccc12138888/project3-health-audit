# 接口健康巡检微服务 (Health Audit)

基于 **Go + Gin + Gorm** 的独立微服务：登记 HTTP(S) 探活目标，**定时 / 手动**并发巡检，将状态码、耗时与异常标记写入 **MySQL**（可选 SQLite），并提供 **REST API** 与 **内嵌 Web 仪表盘**。

## 功能特性

- **动态配置**：REST 增删改「待巡检 URL」、HTTP 方法、是否启用；无需改代码重启。
- **并发巡检**：有界 **协程池** 控制同时探活数量，避免瞬时 goroutine 爆炸。
- **定时调度**：`robfig/cron` 按可配置间隔全量巡检已启用接口。
- **结果落库**：每次探活写入 `CheckLog`（状态码、耗时、`ok`、`anomaly`、错误信息）。
- **全局暂停**：「结束巡检 / 继续巡检」——暂停后定时与手动探活均跳过（配置保留；状态为进程内存，重启恢复未暂停）。
- **可观测**：异常列表、按接口历史、最近全局日志；控制台对异常输出 `[ALERT]` 行。
- **内嵌前端**：`go embed` 打包单页仪表盘（`/`，含表单登记、一键巡检、暂停控制）。

## 技术栈

| 类别 | 选型 |
|------|------|
| 语言 | Go 1.22+ |
| Web | Gin |
| ORM / 数据库 | Gorm、MySQL（默认）/ SQLite |
| 定时 | robfig/cron/v3（含秒字段） |
| 并发 | 自研 worker 池（channel + 固定 worker） |

## 项目结构（简）

```
cmd/server/main.go          # 入口：配置 → DB → 池 → Service → 定时 → HTTP → 优雅退出
internal/config             # 环境变量
internal/database           # 连接 + AutoMigrate
internal/model              # Endpoint / CheckLog
internal/repository         # DAO
internal/service            # 巡检与配置业务
internal/handler            # HTTP + 嵌入页
internal/router             # 路由
internal/worker             # 协程池
internal/scheduler          # 定时触发 RunBatch
internal/webui              # index.html（embed）
scripts/init_mysql.sql      # 可选：建库 health_audit
```

## 快速开始

### 前置

- Go 1.22+
- MySQL（推荐）或 SQLite

### 建库（MySQL）

在 MySQL 中执行（或导入 `scripts/init_mysql.sql`）：

```sql
CREATE DATABASE IF NOT EXISTS health_audit
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;
```

### 配置环境变量

**Windows CMD**（注意整条 DSN 用引号，避免 `&` 被解析为命令分隔符）：

```cmd
set "DB_DSN=root:你的密码@tcp(127.0.0.1:3306)/health_audit?charset=utf8mb4&parseTime=True&loc=Local"
```

**PowerShell**：

```powershell
$env:DB_DSN = 'root:你的密码@tcp(127.0.0.1:3306)/health_audit?charset=utf8mb4&parseTime=True&loc=Local'
```

### 运行

```bash
cd project3-health-audit
go run ./cmd/server
```

默认监听 `http://127.0.0.1:8080/`。

- **仪表盘**：浏览器打开 `/`
- **存活**：`GET /healthz`

### 编译

```bash
go build -o bin/health-audit ./cmd/server
```

## 环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `HTTP_ADDR` | 监听地址 | `:8080` |
| `DB_DRIVER` | `mysql` 或 `sqlite` | `mysql` |
| `DB_DSN` | 数据库连接串 | 见 `internal/config`（本机 `health_audit`） |
| `CHECK_INTERVAL_SEC` | 定时巡检间隔（秒） | `60` |
| `REQUEST_TIMEOUT_SEC` | 单次 HTTP 探活超时（秒） | `10` |
| `WORKER_POOL_SIZE` | 协程池 worker 数 | `8` |

## API 一览

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/healthz` | 进程存活 |
| `GET` | `/` | 内嵌仪表盘 |
| `POST` | `/api/v1/endpoints` | 新增探活目标 |
| `GET` | `/api/v1/endpoints` | 列表 |
| `GET` | `/api/v1/endpoints/:id` | 单条 |
| `PUT` | `/api/v1/endpoints/:id` | 更新 |
| `DELETE` | `/api/v1/endpoints/:id` | 删除 |
| `GET` | `/api/v1/endpoints/:id/logs` | 该目标历史日志 |
| `POST` | `/api/v1/check/run` | 立即全量巡检（暂停时 `409`） |
| `GET` | `/api/v1/check/state` | 是否暂停 |
| `POST` | `/api/v1/check/pause` | 结束巡检（暂停） |
| `POST` | `/api/v1/check/resume` | 继续巡检 |
| `GET` | `/api/v1/logs/recent?limit=50` | 最近全局日志 |
| `GET` | `/api/v1/logs/anomalies` | 异常记录 |

### 登记示例

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/endpoints \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"示例\",\"url\":\"https://httpbin.org/status/200\",\"method\":\"GET\"}"
```

### 手动巡检

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/check/run
```

## 探活语义（当前实现）

- 使用配置的 HTTP 方法（默认 `GET`），**无请求体**、无自定义 Header。
- **2xx** 记为成功（`ok=true`）；**非 2xx** 或 **≥500** 或网络错误会标记 **`anomaly=true`**（规则见 `internal/service/health_check.go`）。
- **不跟随 HTTP 重定向**（保留首个响应状态码）。

## 停止服务

在运行进程的终端中 **`Ctrl+C`**：先优雅关闭 HTTP，再停止定时任务并关闭协程池。

## 模块路径

默认 Go module 为 `github.com/example/health-audit`。若你 fork 到自己的仓库，请全局替换为你的 module 路径并调整 import。

## License

MIT（可按需修改或删除本节。）
