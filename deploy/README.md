# ttx-slack デプロイ手順（Ubuntu / systemd）

Slack ゲートウェイ `ttx-slack` を Ubuntu（GCE 等）のサーバで常駐させる手順です。自前スーパーバイザ `ttx-slack-sup` が `ttx-slack` を監視し、その裏で演習 1 件ごとに `codex exec`（GM）を起動します。ゲートウェイの役割や運用上の要点はリポジトリ直下の [README.md「Slack で実施する」](../README.md#slack-で実施する実験的) を参照してください。

`ttx-slack`・`codex`・per-thread の Unix Domain Socket は **同一ホスト・同一ユーザー**で動かす必要があります（socket は uid 依存のローカル UDS）。本手順では専用ユーザー `ttx` を作成し、そのユーザーで codex を認証してサービスを実行します。

> macOS（Mac mini 運用）の場合は launchd を使います。[`io.github.kuniwak.ttx-slack.plist`](./io.github.kuniwak.ttx-slack.plist) のサンプルを参照してください。

## 前提

- Ubuntu（GCE 等）と `sudo` 権限。
- Slack アプリの **Bot トークン**（`xoxb-…`）と **App-Level トークン**（`xapp-…`, `connections:write`）、および通報用の **Incoming Webhook URL**。
- 外向きネットワーク疎通（Slack Socket Mode は outbound のみ・公開エンドポイント不要）。
- codex のサインインに使うアカウント（ChatGPT アカウント等。組織のポリシーに従って用意してください）。

以降の例では次のパスを使います（環境に合わせて読み替え可。変える場合は [パスを変える場合](#パスを変える場合) を参照）。

| 用途 | パス |
| --- | --- |
| サービスユーザー | `ttx`（ホーム `/home/ttx`） |
| リポジトリ配置先 | `/opt/ttx` |
| バイナリ | `/usr/local/bin/ttx-slack`, `/usr/local/bin/ttx-slack-sup` |
| codex バイナリ | `/home/ttx/.local/bin/codex` |
| トークン等の環境ファイル | `/etc/ttx-slack.env` |

## 1. サービスアカウント `ttx` を作成する

`ttx` は単なるデーモン用アカウントではなく、**codex の認証情報を置くホームディレクトリ**が必要です（codex は `$HOME` 配下に資格情報を保存し、systemd は `User=ttx` のとき `$HOME=/home/ttx` を passwd から自動設定します）。

```console
$ # 既存なら作り直さない（useradd は冪等でなく、再実行は user already exists で失敗する）
$ # --system: システムアカウント / --create-home: ホーム作成 / nologin: 直接ログイン不可
$ id ttx >/dev/null 2>&1 || sudo useradd --system --create-home --home-dir /home/ttx --shell /usr/sbin/nologin ttx

$ # リポジトリを配置先へ clone し、ttx 所有にする
$ sudo git clone https://github.com/Kuniwak/ttx /opt/ttx
$ sudo chown -R ttx:ttx /opt/ttx
```

リポジトリは `/opt/ttx` に置き、`ttx` が読み書きできるようにします（上記 `chown`）。取得方法（HTTPS / SSH / deploy key 等）は環境に合わせてください。

> nologin のままでも、運用は `sudo -u ttx -H …` で `ttx` として実行できます（`-H` で `HOME=/home/ttx` を設定）。

## 2. バイナリを配置する

`ttx-slack` / `ttx-slack-sup` はサーバ専用ビルドで、参加者向けインストーラ（`scripts/install-ttx.sh`）には含まれません。GitHub Release から対象 OS/アーキテクチャ（例: linux/amd64）のバイナリを取得し、`/usr/local/bin/` に配置します。

```console
$ sudo install -m 0755 ./ttx-slack     /usr/local/bin/ttx-slack
$ sudo install -m 0755 ./ttx-slack-sup /usr/local/bin/ttx-slack-sup
```

## 3. codex を `ttx` ユーザーで導入・認証する

公式インストーラは codex を実行ユーザーの `~/.local/bin` に入れます。`ttx` として実行することで `/home/ttx/.local/bin/codex` に入ります。

```console
$ # ttx として導入（公式スタンドアロンインストーラ。Homebrew/Node 不要）
$ sudo -u ttx -H bash -c 'curl -fsSL https://chatgpt.com/codex/install.sh | sh'

$ # 配置を確認
$ file ~/.local/bin/codex
.local/bin/codex: symbolic link to /home/ttx/.codex/packages/standalone/current/bin/codex
```

続いて `ttx` として codex にサインインします（`-H` で `HOME=/home/ttx` を設定。device-code フローは現在の TTY 上で進みます）。

```console
$ sudo -u ttx -H bash
$ cd
$ ~/.local/bin/codex
$ # codex にサインイン（device-code フローが使えないので Web login を選び、認証が完了して Web ブラウザが localhost 宛の URL を開いたら、その URL をコピーしてデプロイ先で `curl -sSfL 'https://localhost...'` を実行）

$ # workspace を trust しておく。
$ cd /opt/ttx
$ ~/.local/bin/codex
$ # 
```

> 対話シェルで作業したい場合は `sudo -u ttx -H bash` で `ttx` のシェルに入れます（nologin でも可）。


## 4. トークン等の環境ファイルを作成する

トークンは systemd（root）が特権降格前に読み込むため、`root` 所有・`0600` で十分です（`ttx` に読ませる必要はありません）。

```console
$ sudo install -m 0600 /dev/null /etc/ttx-slack.env
$ sudo tee /etc/ttx-slack.env >/dev/null <<'EOF'
SLACK_BOT_TOKEN=xoxb-REPLACE_ME
SLACK_APP_TOKEN=xapp-REPLACE_ME
SLACK_ALERT_WEBHOOK_URL=https://hooks.slack.com/services/REPLACE_ME
EOF
```

## 5. systemd ユニットを配置・有効化する

`ttx-slack.service` を所定の場所へコピーして有効化します。ユニットでは codex を `--codex-bin /home/ttx/.local/bin/codex` と**絶対パス**で指定しています（理由は [PATH の話](#codex-が見つからないときpath-の話) を参照）。

```console
$ sudo cp deploy/ttx-slack.service /etc/systemd/system/ttx-slack.service
$ sudo systemctl daemon-reload
$ sudo systemctl enable --now ttx-slack
```

`Restart=no` かつ `WantedBy=multi-user.target` のため、**ブート時に 1 度だけ**起動します。再起動の管理（クラッシュ時の再起動・クラッシュループ検知）は `ttx-slack-sup` が担います。

## 6. 動作確認

```console
$ systemctl status ttx-slack
$ journalctl -u ttx-slack -f          # ログを追う
```

Slack のチャンネルで `@ttx` にメンションして演習が始まれば疎通 OK です。演習が始まらない場合は、まず `ttx` で codex がサインイン済みか（[codex の認証エラー](#codex-の認証エラー)）、次に codex のパス解決（[codex が見つからないとき（PATH の話）](#codex-が見つからないときpath-の話)）を疑います。
