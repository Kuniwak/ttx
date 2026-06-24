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
	CommonOptions    *tools.CommonOptions
	Socket           string
	Type             string
	Text             string
	TextFile         string
	TextProvided     bool
	TextFileProvided bool
	Thinking         bool
}

func ParseOptions(args []string, inout *cli.ProcInout) (*Options, error) {
	flags := flag.NewFlagSet("ttx-ai-send", flag.ContinueOnError)
	flags.SetOutput(inout.Stderr)
	commonRawOptions := &tools.CommonRawOptions{}
	tools.DeclareCommonOptions(flags, commonRawOptions)

	socket := flags.String("socket", ttxipc.DefaultSocketPath(inout.Env), "会話用プロセスの Unix Domain Socket パス（省略時は TTX_SOCKET か既定パス）")
	typ := flags.String("type", "", "メッセージ種別 (answer|notification|question|postmortem|error)")
	text := flags.String("text", "", "メッセージ本文（--text-file・stdin と排他）。\\n 等の Go 形式エスケープを実文字へ展開する")
	textFile := flags.String("text-file", "", "本文をこのファイルから読む（長文ポストモーテム向け）")
	thinking := flags.Bool("thinking", false, "表示後も思考中インジケータを点けたままにする（演習開始通知など、直後に長い準備が続く場合。次のフラグなしメッセージで解除）")

	flags.Usage = func() {
		w := inout.Stderr
		fmt.Fprintln(w, "Usage: ttx-ai-send [--socket <path>] --type <answer|notification|question|postmortem|error> [--text <s> | --text-file <path>]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "GM（Coding Agent）が参加者へ1メッセージ送る。接続＝1回の req/resp で完結する。")
		fmt.Fprintln(w, "socket は省略可（TTX_SOCKET か既定パスを使う）。本文は --text / --text-file / stdin のいずれかで与える。")
		fmt.Fprintln(w, "--text は \\n 等の Go 形式エスケープを実文字へ展開する（複数行を1行で書ける）。リテラルな")
		fmt.Fprintln(w, "バックスラッシュを保ちたい長文は --text-file・stdin を使う（どちらも生バイトのまま送る）。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, "  ttx-ai-send --type answer --text '本日 v2.3.1 をデプロイ済みです。'        # 既定 socket")
		fmt.Fprintln(w, "  ttx-ai-send --type question --text '1. おすすめ\\n2. 一覧から選ぶ'        # \\n が改行になる")
		fmt.Fprintln(w, "  ttx-ai-send --socket /path/conv.sock --type postmortem --text-file postmortem.md  # socket 明示")
		fmt.Fprintln(w, "  ttx-ai-send --type notification --thinking --text '演習を開始します。少々お待ちください。'  # 表示後も思考中表示を維持")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Exit codes: 0=送信成功 / 2=接続・書き込み失敗 / 3=引数不正。")
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

	o := &Options{
		CommonOptions: commonOptions,
		Socket:        *socket,
		Type:          *typ,
		Text:          *text,
		TextFile:      *textFile,
		Thinking:      *thinking,
	}
	// --text / --text-file の指定有無は本文解決の分岐に使う（既定値と区別する）。
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "text":
			o.TextProvided = true
		case "text-file":
			o.TextFileProvided = true
		}
	})
	return o, nil
}
