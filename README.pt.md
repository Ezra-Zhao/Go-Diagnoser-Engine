# Go-Diagnoser-Engine

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | **[Português](README.pt.md)** | [Русский](README.ru.md)

> **Note:** this translation tracks an older README version (scaffold). The English README.md is authoritative for v1.0.

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: scaffold](https://img.shields.io/badge/status-scaffold-orange)


**Mecanismo de diagnóstico backend de alta concorrência** — um microsserviço Go que aceita trabalhos de diagnóstico via REST, executa sondas concorrentemente em um pool de workers limitado e retorna resultados estruturados.

> **Estado do projeto: andaime (edição honesta).** O caminho completo da requisição funciona de ponta a ponta — `POST /api/v1/diagnose` → pool de workers → `GET /api/v1/jobs/{id}` — mas as sondas em si são **placeholders simulados** (claramente rotulados como `SIMULATED` em cada resposta). As regras reais de diagnóstico são o próximo passo e estão marcadas com `TODO(ezra)` no código. Nada aqui finge sondar hardware real.

## Problema

Equipes de backend e infra precisam de uma forma rápida e uniforme de executar verificações de saúde/diagnóstico contra frotas de nós: enviar um alvo e obter resultados estruturados de aprovado/reprovado com latência, sem bloquear o chamador. Este serviço é a espinha dorsal de execução desse fluxo.

## Arquitetura

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

Modelo de concorrência:

- **Pool de workers fixo** (padrão `runtime.NumCPU()`): goroutines de longa duração, sem criar goroutines por requisição.
- **Fila limitada**: contrapressão via 503 de falha rápida em vez de crescimento ilimitado.
- **Fan-out por trabalho**: as verificações de um trabalho rodam em goroutines de curta duração e têm join antes de o worker prosseguir; cada verificação tem seu próprio timeout de 30 s.
- **Desligamento gracioso**: `SIGINT/SIGTERM` → parar de aceitar → drenar em andamento → sair. Sem vazamento de goroutines.

## Tecnologias

- **Go 1.24**, apenas biblioteca padrão (zero dependências externas)
- API REST sobre `net/http` (roteamento method+pattern do Go 1.22+)
- Concorrência com worker pool + channels + `sync.WaitGroup`
- Build multi-estágio do Docker, `docker-compose` para rodar com um comando

## Início rápido

```bash
# compilar e executar
go build -o server ./cmd/server
./server                  # listens on :8080

# ou com docker
docker compose up --build
```

Configuração via ambiente: `PORT` (padrão 8080), `WORKERS` (padrão NumCPU), `QUEUE_SIZE` (padrão 100).

## API

```bash
# enviar um trabalho de diagnóstico (todas as verificações registradas)
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# consultar o resultado
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# executar apenas as verificações selecionadas
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["connectivity"]}'

# saúde
curl -s localhost:8080/healthz
```

## Estrutura do projeto

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## Roteiro

- [x] Andaime: API REST → pool de workers → resultados simulados, de ponta a ponta
- [ ] **Regras reais de diagnóstico** (`internal/engine/checks.go`, `TODO(ezra)`): sonda de conectividade ICMP/TCP, coleta de uso de disco, consulta a `/healthz` e depois classificação de triagem PCIe/XID — a lógica de domínio que faz disto uma peça de portfólio
- [ ] Armazenamento durável de trabalhos (PostgreSQL ou Redis) atrás da API `Store` existente
- [ ] Autenticação (API key) e limitação de taxa por tenant
- [ ] Métricas do Prometheus (`jobs_total`, `check_latency_seconds`, profundidade da fila)
- [ ] Resultados em streaming via Server-Sent Events para trabalhos longos

## Licença

MIT — ver [LICENSE](LICENSE).

Construído por Guangyi "Ezra" Zhao como projeto de portfólio para busca de emprego.

---
All code in this repository is clean-room code written by Guangyi Zhao for learning and research purposes. It does not contain any client or employer confidential information.
