package cmd

import (
	"errors"
	"flag"
	"fmt"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
	"github.com/Kuniwak/ttx/ttxipc"
)

type Options struct {
	CommonOptions *tools.CommonOptions
	Socket        string
	Wait          bool
}

func ParseOptions(args []string, inout *cli.ProcInout) (*Options, error) {
	flags := flag.NewFlagSet("ttx-ai-recv", flag.ContinueOnError)
	flags.SetOutput(inout.Stderr)
	commonRawOptions := &tools.CommonRawOptions{}
	tools.DeclareCommonOptions(flags, commonRawOptions)

	socket := flags.String("socket", ttxipc.DefaultSocketPath(inout.Env), "会話用プロセスの Unix Domain Socket パス（省略時は TTX_SOCKET か既定パス）")
	wait := flags.Bool("wait", true, "次の入力が来るまでブロックする（既定）")
	noWait := flags.Bool("no-wait", false, "滞留が無ければ即終了する（--wait より優先）")

	flags.Usage = func() {
		w := inout.Stderr
		fmt.Fprintln(w, "Usage: ttx-ai-recv [--socket <path>] [--wait | --no-wait]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "GM（Coding Agent）が次の参加者入力を1件受け取り、応答（input|control）を stdout に1行出す。")
		fmt.Fprintln(w, "socket は省略可（TTX_SOCKET か既定パスを使う）。既定は --wait（次入力までブロック）。--no-wait は滞留が無ければ何も出さず終了する。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, "  ttx-ai-recv            # 既定 socket・次入力までブロック")
		fmt.Fprintln(w, "  ttx-ai-recv --no-wait  # 滞留が無ければ即終了")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Exit codes: 0=入力を取得して出力 / 2=接続・送受信失敗 / 4=滞留無し / 5=待機タイムアウト（abort 相当・ポストモーテムなしで終了する）。")
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return &Options{CommonOptions: tools.HelpCommonOptions}, nil
		}
		// 不正なフラグは IPC 障害と同じ終了コードで弾く（移植元と同一契約）。
		return nil, tools.NewExitError(exitIO, err)
	}

	commonOptions, err := tools.ValidateCommonOptions(commonRawOptions)
	if err != nil {
		return nil, err
	}
	if commonOptions.Version {
		return &Options{CommonOptions: tools.VersionCommonOptions}, nil
	}

	return &Options{
		CommonOptions: commonOptions,
		Socket:        *socket,
		Wait:          *wait && !*noWait,
	}, nil
}
