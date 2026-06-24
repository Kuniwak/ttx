// ttx-slack は障害対応机上演習を Slack（Socket Mode）から駆動するゲートウェイです。
//
// チャンネルで @ttx にメンションすると演習を開始し、生成されたスレッドで質疑応答します。
// 裏で演習1件ごとに codex exec を起動し、同時実行数を制限します（billing DoS 防止）。
// per-thread の Unix Domain Socket・codex・ゲートウェイは同一ホストで動かす必要があります。
//
//	ttx-slack [--bot-token <xoxb>] [--app-token <xapp>] [options]
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/slackbot"
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

	if options.BotToken == "" || options.AppToken == "" {
		return fmt.Errorf("Slack トークンが必要です（--bot-token/--app-token か環境変数 SLACK_BOT_TOKEN/SLACK_APP_TOKEN）")
	}

	client, err := slackbot.New(options.BotToken, options.AppToken)
	if err != nil {
		return err
	}

	spawner := newExecCodexSpawner(options.CodexBin, options.RepoDir, options.CodexExecArgs)
	g := newGateway(client, spawner, options.SocketDir, options.StateDir,
		options.MaxSessions, options.IdleTimeout, options.MaxRuntime, options.RecvTimeout, time.Now)

	// 起動時に前回の残骸を回収する（孤児 Codex の停止・stale socket 削除・再起動通知）。
	g.Reconcile()

	// SIGINT/SIGTERM で graceful shutdown。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 放置・最大実行時間の定期回収。
	go g.reapLoop(ctx)

	fmt.Fprintln(inout.Stderr, "ttx-slack: Socket Mode 接続を開始します")
	err = client.Run(ctx, func(m slackbot.Mention) {
		g.HandleMention(MentionEvent{
			ChannelID: m.ChannelID,
			ThreadTS:  m.ThreadTS,
			MessageTS: m.MessageTS,
			UserText:  m.UserText,
		})
	})
	g.shutdown()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
