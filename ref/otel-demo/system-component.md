# OpenTelemetry Demo — サービスカタログ

架空のEC「Astronomy Shop」（天体望遠鏡などを販売）を題材にした、多言語のマイクロサービス群です。各サービスは主に gRPC で相互通信し、注文確定後の処理は Kafka を介して非同期に流れます。

出典（独自記述・転載ではない）: <https://opentelemetry.io/docs/demo/services/> / <https://github.com/open-telemetry/opentelemetry-demo>

## アプリケーションサービス

| 論理名 | 役割 | 主な実装言語 | 主な依存 |
| --- | --- | --- | --- |
| `frontend` | Web UI（Next.js）。各バックエンドを集約して画面を返す | TypeScript | product-catalog, cart, checkout, currency, recommendation, ad, shipping |
| `frontend-proxy` | エッジのリバースプロキシ（Envoy）。外部からの入口 | Envoy | frontend, 各種 |
| `load-generator` | 擬似ユーザー負荷を生成（Locust） | Python | frontend |
| `image-provider` | 商品画像を配信 | nginx | — |
| `product-catalog` | 商品マスタの照会（一覧・詳細） | Go | （フラグ）flagd |
| `cart` | カート操作（追加・取得・空にする） | C# / .NET | valkey（カート永続化） |
| `checkout` | 注文確定のオーケストレーション | Go | cart, currency, product-catalog, payment, shipping, email, kafka |
| `payment` | 決済（カード課金） | JavaScript / Node | （フラグ）flagd |
| `shipping` | 配送見積もり・出荷 | Rust | quote |
| `quote` | 配送料の見積もり計算 | PHP | — |
| `currency` | 通貨換算 | C++ | — |
| `email` | 注文確認メール送信 | Ruby | — |
| `recommendation` | おすすめ商品の算出 | Python | product-catalog |
| `ad` | 広告の配信 | Java | — |
| `accounting` | 注文の会計記録（Kafka を購読） | .NET | kafka |
| `fraud-detection` | 不正検知（Kafka を購読） | Kotlin | kafka |

## ミドルウェア / 基盤

| 論理名 | 役割 |
| --- | --- |
| `kafka` | 注文イベントのメッセージキュー（`orders` トピック）。accounting / fraud-detection が購読 |
| `valkey` | カートの永続化に使う Redis 互換のインメモリストア |
| `flagd` | フィーチャーフラグ配信。障害注入（`failure-scenarios.md`）の切り替えに使う |

## 可観測性スタック（観測の取得元）

| 論理名 | 役割 |
| --- | --- |
| `otel-collector` | テレメトリ（trace / metric / log）の受信・処理・転送 |
| `jaeger` | 分散トレースの保存・可視化 |
| `prometheus` | メトリクスの収集・保存（アラート評価） |
| `grafana` | ダッシュボード・可視化 |
| `opensearch` | ログの保存・検索 |

`OBSERVATION.tsv` の `取得元` 列には、これらの可観測性スタック（Grafana / Prometheus / Jaeger / OpenSearch）やアプリのエンドポイントを記述します。
