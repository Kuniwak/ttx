---
name: design-review
description: origin/master からの差分をレビューする
context: fork
---

現状の差分が以下の検査事項に反していないかチェックしてください。

# 検査事項

- ユーティリティとしてパブリックな関数やメソッドなどに切り出せるものがプライベートなものとして実装されていれば NG
- 各 CLI の cmd パッケージ内はこの CLI に関する CLI 層の処理だけが書かれていなければ NG。仮に Web や RPC などのインターフェースに対応した際にその処理が必要になりうるならば CLI 層の処理ではない
- 別のインターフェースにすれば組み合わせて再利用可能性が増えるものがあれば NG
- 仕様にないフォールバック処理は結果の一貫性を損なう恐れがあるので NG
- コンストラクタや関数の引数が nil / zero value のときに default 実装にフォールバック注入する処理を含んでいれば NG。caller は `nil` と書いた時点で実際には特定の振る舞いを期待しているはずだが、call site には `nil` としか現れず、その期待が「production の本物の時計か」「ログ抑制か」「テスト用 fake か」「単に書き忘れか」は呼び出された側の fallback 実装を読まないと分からない。これが「**暗黙**」であり、call site だけ読んで振る舞いを把握できない可読性低下の原因になる。**caller が期待する振る舞いを実現するコンポーネントを必ず call site に書き、不正値は fail-fast (panic / error 返却) にする**。例: 依存コンポーネントの注入時に nil / zero 許容するのは NG、call site で依存コンポーネントを明示的に書く。テスト側でも `slogtest.NewSpy(t)` や fake clock を明示注入する。production の「ログ抑制」も `slog.New(slog.DiscardHandler)` を call site に明示する。
- 計測 / ログ / メトリクス / トレースなどの**横断的関心事 (cross-cutting concerns)** を、本来そのドメインを知らない関数や構造体の引数 / フィールドに optional dependency として伝播させていれば NG。テスト側でも `nil` 渡しや関係ないシグネチャ追従が広範に必要になる時点で、ISP (Interface Segregation Principle) 違反のサイン。決まり手は decorator pattern: 既存関数の signature は変えず、function type / interface の呼び出し境界で wrapper を挟む。**「テストにシグネチャ変更が伝播するとき」は設計の誤りを疑え**。
- 1 メソッドだけの interface (例: `type Submitter interface { Submit(json.RawMessage) }`) を定義していれば NG。**関数型エイリアス** (例: `type SubmitFunc func(json.RawMessage)`) の方が取り回しが良い：呼び出し側はメソッド値 (`appender.Submit`) や無名関数を直接渡せ、struct を作らずに decorator が書けるため。`io.Reader` / `io.Writer` のような複数メソッドの interface はもちろん別。
- decorator / 計測 / ログなどの依存先を呼び出し側で `if rec != nil { ... }` のように分岐させていれば NG。**NullObject pattern** を使う：interface 化した上で no-op 実装（例: `phasetiming.Null`）を singleton として提供し、call site では分岐せず常に decorator を被せる。「分岐入れ忘れ」のバグを構造的に防ぐ。
- TSV や CSV はフィールドに改行や水平タブが入る可能性があるのでこれらを head や tail, awk, grep など行指向のツールで処理していれば NG。Go 言語であれば csv.{Reader,Writer} の Comma = '\t' 相当の処理を使い、Bash 言語であれば qhs （TSV なら -t を付加） や tsv-tools（tsvcolidx, tsvdedup, tsvdiff, tsvfix, tsvparse, tsvrmcol, tsvrmrow, tsvsplit など。それぞれのコマンドの使い方は -h つきで呼び出して確認すること）を使うこと
- すべてのテストケースが 4-phase テスト（setup, execute, assert, teardown）になっていなければ NG
- setup が複雑すぎる場合は抽象層の不足を示唆しているので NG
- 類似した 4-phase テストがあったら table-driven テストにまとめられてなければ NG
