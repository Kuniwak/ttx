package cmd

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
)

type Options struct {
	CommonOptions *tools.CommonOptions
	BotToken      string
	AppToken      string
	MaxSessions   int
	IdleTimeout   time.Duration
	MaxRuntime    time.Duration
	RecvTimeout   time.Duration
	CodexBin      string
	CodexExecArgs []string
	RepoDir       string
	SocketDir     string
	StateDir      string
}

// stringSliceFlag は同名フラグの繰り返し指定を順序保持で集める flag.Value です。
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, " ") }

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// defaultCodexExecArgs は --codex-exec-arg 省略時に codex exec の "exec" とプロンプトの間へ
// 挿入する既定の引数列です。GM は per-thread の UDS へ接続するためサンドボックスは
// danger-full-access が必須で、ライブ進行のテンポのため reasoning effort を low にします。
// 毎回新しいスライスを返し、共有による破壊を避けます。
func defaultCodexExecArgs() []string {
	return []string{"-s", "danger-full-access", "-c", `model_reasoning_effort="low"`}
}

func ParseOptions(args []string, inout *cli.ProcInout) (*Options, error) {
	flags := flag.NewFlagSet("ttx-slack", flag.ContinueOnError)
	flags.SetOutput(inout.Stderr)
	commonRawOptions := &tools.CommonRawOptions{}
	tools.DeclareCommonOptions(flags, commonRawOptions)

	botToken := flags.String("bot-token", inout.Env("SLACK_BOT_TOKEN"), "Slack Bot トークン（xoxb-…）。省略時は環境変数 SLACK_BOT_TOKEN")
	appToken := flags.String("app-token", inout.Env("SLACK_APP_TOKEN"), "Slack App-Level トークン（xapp-…）。省略時は環境変数 SLACK_APP_TOKEN")
	maxSessions := flags.Int("max-sessions", 3, "同時に実行する演習の最大数（billing DoS 防止）")
	idleTimeout := flags.Duration("idle-timeout", 60*time.Minute, "最終操作からこの時間で演習を自動終了する")
	maxRuntime := flags.Duration("max-runtime", 120*time.Minute, "1演習の最大実行時間（0 で無効）")
	recvTimeout := flags.Duration("recv-timeout", 30*time.Minute, "参加者入力をこの時間待っても無ければ GM を abort 相当で終了させる（0 で無期限）")
	codexBin := flags.String("codex-bin", "codex", "GM を起動するコマンド")
	var codexExecArgs stringSliceFlag
	flags.Var(&codexExecArgs, "codex-exec-arg", `codex exec の "exec" とプロンプトの間に挿入する引数（反復指定可）。省略時の既定: -s danger-full-access -c model_reasoning_effort="low"。指定した場合は指定値のみを使う（既定は付与しない）。例: --codex-exec-arg -s --codex-exec-arg workspace-write --codex-exec-arg --model --codex-exec-arg gpt-5-codex`)
	repoDir := flags.String("repo-dir", ".", "演習リポジトリの作業ディレクトリ")
	socketDir := flags.String("socket-dir", "/tmp", "per-thread の Unix Domain Socket を置くディレクトリ（パス長制限のため短いパス推奨）")
	stateDir := flags.String("state-dir", defaultStateDir(inout.Env), "クラッシュ復旧用の状態ファイルを置くディレクトリ")

	flags.Usage = func() {
		w := inout.Stderr
		fmt.Fprintln(w, "Usage: ttx-slack [--bot-token <xoxb>] [--app-token <xapp>] [options]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Slack（Socket Mode）から障害対応机上演習を駆動するゲートウェイ。@ttx への")
		fmt.Fprintln(w, "メンションで演習を開始し、そのスレッドで質疑応答する。裏で codex exec を起動し、")
		fmt.Fprintln(w, "同時実行数を max-sessions に制限する。Codex 認証済みの同一ホストで動かすこと。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		flags.PrintDefaults()
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

	// --codex-exec-arg は省略時のみ既定を適用し、指定時は指定値のみを使う（完全上書き）。
	// 1回でも指定があれば既定は付与しない。空文字 "" の指定は「空引数1個」であり既定クリアではない。
	execArgs := []string(codexExecArgs)
	if len(execArgs) == 0 {
		execArgs = defaultCodexExecArgs()
	}

	return &Options{
		CommonOptions: commonOptions,
		BotToken:      *botToken,
		AppToken:      *appToken,
		MaxSessions:   *maxSessions,
		IdleTimeout:   *idleTimeout,
		MaxRuntime:    *maxRuntime,
		RecvTimeout:   *recvTimeout,
		CodexBin:      *codexBin,
		CodexExecArgs: execArgs,
		RepoDir:       *repoDir,
		SocketDir:     *socketDir,
		StateDir:      *stateDir,
	}, nil
}

// defaultStateDir はクラッシュ復旧用状態ファイルの既定ディレクトリを返します。
// 再起動をまたいで残る必要があるため $XDG_STATE_HOME / HOME 配下を優先します。
func defaultStateDir(getenv func(string) string) string {
	if d := getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "ttx-slack")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".ttx", "ttx-slack")
	}
	return filepath.Join(os.TempDir(), "ttx-slack")
}
