//go:build windows

package cmd

import (
	"io"

	"github.com/Kuniwak/ttx/ttxserver"
)

// Windows は raw mode のキー処理が異なるため対話UIは未対応とし、常に dumb に倒す（回帰なし）。
func tryTTYFrontend(stdin io.Reader, stdout io.Writer) (ttxserver.Frontend, bool) {
	return nil, false
}
