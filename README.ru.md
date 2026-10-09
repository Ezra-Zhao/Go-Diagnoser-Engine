# Go-Diagnoser-Engine

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Português](README.pt.md) | **[Русский](README.ru.md)**

> **Note:** this translation tracks an older README version (scaffold). The English README.md is authoritative for v1.0.

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: scaffold](https://img.shields.io/badge/status-scaffold-orange)


**Высоконагруженный backend-движок диагностики** — микросервис на Go, принимающий диагностические задания по REST, выполняющий пробы конкурентно в ограниченном пуле воркеров и возвращающий структурированные результаты.

> **Статус проекта: каркас (честная редакция).** Полный путь запроса работает end-to-end — `POST /api/v1/diagnose` → пул воркеров → `GET /api/v1/jobs/{id}` — но сами пробы являются **симулированными заглушками** (в каждом ответе явно помечены `SIMULATED`). Настоящие правила диагностики — следующий шаг, они помечены в коде как `TODO(ezra)`. Ничто здесь не притворяется, что зондирует реальное оборудование.

## Задача

Командам backend и инфраструктуры нужен быстрый, унифицированный способ запуска проверок работоспособности/диагностики на флоте узлов: отправить цель и получить структурированные результаты pass/fail с задержками, не блокируя вызывающую сторону. Этот сервис — исполнительный каркас такого workflow.

## Архитектура

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

Модель конкурентности:

- **Фиксированный пул воркеров** (по умолчанию `runtime.NumCPU()`): долгоживущие горутины, без создания горутин на запрос.
- **Ограниченная очередь**: противодавление через быстрый отказ 503 вместо неограниченного роста.
- **Разветвление на задание**: проверки задания выполняются в короткоживущих горутинах, воркер продолжает работу после их join; у каждой проверки свой таймаут 30 с.
- **Корректное завершение**: `SIGINT/SIGTERM` → прекращение приёма → ожидание текущих задач → выход. Без утечек горутин.

## Технологии

- **Go 1.24**, только стандартная библиотека (ноль внешних зависимостей)
- REST API на `net/http` (маршрутизация method+pattern из Go 1.22+)
- Конкурентность: пул воркеров + каналы + `sync.WaitGroup`
- Docker multi-stage сборка, `docker-compose` для запуска одной командой

## Быстрый старт

```bash
# сборка и запуск
go build -o server ./cmd/server
./server                  # listens on :8080

# или через docker
docker compose up --build
```

Настройка через окружение: `PORT` (по умолчанию 8080), `WORKERS` (по умолчанию NumCPU), `QUEUE_SIZE` (по умолчанию 100).

## API

```bash
# отправить диагностическое задание (все зарегистрированные проверки)
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# опрос результата
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# выполнить только выбранные проверки
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["connectivity"]}'

# проверка работоспособности
curl -s localhost:8080/healthz
```

## Структура проекта

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## Планы развития

- [x] Каркас: REST API → пул воркеров → симулированные результаты, end-to-end
- [ ] **Настоящие правила диагностики** (`internal/engine/checks.go`, `TODO(ezra)`): ICMP/TCP-пробы связности, сбор данных об использовании диска, запрос `/healthz`, затем классификация PCIe/XID triage — доменная логика, делающая проект портфолио-достойным
- [ ] Долговременное хранилище заданий (PostgreSQL или Redis) за существующим API `Store`
- [ ] Аутентификация (API-ключ) и ограничение частоты запросов на тенант
- [ ] Метрики Prometheus (`jobs_total`, `check_latency_seconds`, глубина очереди)
- [ ] Потоковая выдача результатов через Server-Sent Events для долгих заданий

## Лицензия

MIT — см. [LICENSE](LICENSE).

Создано Guangyi "Ezra" Zhao как проект портфолио для поиска работы.
