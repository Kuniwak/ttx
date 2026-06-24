package cmd

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
)

type Options struct {
	CommonOptions *tools.CommonOptions
	WebhookURL    string
	MaxRestarts   int
	Window        time.Duration
	Backoff       time.Duration
	Target        []string // 監視対象コマンド（`--` 以降の引数）
}

func ParseOptions(args []string, inout *cli.ProcInout) (*Options, error) {
	flags := flag.NewFlagSet("ttx-slack-sup", flag.ContinueOnError)
	flags.SetOutput(inout.Stderr)
	commonRawOptions := &tools.CommonRawOptions{}
	tools.DeclareCommonOptions(flags, commonRawOptions)

	webhookURL := flags.String("webhook-url", inout.Env("SLACK_ALERT_WEBHOOK_URL"), "監視停止を通報する Slack incoming webhook の URL。省略時は環境変数 SLACK_ALERT_WEBHOOK_URL（未設定なら標準エラーへ通報）")
	maxRestarts := flags.Int("max-restarts", 5, "window 内でこの回数の終了が続いたらクラッシュループと判断して監視停止")
	window := flags.Duration("window", 10*time.Minute, "終了回数を数える時間窓")
	backoff := flags.Duration("backoff", 2*time.Second, "再起動前の待機")

	flags.Usage = func() {
		w := inout.Stderr
		fmt.Fprintln(w, "Usage: ttx-slack-sup [options] -- <command> [args...]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "監視対象（通常 ttx-slack）を起動し、終了するたび再起動する自前のスーパーバイザ。")
		fmt.Fprintln(w, "window 内で max-restarts 回終了が続いたらクラッシュループとみなして監視を停止し、")
		fmt.Fprintln(w, "Slack incoming webhook（SDK 非依存の生 HTTP）でエラーと直近の標準エラーを通報する。")
		fmt.Fprintln(w, "無限再起動による計算機資源の浪費を防ぐ。launchd/systemd からは本プロセスを1回だけ起動する。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Example:")
		fmt.Fprintln(w, "  ttx-slack-sup --webhook-url https://hooks.slack.com/... -- ttx-slack --repo-dir /opt/ttx")
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
		WebhookURL:    *webhookURL,
		MaxRestarts:   *maxRestarts,
		Window:        *window,
		Backoff:       *backoff,
		Target:        flags.Args(),
	}, nil
}
