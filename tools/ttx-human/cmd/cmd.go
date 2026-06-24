// ttx-human は障害対応机上演習の会話用プロセスです。
//
// Unix Domain Socket を1本 listen し、接続ごとに1回の req/resp を処理します。
// GM（Coding Agent）からの送信（answer|notification|question|postmortem|error）は人間へ表示し、
// recv 要求には未回収の参加者入力を1件返します。接続が閉じても終了せず、
// 人間側入力（stdin）の EOF で正常終了します。
//
//	ttx-human [--socket <path>] [--public-log <path>]
package cmd

import (
	"fmt"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
	"github.com/Kuniwak/ttx/version"
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

	return 0
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
	// net.Listen が即エラーを返すので、ここで防御的な事前チェックはしない。
	cfg := Config{SocketPath: options.Socket, PublicLog: options.PublicLog, RecvTimeout: options.RecvTimeout}
	return serveSocket(cfg, inout.Stdin, inout.Stdout, inout.Stderr)
}
