# Go-Diagnoser-Engine

**[English](README.md)** | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Português](README.pt.md) | [Русский](README.ru.md)

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: v1.0](https://img.shields.io/badge/status-v1.0-green)


**High-Concurrency Backend Diagnostics Engine** — a Go microservice that accepts
diagnostic jobs over REST, executes probes concurrently on a bounded worker
pool, and returns structured results. Ships with Prometheus metrics,
deterministic simulated probes, and one real probe (`host_load`).

> **Project status: v1.0 (honest edition).** The full request path works
> end to end — `POST /api/v1/diagnose` → worker pool → `GET /api/v1/jobs/{id}` —
> with Prometheus metrics on `/metrics`. The network/disk/service probes are
> **simulated** (deterministic per target, clearly labeled `SIMULATED` in every
> response); `host_load` reads the real `/proc/loadavg` on Linux. Real
> diagnostic rules (TCP dial, Statfs, /healthz query) are the next step and
> are marked with `TODO(ezra)` in the code.

## Problem

Backend and infra teams need a fast, uniform way to run health/diagnostic
checks against fleets of nodes: submit a target, get structured pass/fail
results with latency, without blocking the caller. This service is the
execution backbone for that workflow.

## Architecture

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
                 | connectivity | disk_usage | service_health | host_load |
                 +----------------+----------------------------+
                                          |
                                   +-------------+
                                   |    Store    |  in-memory (Postgres/Redis TODO)
                                   +-------------+
                                          |
  GET /jobs/{id} <-- structured CheckResult[] with latency

  GET /metrics   <-- Prometheus: jobs, queue depth, per-check latency/pass-fail
```

Concurrency model:

- **Fixed worker pool** (default `runtime.NumCPU()`): long-lived goroutines,
  no per-request goroutine spawning.
- **Bounded queue**: backpressure via fail-fast 503 instead of unbounded growth.
- **Per-job fan-out**: a job's checks run on short-lived goroutines and are
  joined before the worker proceeds; each check has its own 30s timeout.
- **Graceful shutdown**: `SIGINT/SIGTERM` → stop accepting → drain in-flight →
  exit. No goroutine leaks.
- **Deterministic simulation**: simulated probes seed from `hash(target, check)`,
  so the same target always yields the same outcome — demos are reproducible
  and retries are comparable.

## Tech stack

- **Go 1.24**, standard library only (zero external dependencies)
- REST API on `net/http` (Go 1.22+ method+pattern routing)
- Worker pool + channels + `sync.WaitGroup` concurrency
- Prometheus text format rendered by hand (`internal/metrics`, no client lib)
- Docker multi-stage build, `docker-compose` for one-command run

## Quickstart

```bash
# build & run
go build -o server ./cmd/server
./server                  # listens on :8080

# or with docker
docker compose up --build
```

Config via environment: `PORT` (default 8080), `WORKERS` (default NumCPU),
`QUEUE_SIZE` (default 100).

## API

```bash
# submit a diagnostic job (all registered checks)
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# poll for the result
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# run only selected checks
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["host_load"]}'

# health
curl -s localhost:8080/healthz
# -> {"status":"ok"}

# prometheus metrics
curl -s localhost:8080/metrics
# -> diagnoser_jobs_submitted_total 31
#    diagnoser_queue_depth 0
#    diagnoser_check_latency_ms_avg{check="connectivity"} 155
#    ...
```

## Load testing

The point of the worker pool + bounded queue is surviving bursts. Verify it:

```bash
# 200 concurrent clients, 30s — watch for 503s (backpressure) vs 5xx (bugs)
cat > /tmp/wrk-post.lua <<'EOF'
wrk.method = "POST"
wrk.headers["Content-Type"] = "application/json"
wrk.body = '{"target":"load-node"}'
EOF
wrk -t4 -c200 -d30s -s /tmp/wrk-post.lua http://localhost:8080/api/v1/diagnose

# ...or with ApacheBench (10k requests, 100 concurrent):
ab -n 10000 -c 100 -p body.json -T 'application/json' \
   http://localhost:8080/api/v1/diagnose   # body.json: {"target":"load-node"}

# While the load runs, watch the service explain itself:
watch -n1 'curl -s localhost:8080/metrics | grep -E "diagnoser_(jobs|queue)" | grep -v "^#"'
```

What to look for: `diagnoser_queue_depth` rising under burst (expected),
`diagnoser_jobs_failed_total` staying 0 (no bugs), p99 latency stable as
concurrency grows. If the queue saturates, the API returns 503 with
`{"error":"job queue full, try again later"}` — that is backpressure working
as designed, not an error to "fix" by growing the queue unboundedly.

## Project layout

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz, metrics)
internal/engine/     worker pool + check registry (+ tests)
internal/metrics/    Prometheus-format registry (stdlib only)
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## Roadmap

- [x] v1.0: REST API → worker pool → results, metrics, deterministic sim probes
- [ ] **Real diagnostic rules** (`internal/engine/checks.go`, `TODO(ezra)`):
      TCP dial probe, `syscall.Statfs` disk scrape, `/healthz` query,
      then PCIe/XID triage classification
- [ ] Durable job store (PostgreSQL or Redis) behind the existing `Store` API
- [ ] Auth (API key) and per-tenant rate limiting
- [ ] Streaming results via Server-Sent Events for long jobs

## License

MIT — see [LICENSE](LICENSE).

Built by Guangyi "Ezra" Zhao as a job-search portfolio project.

---

All code in this repository is clean-room code written by Guangyi Zhao for learning and research purposes. It does not contain any client or employer confidential information.
