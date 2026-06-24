// ttx-ai-send は GM（Coding Agent）が参加者へ1メッセージ送るための極小 CLI です。
// 詳しい使い方・例・終了コードは -h（Usage）を参照してください。
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools"
	"github.com/Kuniwak/ttx/ttxipc"
	"github.com/Kuniwak/ttx/version"
)

const (
	exitOK    = 0
	exitIO    = 2
	exitUsage = 3
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
	// ttxipc.Send の Dial が即エラー（接続失敗=2）を返すので、ここでは事前チェックしない。
	sendType, err := ttxipc.ParseSendType(options.Type)
	if err != nil {
		return tools.NewExitError(exitUsage, err)
	}

	body, err := resolveText(options, inout.Stdin)
	if err != nil {
		return err
	}

	if err := ttxipc.Send(options.Socket, ttxipc.Message{Type: string(sendType), Text: body, Thinking: options.Thinking}); err != nil {
		return tools.NewExitError(exitIO, err)
	}
	return nil
}

// resolveText は本文を --text / --text-file / stdin のいずれかから取得します。
// --text と --text-file の同時指定は不正、本文が空も不正です。
func resolveText(options *Options, stdin io.Reader) (string, error) {
	var body string
	switch {
	case options.TextProvided && options.TextFileProvided:
		return "", tools.NewExitError(exitUsage, errors.New("--text と --text-file は同時に指定できません"))
	case options.TextProvided:
		// --text はシェルのシングルクォートで \n が素通しされるため、ここで Go 形式の
		// エスケープを展開する。リテラルなバックスラッシュを残したい長文は --text-file・
		// stdin（どちらも byte-faithful）を使う。
		body = unescapeText(options.Text)
	case options.TextFileProvided:
		b, err := os.ReadFile(options.TextFile)
		if err != nil {
			return "", tools.NewExitError(exitIO, fmt.Errorf("本文ファイルを読めません: %w", err))
		}
		body = string(b)
	default:
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", tools.NewExitError(exitIO, fmt.Errorf("stdin から本文を読めません: %w", err))
		}
		body = string(b)
	}

	if strings.TrimSpace(body) == "" {
		return "", tools.NewExitError(exitUsage, errors.New("本文が空です（--text / --text-file / stdin のいずれかで指定してください）"))
	}
	return body, nil
}
