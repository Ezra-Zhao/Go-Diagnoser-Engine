# Go-Diagnoser-Engine

**High-Concurrency Backend Diagnostics Engine** — a Go microservice that accepts
diagnostic jobs over REST, executes probes concurrently on a bounded worker
pool, and returns structured results.

> **Project status: scaffold (honest edition).** The full request path works
> end to end — `POST /api/v1/diagnose` → worker pool → `GET /api/v1/jobs/{id}` —
> but the probes themselves are **simulated placeholders** (clearly labeled
> `SIMULATED` in every response). Real diagnostic rules are the next step and
> are marked with `TODO(ezra)` in the code. Nothing here pretends to probe
> real hardware.

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
                 |   connectivity | disk_usage | service_health |
                 +----------------+----------------------------+
                                          |
                                   +-------------+
                                   |    Store    |  in-memory (Postgres/Redis TODO)
                                   +-------------+
                                          |
  GET /jobs/{id} <-- structured CheckResult[] with latency
```

Concurrency model:

- **Fixed worker pool** (default `runtime.NumCPU()`): long-lived goroutines,
  no per-request goroutine spawning.
- **Bounded queue**: backpressure via fail-fast 503 instead of unbounded growth.
- **Per-job fan-out**: a job's checks run on short-lived goroutines and are
  joined before the worker proceeds; each check has its own 30s timeout.
- **Graceful shutdown**: `SIGINT/SIGTERM` → stop accepting → drain in-flight →
  exit. No goroutine leaks.

## Tech stack

- **Go 1.24**, standard library only (zero external dependencies)
- REST API on `net/http` (Go 1.22+ method+pattern routing)
- Worker pool + channels + `sync.WaitGroup` concurrency
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
  -d '{"target":"node-42","checks":["connectivity"]}'

# health
curl -s localhost:8080/healthz
```

## Project layout

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## Roadmap

- [x] Scaffold: REST API → worker pool → simulated results, end to end
- [ ] **Real diagnostic rules** (`internal/engine/checks.go`, `TODO(ezra)`):
      ICMP/TCP connectivity probe, disk-usage scrape, `/healthz` query,
      then PCIe/XID triage classification — the domain logic that makes
      this a portfolio piece
- [ ] Durable job store (PostgreSQL or Redis) behind the existing `Store` API
- [ ] Auth (API key) and per-tenant rate limiting
- [ ] Prometheus metrics (`jobs_total`, `check_latency_seconds`, queue depth)
- [ ] Streaming results via Server-Sent Events for long jobs

## License

MIT — see [LICENSE](LICENSE).

Built by Guangyi "Ezra" Zhao as a job-search portfolio project.
