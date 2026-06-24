リリース方法
==========

```console
$ git checkout master
$ git pull origin master

$ actionlint && ghalint run
$ go test ./...

$ # セマンティックバージョンにしたがってバージョンを上げる
$ edit ./version/version.go

$ git add ./version/version.go
$ # x.x.x は上げた後のバージョンを記載する
$ git commit -m "Ready to be vx.x.x"

$ git push
$ git tag x.x.x
$ git push --tags
```

タグを push すると GitHub Actions（GoReleaser）がクロスプラットフォームのバイナリを
ビルドし、GitHub Release を作成します（`.goreleaser.yaml`）。

Homebrew tap など別の配布チャネルを使う場合は、その formula の checksum と URL の
バージョンを更新します。配布チャネルは公開時に確定するため、未公開の間はこの手順は
省略してかまいません。
