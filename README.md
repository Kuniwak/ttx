# 障害対応机上演習ツール（ttx）

効果的な障害対応の机上演習（テーブルトップ演習）を実施するために、AI にゲームマスターになってもらうツール群です。AI が状況設定と段階的な情報開示を担い、対応者は原因の切り分けと対応手順の模索に集中できます。

- **対象システム（演習で扱う架空のシステム）**: [OpenTelemetry Demo](https://github.com/open-telemetry/opentelemetry-demo)（Apache-2.0 の公開マイクロサービスEC）を題材にしています。サービス構成・アーキテクチャは公開資料に基づいて `ref/` に同梱しています。
- **インシデント対応の土台**: [Google SRE Book / Workbook](https://sre.google/books/) の考え方（インシデント指揮体制、Severity、エスカレーション、コミュニケーション、ポストモーテム文化）を参照して独自に記述しています。


## これは何か

演習の人間向け入口は統合 skill `00-run-exercise` だけです。これを起動すると、ゲームマスター役の AI が演習全体を進行します。

演習では2つの画面を使い分けます。

- **ゲームマスター役の AI（Codex / Cursor / Claude など）の画面**: 思考過程や内部情報（隠れた原因・未来の展開）が映るため、**参加者からは隠します**。
- **会話用プロセス `ttx-human` の画面**: 別窓で起動し、参加者にはこちらだけを見せます。

AI は観測可能な事実だけを開示し、原因・影響の推測や次手順への誘導はしません（推論は対応者に委ねます）。

## セットアップ

> [!TIP]
> コーディングエージェント（**Codex 推奨**。Cursor・Claude Code など使い慣れたものでも可）にセットアップごと任せるのが一番かんたんです。

### 1. AI コーディングエージェントを用意する

応答が速いため Codex を推奨します。すでに使い慣れたエージェントがあればそれでも構いません。

- **Codex（macOS / Linux・Homebrew/Node 不要）**: `curl -fsSL https://chatgpt.com/codex/install.sh | sh`
- **Windows**: WSL 上で上記を実行するのが確実です。
- Codex を使う場合は ChatGPT アカウントが必要です（起動時にサインイン）。組織で利用する場合は所属組織のアカウント発行ポリシーに従ってください。

### 2. リポジトリを取得して演習を開始する

```sh
# 取得（公開リポジトリ。submodule はありません）
gh repo clone Kuniwak/ttx
cd ttx

# 演習を開始（エージェントに以下を伝えるか、/00-run-exercise を実行）
#   「.ai/skills/00-run-exercise/SKILL.md に従って演習を始めて」
```

会話用プロセスのバイナリ（`ttx-human` など）は、リリース取得スクリプト `scripts/install-ttx.sh` で導入されます。リリースが未公開の間は、ローカルでビルドして使えます。

```sh
go build -o ttx-human ./tools/ttx-human
go build -o ttx-ai-send ./tools/ttx-ai-send
go build -o ttx-ai-recv ./tools/ttx-ai-recv
```

### 3. 応答を速くする（任意）

演習のライブ進行（参加者との質疑応答）はテンポが重要です。Codex で動かす場合は、起動直後に reasoning effort を `low` に下げておくとテンポよく進みます（質疑が始まるとゲームマスター画面はネタバレ防止で隠れて操作できなくなるため、起動直後に設定します）。演習後は元の設定に戻してください。

## 使い方

1. 起動すると、会話に必要なものが自動で用意されます。
2. **訓練モード**（既定は**全体訓練**＝対応者1人で全ロールの対応を担う／**個別訓練**＝自ロールだけを担当し、それ以外は AI が NPC として能動対応する）と、シナリオの選び方（既定は対応者の所属ロールによるおすすめ選択。ほかにシナリオIDの明示指定、無作為選定）、準備モード（事前生成済みシナリオの**再利用＝推奨**か、その場で作る**アドホック生成**か）を確認します。シナリオ一覧は `01b-list-scenarios` で確認できます。
3. **新しいターミナルの窓（別窓）で `ttx-human` を起動**するよう案内され、起動確認を求められます。
4. 続いて「ゲームマスター側の思考過程・内部情報が映る画面を参加者から隠す必要がある」ことが提示され、10秒のカウントダウンののち先へ進みます。以降は AI が自動で進行します。
5. 参加者とのやりとりは会話用プロセス `ttx-human` 経由で行います。
6. タイムラインを最後まで進めるか `/giveup` で終了すると、ポストモーテムが提示されます（`/abort` で中断した場合はポストモーテムは作成されません）。

## 参加者が使えるコマンド

| 入力 | 動作 |
| --- | --- |
| `/help` | 利用可能なコマンド一覧を表示します。会話プロセスがローカルで即時応答し、GM（コーディングエージェント）には送られません。 |
| `/giveup` | お手上げを宣言して演習を終了し、ポストモーテム生成へ進みます。`control` として GM へ送られます。 |
| `/abort` | 演習を中断します。`/giveup` と異なり**ポストモーテムを作成しません**。`control` として GM へ送られます。 |
| `/claim <主張>` | GM の前提に異議を申し立て、GM にその前提を一旦取り下げさせて演習を進めます。受け入れた前提は後で真偽を確認すべき項目としてポストモーテムの「要確認事項」に記録されます。本文なしで実行すると GM が争点を尋ね、次の入力を主張として扱います。`control` として GM へ送られます。 |
| 上記以外 | そのまま GM へ送られます。質問でも対応行動でも構いません。 |

## 対象システムとシナリオ

- `ref/`: 対象システム（OpenTelemetry Demo）のサービスカタログ・アーキテクチャ・障害解説（公開資料を典拠に独自記述）。
- `ops/`: インシデント対応マニュアル（`ops/incident-handling-manual.md`）と各シナリオの対応手順（runbook）。Google SRE の考え方が土台です。
- `SCENARIO.tsv` / `scenarios/` / `timelines/<ID>/` / `OBSERVATION.tsv` / `INCIDENTS.tsv`: シナリオ定義・事前生成タイムライン・観測カタログ・参照インシデント。

新しいシナリオの追加は、1つの入力（`ops/<name>.md` 障害対応手順書 / `INCIDENTS.tsv` の1行）から `.ai/skills/92-build-scenario/SKILL.md` で1件ずつ行います。データ形式は [`.ai/shared/references/data-formats.md`](./.ai/shared/references/data-formats.md) を参照してください。

## Slack で実施する（実験的）

ターミナルの代わりに **Slack** で演習を実施するためのゲートウェイ `ttx-slack` があります。チャンネルで `@ttx` にメンションすると演習が始まり、生成されたスレッド内で `@ttx` にメンションして質疑応答します。デプロイ手順は [`deploy/`](./deploy) を参照してください。

```sh
ttx-slack --repo-dir /path/to/ttx --max-sessions 3
#   SLACK_BOT_TOKEN / SLACK_APP_TOKEN を環境変数で渡す
```

## ポストモーテムの評価基準

[EVAL_CRITERIA.md](./EVAL_CRITERIA.md) を参照してください。

## 詳細仕様

システム仕様（背景・Skill構成・入力・シナリオ選定・ステップ・中間ファイル・各種フォーマット）: [docs/SPEC.md](./docs/SPEC.md)

## ライセンスと帰属

- 本リポジトリは [Apache License 2.0](./LICENSE) で配布します。
- 題材システムは [OpenTelemetry Demo](https://github.com/open-telemetry/opentelemetry-demo)（Apache-2.0, © The OpenTelemetry Authors）に基づきます。`ref/` の記述は公開ドキュメントを典拠にした独自記述で、原文の転載ではありません。
- インシデント対応の枠組みは [Google SRE Book / Workbook](https://sre.google/books/) を参照して独自に記述しています。
