package cmd

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
	"github.com/Kuniwak/ttx/ttxipc"
)

type Options struct {
	CommonOptions *tools.CommonOptions
	Socket        string
	PublicLog     string
	RecvTimeout   time.Duration
}

func ParseOptions(args []string, inout *cli.ProcInout) (*Options, error) {
	flags := flag.NewFlagSet("ttx-human", flag.ContinueOnError)
	flags.SetOutput(inout.Stderr)
	commonRawOptions := &tools.CommonRawOptions{}
	tools.DeclareCommonOptions(flags, commonRawOptions)

	socket := flags.String("socket", ttxipc.DefaultSocketPath(inout.Env), "Unix Domain Socket のパス（省略時は TTX_SOCKET か既定パス）")
	publicLog := flags.String("public-log", "public_log.jsonl", "公開IPCログのパス")
	recvTimeout := flags.Duration("recv-timeout", 30*time.Minute, "参加者入力をこの時間待っても無ければ recv を打ち切る（GM は abort 相当で終了。0 で無期限）")

	flags.Usage = func() {
		w := inout.Stderr
		fmt.Fprintln(w, "Usage: ttx-human [--socket <path>] [--public-log <path>] [--recv-timeout <dur>]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "障害対応机上演習の会話用プロセス。socket を1本 listen し、接続ごとに送信1件または")
		fmt.Fprintln(w, "受信1件の req/resp を処理する。接続が閉じても終了せず、人間側 stdin の EOF で終了する。")
		fmt.Fprintln(w, "socket は省略可（TTX_SOCKET か既定パスを使う）。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, "  ttx-human --public-log public_log.jsonl              # 既定 socket")
		fmt.Fprintln(w, "  ttx-human --socket /path/conv.sock                  # socket 明示")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "GM 側は ttx-ai-send / ttx-ai-recv で同じ socket を使って送受信する。")
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return &Options{CommonOptions: tools.HelpCommonOptions}, nil
		}
		return nil, err
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
		PublicLog:     *publicLog,
		RecvTimeout:   *recvTimeout,
	}, nil
}
