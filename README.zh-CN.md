# Go-Diagnoser-Engine

[English](README.md) | **[简体中文](README.zh-CN.md)** | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Português](README.pt.md) | [Русский](README.ru.md)

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: scaffold](https://img.shields.io/badge/status-scaffold-orange)


**高并发后端诊断引擎**——一个 Go 微服务，通过 REST 接收诊断任务，在有界 worker 池上并发执行探针，并返回结构化结果。

> **项目状态：脚手架（诚实版）。**完整请求链路已端到端跑通——`POST /api/v1/diagnose` → worker 池 → `GET /api/v1/jobs/{id}`——但探针本身是**模拟占位**（每个响应中都明确标注 `SIMULATED`）。真正的诊断规则是下一步，代码中以 `TODO(ezra)` 标记。这里没有任何东西假装在探测真实硬件。

## 问题

后端和 infra 团队需要一种快速、统一的方式，对成群节点运行健康／诊断检查：提交一个目标，拿到带延迟的结构化通过／失败结果，且不阻塞调用方。本服务就是这套工作流的执行骨干。

## 架构

```
                    +------------------+
                    |   REST API       |
  POST /diagnose -->|  (net/http)      |--+
                    +------------------+  |
                                          v
                                   +-------------+
                                   |  Job queue  |  bounded channel
                                   | (backpressure: 503 when full)
                                   +-------------+
                                          |
                 +------------+-----------+------------+-----------+
                 |            |           |            |           |
              worker 1     worker 2    worker 3  ...       worker N
                 |            |           |            |
                 +------ checks fan out per job ------+
                 |   connectivity | disk_usage | service_health |
                 +----------------+----------------------------+
                                          |
                                   +-------------+
                                   |    Store    |  in-memory (Postgres/Redis TODO)
                                   +-------------+
                                          |
  GET /jobs/{id} <-- structured CheckResult[] with latency
```

并发模型：

- **固定 worker 池**（默认 `runtime.NumCPU()`）：goroutine 常驻，不为每个请求新建 goroutine。
- **有界队列**：通过快速失败的 503 实现背压，而不是无界增长。
- **单任务扇出**：一个任务的各项检查跑在短生命周期的 goroutine 上，worker 在它们 join 之后才继续；每项检查有各自 30 秒超时。
- **优雅停机**：`SIGINT/SIGTERM` → 停止接收 → 排空在途任务 → 退出。无 goroutine 泄漏。

## 技术栈

- **Go 1.24**，仅标准库（零外部依赖）
- 基于 `net/http` 的 REST API（Go 1.22+ 的 method+pattern 路由）
- Worker 池 ＋ channel ＋ `sync.WaitGroup` 并发
- Docker 多阶段构建，`docker-compose` 一键运行

## 快速上手

```bash
# 构建并运行
go build -o server ./cmd/server
./server                  # listens on :8080

# 或用 docker
docker compose up --build
```

通过环境变量配置：`PORT`（默认 8080）、`WORKERS`（默认 NumCPU）、`QUEUE_SIZE`（默认 100）。

## API

```bash
# 提交一个诊断任务（运行所有已注册检查）
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# 轮询结果
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# 只运行选定的检查
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["connectivity"]}'

# 健康检查
curl -s localhost:8080/healthz
```

## 项目布局

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## 路线图

- [x] 脚手架：REST API → worker 池 → 模拟结果，端到端跑通
- [ ] **真实诊断规则**（`internal/engine/checks.go`，`TODO(ezra)`）：ICMP/TCP 连通性探针、磁盘用量采集、`/healthz` 查询，再到 PCIe/XID 分诊分类——让本项目成为作品集亮点的领域逻辑
- [ ] 在现有 `Store` API 后面接持久化任务存储（PostgreSQL 或 Redis）
- [ ] 鉴权（API key）与按租户限流
- [ ] Prometheus 指标（`jobs_total`、`check_latency_seconds`、队列深度）
- [ ] 长任务通过 Server-Sent Events 流式返回结果

## 许可证

MIT —— 见 [LICENSE](LICENSE)。

由 Guangyi "Ezra" Zhao 构建，作为求职作品集项目。
