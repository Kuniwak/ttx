---
name: 92-build-scenario
description: 障害対応手順書・インシデント表のいずれか1つを入力に、机上演習用のシナリオを1件追加するフロー外の保守スキル。SCENARIO.tsv 行・timelines/<ID>/ の3点・SCENARIO-LIST.tsv を整備する必要があるときに使う。
---

# シナリオ追加ランナー

このskillは演習の直線フローには属しません。「障害対応手順書1つにつき対応するシナリオが1つ」ある状態へ近づけるため、**1つの入力**から**1つのシナリオ**を完成させる canonical 正本の整備ツールです。`90-build-scenario-list`・`91-design-review` と同じフロー外バッチの位置づけです。

演習中ではないので、IPC（`ttx-*`）・画面隠し・`.ai/runs/<run-id>/` の演習機構は使いません。判断が必要な場面では操作者と**直接対話**してよいです（参加者はいません）。広い推論を要する生成・評価は build agent（`93`〜`97`）に委ね、このskillは順序制御・入力正規化・ID採番・ファイル受け渡し・重複ゲート・一覧再生成という決定的処理を担います。

## 言語

`シナリオ` 本文・タイムライン・質問・回答は日本語で書きます。技術識別子（観測ID・手順書パス・コンポーネント名）は元資料のまま使ってかまいません。

## 入力

次のいずれか**1つ**を入力に取ります。

- 障害対応手順書: `ops/<番号>.md`
- インシデント表の1行: `INCIDENTS.tsv:N`（`N` はヘッダ込み1始まりの行番号）または事象文字列

参照する正本・資料:

- `SCENARIO.tsv` / `scenarios/` / `timelines/` / `SCENARIO-LIST.tsv`
- `OBSERVATION.tsv` / `INCIDENTS.tsv` / `EVAL_CRITERIA.md`
- `ops/incident-handling-manual.md` ほか `./ops` 配下、`./ref` 配下の仕様・設計・アーキテクチャ資料
- `.ai/shared/references/data-formats.md`

## 出力

- `SCENARIO.tsv` への1行追記と `scenarios/<新ID>` の書き出し
- `timelines/<新ID>/{TIMELINE-1.tsv, QUESTIONS-1.tsv, ANSWERS-1.tsv}`
- `SCENARIO-LIST.tsv` の全件再生成（`90-build-scenario-list` 経由）
- 内部ログの記録（任意。フロー外のため `.ai/runs/` が無ければ省略可）

## 共通仕様

- TSV を読み書きする前に `.ai/shared/references/data-formats.md` を読む。`元リスク事象ID` の値規約と先頭データ行の重大度・エスカレーション注記規約に従う。
- TSV は列ずれを避けるため TSV 対応ツール（`qhs -t` など）で扱い、`awk`／`tail` で列を切らない。本文中のタブ・改行は空白へ正規化して1行に収める。
- `timelines/<ID>/` は3点構成（`TIMELINE-1` / `QUESTIONS-1` / `ANSWERS-1`）を保つ。`REVIEWED-ANSWERS-<N>.tsv` 等は作らない。

## 実行手順

1. **入力正規化**: 入力1つの種別を判定し、`{input_kind, source_ref, seed_risk_id}` を作る。
   - `ops/<番号>.md` → `input_kind=ops`。`OBSERVATION.tsv` の `参照手順書` でその手順書にひも付く観測を集め、観測の `関連シナリオID` から既存シナリオ候補を控える。`seed_risk_id` は未定（手順1で確定）。
   - `INCIDENTS.tsv:N`／事象文字列 → `input_kind=incident`。行を解決し `seed_risk_id=INCIDENTS.tsv:N`。
2. **`ref/` 資料の確認**: 後続 agent が参照する対象システムの仕様・設計（`./ref` 配下）が揃っているか確認する。本リポジトリでは `ref/` は通常ディレクトリとして同梱されている（submodule ではない）。不足するときは捏造せず、縮退実行するか補完を待つかを**操作者に確認**してから決める（skillが無言で縮退判断しない）。縮退実行するときは `参照文書` を捏造せず、`ref/` 由来の肉付けが無かったことを内部ログに残す。
3. **着想（agent 93）**: `.ai/agents/93-conceive-scenario.md` に正規化入力＋`SCENARIO.tsv` 全行を渡し、`シナリオ` 本文・`元リスク事象ID`・`重複判定`・`参照した根拠` を得る。
4. **重複ゲート**: `重複判定` が `new` 以外（`duplicate-of:` ／ `consolidate-into:`）なら、**既定で停止**し、該当シナリオIDと根拠を操作者へ提示して選ばせる。
   - **スキップ**: 既にカバー済みとして何もせず終了する。
   - **統廃合**: 新しい `元リスク事象ID` を既存行の `元リスク事象ID`（`;` 連結）へ統合する。**新IDは採番しない**。既定では既存の3点は据え置き、操作者が望む場合のみ手順6〜9を**既存SCN-ID**に対して再実行する。統廃合後は手順10へ進む。
   - **強制新規**: 操作者が別個のシナリオだと判断した場合に限り、`new` として手順5へ進む。
   `new` ならそのまま手順5へ進む。これにより同じ入力で再実行しても重複生成しない（冪等）。
5. **SCN-ID採番＋行書き出し（決定的）**: `SCENARIO.tsv` の全 `SCN-NNN` の数値部の `max+1` を3桁ゼロ詰めして新IDにする（例 `SCN-035`）。IDは永続参照される識別子なので、欠番があっても詰めず、過去に使った番号は再利用しない（常に歴史上の最大 +1）。`SCENARIO.tsv` に `<新ID>` ＋ `シナリオ` ＋ `元リスク事象ID` の1行を追記し、`scenarios/<新ID>`（ヘッダ＋1行、既存の分割ファイル形式）を書き出す。手順書入力で `元リスク事象ID` が未確定なら、`OPS-CASE-NN`／`OPS-FREQ-NNN` を `SCENARIO.tsv` 走査で次番号採番する（93 の提案を採用。曖昧なら操作者に確認。根拠不足なら空欄）。
6. **理想タイムライン（agent 94）**: `.ai/agents/94-build-canonical-timeline.md` で `timelines/<新ID>/TIMELINE-1.tsv` を生成する（生成後に `EVAL_CRITERIA.md` で自己評価・改善まで 94 が行う）。
7. **想定質問集（agent 95）**: `.ai/agents/95-build-canonical-questions.md` で `timelines/<新ID>/QUESTIONS-1.tsv` を作る。
8. **想定回答集（agent 96）**: `.ai/agents/96-build-canonical-answers.md` で `timelines/<新ID>/ANSWERS-1.tsv` を作る。
9. **開示レビュー（agent 97）**: `.ai/agents/97-review-canonical-disclosure.md` で3点を RP-1〜RP-7 と開示基準に照らして点検し、その場修正する。97 が挙げた「要確認事項」（判断が割れる項目）があれば操作者へ提示する。
10. **一覧再生成**: `.ai/agents/90-build-scenario-list.md` を使って `SCENARIO-LIST.tsv` を**全件再生成**する（差分追記しない。並び・件数・初動対応部署を `SCENARIO.tsv` と整合させ、統廃合にも追従する）。
11. 追加結果（新ID、`元リスク事象ID`、3点のパス、要確認事項）を操作者へ要約する。

## 開示ルール

- `シナリオ` 概要・タイムラインの `開示不能情報`（列4）・先頭行の重大度・エスカレーション注記・想定質問・想定回答案・レビュー記録は内部情報として扱う。
- 参加者向け面に出てよいのは、`90-build-scenario-list` が抽出する `最初の開示可能イベント` の要約と`初動対応部署` だけ。タイムラインの参加者向け面（列2の能動通知本文・列3の `開示可能情報`）には観測された事実だけを書き、「〜が必要」等の手がかり・誘導・推測を入れない（RP-1）。
- このskillは演習を起動しない。IPC へは何も送らない。

## 個別デバッグ

各ステップは前段の出力ファイルを固定入力として単体で再実行できる（各 build agent の「個別デバッグ」節を参照）。例: `timelines/<ID>/QUESTIONS-1.tsv` と `TIMELINE-1.tsv` を固定して `96` だけ再実行する。ユーザーが明示的に求めない限り、前段ファイルを作り直してはいけない。
