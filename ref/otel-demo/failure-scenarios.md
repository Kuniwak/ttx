# OpenTelemetry Demo — 注入可能な障害モード

OpenTelemetry Demo は `flagd`（フィーチャーフラグ）で**意図的な障害を注入**できます。机上演習のシナリオは、これらの障害モードを「本番で起きうる障害」に見立てて題材にします（フラグの ON は、現実では不具合のあるデプロイ・設定変更・依存先障害に相当）。

出典（独自記述）: <https://opentelemetry.io/docs/demo/feature-flags/>

| フラグ（障害モード） | 起きること | 主な観測 |
| --- | --- | --- |
| `paymentServiceFailure` | `payment` がカード課金に失敗し、チェックアウトが決済段で失敗する | checkout/payment のエラー率スパイク、payment スパンがエラーのトレース、注文確定の失敗 |
| `paymentServiceUnreachable` | `payment` に到達できず、checkout が決済段でタイムアウト/接続失敗 | checkout→payment 間の接続エラー、レイテンシ増大 |
| `cartServiceFailure` | `cart` の操作（カート取得・空化）が失敗する（Valkey 接続の問題に相当） | cart のエラー率上昇、カート画面・チェックアウト前段の失敗 |
| `productCatalogFailure` | `product-catalog` が特定商品の照会でエラーを返す | product-catalog の部分的エラー、特定商品ページの失敗、recommendation への波及 |
| `recommendationServiceCacheFailure` | `recommendation` のキャッシュが無制限に増大（メモリリーク） | recommendation のメモリ使用量とレイテンシの増大、OOM の兆候 |
| `adServiceFailure` / `adServiceHighCpu` | `ad` がエラーを返す / CPU を消費する | ad のエラー率・CPU・GC の上昇、広告枠の劣化 |
| `kafkaQueueProblems` | `kafka` の滞留が増え、消費が遅延する | consumer lag 増大、accounting / fraud-detection の処理遅延 |
| `loadGeneratorFloodHomepage` | トップページへ過剰な負荷 | frontend のレイテンシ・リソース上昇 |
| `imageSlowLoad` | 商品画像の読み込みが遅延 | フロントの画像表示遅延 |

## 本リポジトリで採用しているシナリオ

`SCN-001` は **`paymentServiceFailure`** を題材にしています。`payment` の課金失敗により `checkout` が決済段で失敗し、顧客が注文を完了できなくなる障害です。クリティカルパス上の単一サービス障害で、検知（エラー率スパイク・トレース）→ 暫定対応（フラグ切り戻し/ロールバック）→ 真因特定 → 影響整理、という SRE 的に筋の良い流れを学べます（`scenarios/SCN-001`、`timelines/SCN-001/`、`ops/` を参照）。
