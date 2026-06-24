package ttxipc

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvSocket は --socket 省略時に既定パスを上書きする環境変数名です（Docker の DOCKER_HOST 相当）。
const EnvSocket = "TTX_SOCKET"

// DefaultSocketPath は --socket 省略時に使う既定の Unix Domain Socket パスを返します。
// 優先順位は次の通りで、いずれも CWD に依存しない絶対パスです:
//
//	TTX_SOCKET > $XDG_RUNTIME_DIR/ttx.sock > $TMPDIR/ttx-<uid>.sock
//
// $XDG_RUNTIME_DIR（Linux の /run/user/<uid>）はユーザー単位ランタイム socket の標準置き場です。
// フォールバックは os.TempDir()（macOS はユーザー別 $TMPDIR、Linux は共有 /tmp）配下に
// uid を付けて共有 /tmp での衝突を避けます。getenv は inout.Env を渡せばテストできます（nil 不可）。
//
// 既定パスは uid と環境変数から算出するため、相互に通信する3ツールは同一ユーザー・同一環境で
// 動かす必要があります（別ユーザーや sudo を跨ぐ場合は TTX_SOCKET か --socket で揃える）。
// Windows では os.Getuid() が -1 を返すため既定パスは best-effort です。
func DefaultSocketPath(getenv func(string) string) string {
	if s := getenv(EnvSocket); s != "" {
		return s
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "ttx.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ttx-%d.sock", os.Getuid()))
}
