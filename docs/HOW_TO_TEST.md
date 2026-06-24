テスト方法
=========

テストの実行方法
----------------

```console
$ go test ./...
```


バイナリを使う方法
------------------

多くの場合は `go run ./tools/<id>/main.go` で十分です。
ビルドされたバイナリが必要な場合は Goreleaser を使って `./bin` 配下にビルドしてから利用してください。

```console
$ goreleaser build --clean --snapshot --single-target --id <id> -o ./bin
$ ls ./bin
<id>
```


会話用プロセス用バイナリの取得（操作者向け）
--------------------------------------------

操作者（Go 環境が無い場合を含む）は `scripts/install-ttx.sh` で `ttx-human` /
`ttx-ai-send` / `ttx-ai-recv` を取得・配置できます。GitHub Release のビルド済み
バイナリを使うため Go は不要です（取得には gh CLI を使い、未導入なら自動導入を試みます）。

```console
$ bash scripts/install-ttx.sh
```

テスト時は環境変数で挙動を上書きできます。

* `TTX_REPO`: 取得元リポジトリ（既定 `Kuniwak/ttx`）
* `TTX_VERSION`: 取得する Release タグ（既定は最新 Release）
* `TTX_INSTALL_DIR`: 配置先（既定は PATH 上の書込可ディレクトリ、無ければ `~/.local/bin`）

```console
$ TTX_INSTALL_DIR=$(mktemp -d) TTX_VERSION=0.0.1 bash scripts/install-ttx.sh
```
