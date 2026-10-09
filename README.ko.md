# Go-Diagnoser-Engine

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | **[한국어](README.ko.md)** | [Español](README.es.md) | [Português](README.pt.md) | [Русский](README.ru.md)

> **Note:** this translation tracks an older README version (scaffold). The English README.md is authoritative for v1.0.

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: scaffold](https://img.shields.io/badge/status-scaffold-orange)


**고동시성 백엔드 진단 엔진** — REST로 진단 작업을 받아 유계 워커 풀에서 프로브를 병렬로 실행하고 구조화된 결과를 반환하는 Go 마이크로서비스.

> **프로젝트 상태: 스캐폴드(정직 에디션).** 전체 요청 경로가 엔드투엔드로 동작합니다 — `POST /api/v1/diagnose` → 워커 풀 → `GET /api/v1/jobs/{id}` — 다만 프로브 자체는 **시뮬레이션 플레이스홀더**입니다(모든 응답에 `SIMULATED`로 명시). 실제 진단 규칙은 다음 단계이며 코드에 `TODO(ezra)`로 표시되어 있습니다. 여기 있는 어떤 것도 실제 하드웨어를 프로빙하는 척하지 않습니다.

## 문제

백엔드 및 인프라 팀은 노드 플릿에 대해 상태/진단 검사를 실행할 빠르고 통일된 방법이 필요합니다: 대상을 제출하면 호출자를 차단하지 않고 지연 시간이 포함된 구조화된 통과/실패 결과를 받습니다. 이 서비스는 해당 워크플로의 실행 백본입니다.

## 아키텍처

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

동시성 모델:

- **고정 워커 풀**(기본값 `runtime.NumCPU()`): 장수명 고루틴, 요청당 고루틴 생성 없음.
- **유계 큐**: 무한 증가 대신 빠른 실패 503을 통한 백프레셔.
- **작업 단위 팬아웃**: 작업의 검사들은 단수명 고루틴에서 실행되고 워커는 이들이 join된 후에 진행합니다. 각 검사에는 30초 타임아웃이 있습니다.
- **그레이스풀 셧다운**: `SIGINT/SIGTERM` → 수락 중단 → 진행 중 작업 드레인 → 종료. 고루틴 누수 없음.

## 기술 스택

- **Go 1.24**, 표준 라이브러리만(외부 의존성 제로)
- `net/http` 기반 REST API(Go 1.22+ method+pattern 라우팅)
- 워커 풀 + channel + `sync.WaitGroup` 동시성
- Docker 멀티 스테이지 빌드, `docker-compose`로 한 번에 실행

## 빠른 시작

```bash
# 빌드 및 실행
go build -o server ./cmd/server
./server                  # listens on :8080

# 또는 docker로
docker compose up --build
```

환경 변수로 설정: `PORT`(기본값 8080), `WORKERS`(기본값 NumCPU), `QUEUE_SIZE`(기본값 100).

## API

```bash
# 진단 작업 제출(등록된 모든 검사 실행)
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# 결과 폴링
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# 선택한 검사만 실행
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["connectivity"]}'

# 헬스 체크
curl -s localhost:8080/healthz
```

## 프로젝트 레이아웃

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## 로드맵

- [x] 스캐폴드: REST API → 워커 풀 → 시뮬레이션 결과, 엔드투엔드
- [ ] **실제 진단 규칙**(`internal/engine/checks.go`, `TODO(ezra)`): ICMP/TCP 연결성 프로브, 디스크 사용량 스크레이프, `/healthz` 쿼리, 그리고 PCIe/XID 트리아지 분류 — 이 프로젝트를 포트폴리오로 만드는 도메인 로직
- [ ] 기존 `Store` API 뒤에 영속적 작업 저장소(PostgreSQL 또는 Redis)
- [ ] 인증(API 키) 및 테넌트별 속도 제한
- [ ] Prometheus 메트릭(`jobs_total`, `check_latency_seconds`, 큐 깊이)
- [ ] 장기 작업용 Server-Sent Events로 결과 스트리밍

## 라이선스

MIT — [LICENSE](LICENSE) 참조.

Guangyi "Ezra" Zhao가 구직 포트폴리오 프로젝트로 제작.

---
All code in this repository is clean-room code written by Guangyi Zhao for learning and research purposes. It does not contain any client or employer confidential information.
