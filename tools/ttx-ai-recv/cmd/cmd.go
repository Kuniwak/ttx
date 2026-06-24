// ttx-ai-recv は GM（Coding Agent）が次の参加者入力を1件受け取るための極小 CLI です。
// 詳しい使い方・例・終了コードは -h（Usage）を参照してください。
package cmd

import (
	"errors"
	"fmt"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
	"github.com/Kuniwak/ttx/ttxipc"
	"github.com/Kuniwak/ttx/version"
)

const (
	exitOK      = 0
	exitIO      = 2
	exitEmpty   = 4
	exitTimeout = 5
)

func MainCommandByArgs(args []string, inout *cli.ProcInout) int {
	options, err := ParseOptions(args, inout)
	if err != nil {
		// フラグのパースエラーは flag が Usage を出力済みなので、ここでは再出力しない。
		return tools.ExitCodeOf(err)
	}

	if err := MainCommandByOptions(options, inout); err != nil {
		fmt.Fprintln(inout.Stderr, err)
		return tools.ExitCodeOf(err)
	}

	return exitOK
}

func MainCommandByOptions(options *Options, inout *cli.ProcInout) error {
	if options.CommonOptions.Help {
		return nil
	}

	if options.CommonOptions.Version {
		fmt.Fprintln(inout.Stdout, version.Version)
		return nil
	}

	// socket は既定パスを持つため通常は非空。空（--socket "" 明示）の場合は
	// ttxipc.Recv の Dial が即エラー（接続失敗=2）を返すので、ここでは事前チェックしない。
	msg, ok, err := ttxipc.Recv(options.Socket, options.Wait)
	if err != nil {
		return tools.NewExitError(exitIO, err)
	}
	if !ok {
		return tools.NewExitError(exitEmpty, errors.New("受け取れる入力がありません"))
	}
	if msg.Type == ttxipc.TypeTimeout {
		// アイドルタイムアウト。GM 側はこれを abort 相当（ポストモーテムなし）の打ち切りとして扱う。
		return tools.NewExitError(exitTimeout, errors.New("入力待ちが規定時間を超えたため打ち切りました"))
	}
	if err := ttxipc.WriteLine(inout.Stdout, msg); err != nil {
		return tools.NewExitError(exitIO, err)
	}
	return nil
}
