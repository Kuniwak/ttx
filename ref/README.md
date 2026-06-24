# ref/ — 対象システムの参照資料

このディレクトリは、机上演習で扱う**対象システム（system-under-test）の仕様・設計・アーキテクチャ資料**を置く場所です。シナリオ着想（`.ai/agents/93-conceive-scenario.md`）や理想タイムライン生成（`.ai/agents/94-build-canonical-timeline.md`）が参照します。

本リポジトリでは、対象システムとして公開のサンプル分散システム **[OpenTelemetry Demo（Astronomy Shop）](https://github.com/open-telemetry/opentelemetry-demo)** を採用しています（Apache-2.0, © The OpenTelemetry Authors）。

- `otel-demo/system-component.md` — サービスカタログ（論理名・実装言語・役割）
- `otel-demo/architecture.md` — リクエストの流れ・依存関係・可観測性スタック
- `otel-demo/failure-scenarios.md` — フィーチャーフラグで注入できる障害モード（シナリオの題材）

> [!NOTE]
> ここに置く記述は OpenTelemetry Demo の公開ドキュメント（<https://opentelemetry.io/docs/demo/>）を典拠にした**独自記述**であり、原文の転載ではありません。詳細・最新情報は一次資料を参照してください。
>
> 別の対象システムで演習したい場合は、この `ref/` を差し替え、`ops/`・`OBSERVATION.tsv`・`SCENARIO.tsv`・`timelines/` を作り直します（旧来は git submodule でしたが、OSS 版では通常ディレクトリとして同梱します）。
