# Go-Diagnoser-Engine

[English](README.md) | [简体中文](README.zh-CN.md) | **[日本語](README.ja.md)** | [한국어](README.ko.md) | [Español](README.es.md) | [Português](README.pt.md) | [Русский](README.ru.md)

> **Note:** this translation tracks an older README version (scaffold). The English README.md is authoritative for v1.0.

![Go 1.24](https://img.shields.io/badge/go-1.24-blue) ![License: MIT](https://img.shields.io/badge/license-MIT-green) ![Status: scaffold](https://img.shields.io/badge/status-scaffold-orange)


**高並行バックエンド診断エンジン**——REST で診断ジョブを受け付け、有界ワーカープール上でプローブを並行実行し、構造化された結果を返す Go マイクロサービス。

> **プロジェクト状態：スキャフォールド（正直エディション）。**リクエストパス全体がエンドツーエンドで動作します——`POST /api/v1/diagnose` → ワーカープール → `GET /api/v1/jobs/{id}`——ただしプローブ自体は**シミュレーションのプレースホルダー**です（全レスポンスに `SIMULATED` と明示）。実際の診断ルールは次のステップで、コード内に `TODO(ezra)` としてマークされています。ここにあるものは、実ハードウェアをプローブしているふりをしていません。

## 課題

バックエンド・インフラチームには、ノード群に対してヘルス／診断チェックを実行する高速で統一的な方法が必要です：ターゲットを投入すれば、呼び出し元をブロックせずにレイテンシ付きの構造化された合否結果が得られる。このサービスは、そのワークフローの実行基盤です。

## アーキテクチャ

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

並行モデル：

- **固定ワーカープール**（デフォルト `runtime.NumCPU()`）：長寿命 goroutine、リクエストごとの goroutine 生成なし。
- **有界キュー**：無制限な増加ではなく、フェイルファストな 503 によるバックプレッシャー。
- **ジョブ単位のファンアウト**：ジョブの各チェックは短寿命 goroutine で実行され、worker はそれらが join してから次へ進みます。各チェックには 30 秒の個別タイムアウトがあります。
- **グレースフルシャットダウン**：`SIGINT/SIGTERM` → 受付停止 → 処理中をドレイン → 終了。goroutine リークなし。

## 技術スタック

- **Go 1.24**、標準ライブラリのみ（外部依存ゼロ）
- `net/http` 上の REST API（Go 1.22+ の method+pattern ルーティング）
- ワーカープール ＋ channel ＋ `sync.WaitGroup` による並行処理
- Docker マルチステージビルド、`docker-compose` でワンコマンド実行

## クイックスタート

```bash
# ビルドと実行
go build -o server ./cmd/server
./server                  # listens on :8080

# または docker で
docker compose up --build
```

環境変数で設定：`PORT`（デフォルト 8080）、`WORKERS`（デフォルト NumCPU）、`QUEUE_SIZE`（デフォルト 100）。

## API

```bash
# 診断ジョブを投入（登録済みチェックを全実行）
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42"}'
# -> {"id":"<job-id>","status":"queued"}   (HTTP 202)

# 結果をポーリング
curl -s localhost:8080/api/v1/jobs/<job-id> | jq .
# -> {"id":..., "status":"done",
#     "results":[{"name":"connectivity","passed":true,
#                 "detail":"SIMULATED: ...","latency_ms":153}, ...]}

# 選択したチェックのみ実行
curl -s -X POST localhost:8080/api/v1/diagnose \
  -H 'Content-Type: application/json' \
  -d '{"target":"node-42","checks":["connectivity"]}'

# ヘルスチェック
curl -s localhost:8080/healthz
```

## プロジェクト構成

```
cmd/server/          entrypoint: config, graceful shutdown
internal/api/        HTTP routes (diagnose, job status, healthz)
internal/engine/     worker pool + check registry
internal/store/      job persistence (in-memory; durable backend TODO)
pkg/models/          shared domain types
```

## ロードマップ

- [x] スキャフォールド：REST API → ワーカープール → シミュレーション結果、エンドツーエンド
- [ ] **実際の診断ルール**（`internal/engine/checks.go`、`TODO(ezra)`）：ICMP/TCP 接続性プローブ、ディスク使用量スクレイプ、`/healthz` クエリ、そして PCIe/XID トリアージ分類——本プロジェクトをポートフォリオたらしめるドメインロジック
- [ ] 既存 `Store` API の背後に永続ジョブストア（PostgreSQL または Redis）
- [ ] 認証（API キー）とテナント単位のレート制限
- [ ] Prometheus メトリクス（`jobs_total`、`check_latency_seconds`、キュー深さ）
- [ ] 長時間ジョブ向けに Server-Sent Events で結果をストリーミング

## ライセンス

MIT —— [LICENSE](LICENSE) を参照。

Guangyi "Ezra" Zhao による求職ポートフォリオプロジェクト。

---
All code in this repository is clean-room code written by Guangyi Zhao for learning and research purposes. It does not contain any client or employer confidential information.
