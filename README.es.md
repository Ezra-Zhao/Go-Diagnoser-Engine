# Go-Diagnoser-Engine

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | **[Español](README.es.md)** | [Português](README.pt.md) | [Русский](README.ru.md)

> **Note:** this translation tracks an older README version (scaffold). The English README.md is authoritative for v1.0.

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: scaffold](https://img.shields.io/badge/status-scaffold-orange)


**Motor de diagnóstico backend de alta concurrencia**: un microservicio Go que acepta trabajos de diagnóstico vía REST, ejecuta sondeos concurrentemente en un pool de workers acotado y devuelve resultados estructurados.

> **Estado del proyecto: andamiaje (edición honesta).** La ruta completa de la solicitud funciona de extremo a extremo: `POST /api/v1/diagnose` → pool de workers → `GET /api/v1/jobs/{id}`; pero los sondeos en sí son **marcadores simulados** (etiquetados claramente como `SIMULATED` en cada respuesta). Las reglas de diagnóstico reales son el siguiente paso y están marcadas con `TODO(ezra)` en el código. Nada aquí pretende sondear hardware real.

## Problema

Los equipos de backend e infraestructura necesitan una forma rápida y uniforme de ejecutar comprobaciones de salud/diagnóstico contra flotas de nodos: enviar un objetivo y obtener resultados estructurados de aprobado/fallido con latencia, sin bloquear al llamante. Este servicio es la columna vertebral de ejecución de ese flujo.

## Arquitectura

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

Modelo de concurrencia:

- **Pool de workers fijo** (por defecto `runtime.NumCPU()`): goroutines de larga vida, sin crear goroutines por solicitud.
- **Cola acotada**: contrapresión mediante 503 de fallo rápido en lugar de crecimiento ilimitado.
- **Fan-out por trabajo**: las comprobaciones de un trabajo corren en goroutines de corta vida y se sincronizan (join) antes de que el worker continúe; cada comprobación tiene su propio timeout de 30 s.
- **Apagado elegante**: `SIGINT/SIGTERM` → dejar de aceptar → drenar en curso → salir. Sin fugas de goroutines.

## Tecnologías

- **Go 1.24**, solo biblioteca estándar (cero dependencias externas)
- API REST sobre `net/http` (enrutamiento method+pattern de Go 1.22+)
- Concurrencia con worker pool + channels + `sync.WaitGroup`
- Build multi-etapa de Docker, `docker-compose` para ejecutar con un solo comando

## Inicio rápido

```bash
# compilar y ejecutar
go build -o server ./cmd/server
./server                  # listens on :8080

# o con docker
docker compose up --build
```

Configuración por entorno: `PORT` (8080 por defecto), `WORKERS` (NumCPU por defecto), `QUEUE_SIZE` (100 por defecto).

## API

```bash
# enviar un trabajo de diagnóstico (todas las comprobaciones registradas)
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# consultar el resultado
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# ejecutar solo las comprobaciones seleccionadas
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["connectivity"]}'

# salud
curl -s localhost:8080/healthz
```

## Estructura del proyecto

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## Hoja de ruta

- [x] Andamiaje: API REST → pool de workers → resultados simulados, de extremo a extremo
- [ ] **Reglas de diagnóstico reales** (`internal/engine/checks.go`, `TODO(ezra)`): sondeo de conectividad ICMP/TCP, recolección de uso de disco, consulta a `/healthz` y luego clasificación de triaje PCIe/XID; la lógica de dominio que hace de esto una pieza de portafolio
- [ ] Almacén de trabajos duradero (PostgreSQL o Redis) tras la API `Store` existente
- [ ] Autenticación (API key) y limitación de tasa por tenant
- [ ] Métricas de Prometheus (`jobs_total`, `check_latency_seconds`, profundidad de la cola)
- [ ] Resultados en streaming vía Server-Sent Events para trabajos largos

## Licencia

MIT — ver [LICENSE](LICENSE).

Construido por Guangyi "Ezra" Zhao como proyecto de portafolio para búsqueda de empleo.

---
All code in this repository is clean-room code written by Guangyi Zhao for learning and research purposes. It does not contain any client or employer confidential information.
