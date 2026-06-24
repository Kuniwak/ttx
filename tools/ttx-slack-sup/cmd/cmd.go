// ttx-slack-sup は ttx-slack を監視する自前のスーパーバイザ（hypervisor）です。
//
// 監視対象を起動し、終了するたび backoff を挟んで再起動します。ただし window 内で
// max-restarts 回終了が続いたらクラッシュループとみなして監視を停止し、Slack incoming
// webhook（SDK 非依存の生 HTTP）へエラーと直近の標準エラーを通報します。これにより
// launchd/systemd の素朴な KeepAlive による無限再起動（計算機資源の浪費）を避けます。
// init システムからは本プロセスを起動1回のみ登録します（クラッシュループ停止を最終とするため）。
//
//	ttx-slack-sup [options] -- <command> [args...]
package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
	"github.com/Kuniwak/ttx/version"
)

func MainCommandByArgs(args []string, inout *cli.ProcInout) int {
	options, err := ParseOptions(args, inout)
	if err != nil {
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

	if len(options.Target) == 0 {
		return fmt.Errorf("監視対象コマンドを `--` の後に指定してください（例: -- ttx-slack --repo-dir ...）")
	}

	// webhook 未設定なら標準エラーへ通報（呼び出し側で経路を明示的に選ぶ。nil フォールバックは設けない）。
	var alert alertFunc
	if options.WebhookURL != "" {
		alert = webhookAlerter(options.WebhookURL)
	} else {
		alert = stderrAlerter(inout.Stderr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	supervise(ctx, execRunner(options.Target), alert, time.Now, time.Sleep, supervisePolicy{
		maxRestarts: options.MaxRestarts,
		window:      options.Window,
		backoff:     options.Backoff,
	})
	return nil
}
