# OpenTelemetry Demo — アーキテクチャと主要フロー

出典（独自記述）: <https://opentelemetry.io/docs/demo/architecture/>

## 全体像

外部リクエストは `frontend-proxy`（Envoy）を入口に `frontend` へ届き、`frontend` が各バックエンドサービスを gRPC で呼び出して画面を構成します。注文確定（チェックアウト）は `checkout` が複数サービスをオーケストレーションし、確定した注文は Kafka に発行されて `accounting` と `fraud-detection` が非同期に処理します。

```
ユーザー → frontend-proxy(Envoy) → frontend ─┬→ product-catalog
                                             ├→ cart ──→ valkey
                                             ├→ recommendation → product-catalog
                                             ├→ ad
                                             ├→ currency
                                             ├→ shipping → quote
                                             └→ checkout ─┬→ cart（内容取得）
                                                          ├→ product-catalog
                                                          ├→ currency（換算）
                                                          ├→ shipping（送料）
                                                          ├→ payment（課金）
                                                          ├→ email（確認メール）
                                                          └→ kafka(orders) ─┬→ accounting
                                                                            └→ fraud-detection
```

## チェックアウト（注文確定）のフロー

1. ユーザーがカートで「購入」を実行し、`frontend` → `checkout` に確定要求。
2. `checkout` は `cart` からカート内容、`product-catalog` から商品情報、`currency` から換算、`shipping`（→`quote`）から送料を取得する。
3. `checkout` が `payment` を呼び、カード課金を行う。**ここが失敗すると注文は確定できない。**
4. 課金成功後、`checkout` が `email` で確認メールを送り、注文イベントを `kafka` の `orders` トピックに発行する。
5. `accounting` と `fraud-detection` が `orders` を購読して非同期処理する（チェックアウト応答のクリティカルパス外）。

このフロー上、`payment` はチェックアウトのクリティカルパスにあり、障害が直接「注文できない」顧客影響に直結します。一方 `accounting` / `fraud-detection` の遅延は、購入体験を即座には止めません（ただし会計・不正検知の滞留として後から効いてきます）。

## 可観測性

全サービスは OpenTelemetry SDK で計装され、trace / metric / log を `otel-collector` 経由で `jaeger`（trace）・`prometheus`（metric）・`opensearch`（log）へ送ります。`grafana` がダッシュボードを提供します。障害調査では、エラー率・レイテンシのメトリクス（Prometheus/Grafana）と、失敗したリクエストの分散トレース（Jaeger）を突き合わせて、どのサービス境界で失敗しているかを特定できます。
