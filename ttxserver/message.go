// Package ttxserver は ttx-human（会話用プロセス）の socket サーバ中核です。
//
// Unix Domain Socket を1本 listen し、接続ごとに1回の req/resp を処理します。
// GM（Coding Agent）からの送信（answer|notification|question|postmortem|error）は Frontend へ表示し、
// recv 要求には未回収の参加者入力を1件返します。表示先（端末・Slack など）は Frontend
// 抽象で差し替え可能で、本パッケージは表示の実体を持ちません。
package ttxserver

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/Kuniwak/ttx/jsonl"
	"github.com/Kuniwak/ttx/ttxipc"
)

const (
	cmdHelp   = "/help"
	cmdGiveup = "/giveup"
	cmdClaim  = "/claim"
	cmdAbort  = "/abort"
)

// announceText は演習で使ってよい道具を参加者へ明示する案内文です。
// 起動時バナーと /help の両方がこの唯一の定義元を参照します（重複を避ける）。
const announceText = "この演習では、本番の障害対応で使ってよいものは何でも使ってかまいません" +
	"（社内ドキュメント、Runbook、検索、この演習システム以外のAIツールなど）。" +
	"実際の障害対応と同じ環境・道具で臨んでください。"

// abortFarewellText は /abort 受付時に会話プロセスがローカルに表示する別れの文言です。
// 自動終了で GM の中断通知が表示されないため、参加者へ終了を伝える目的でローカルに出します。
// 文言は GM の中断通知（00-run-exercise SKILL ステップ13）と揃えます。
const abortFarewellText = "演習を中断しました。ポストモーテムは作成しません。"

// Command は参加者が使える組み込みコマンドです。
type Command struct {
	Name    string   // コマンド名（例: "/help"）
	Alias   []string // 別名（例: 会話UIの "?"）
	Usage   string   // ヘルプに表示する説明
	Control bool     // true なら type:"control" として GM へ送る演習制御コマンド
}

// Commands は参加者が使える組み込みコマンドの定義元です。
// ヘルプ表示はこの一覧をループして生成するため、コマンドと説明の唯一の定義元となります。
// 会話UI（issue #3）の `?` 表示もこの一覧を再利用します（`?` は /help の別名）。
var Commands = []Command{
	{Name: cmdHelp, Alias: []string{"?"}, Usage: "このヘルプを表示します（GMには送信されません）。"},
	{Name: cmdGiveup, Control: true, Usage: "演習を終了し、ポストモーテム生成へ進みます。"},
	{Name: cmdClaim, Control: true, Usage: "前提に異議を申し立て、GMに一旦受け入れさせて進めます（本文なしでGMが争点を尋ねます）。"},
	{Name: cmdAbort, Control: true, Usage: "演習を中断します。ポストモーテムは作成しません。"},
}

// HelpText は Commands をループして利用可能なコマンド一覧を組み立てます。
// /help のローカル応答と、表示の見た目を検証するゴールデンテストが参照します。
func HelpText() string {
	var b strings.Builder
	b.WriteString("利用可能なコマンド:\n")
	for _, c := range Commands {
		name := c.Name
		if len(c.Alias) > 0 {
			name += " (" + strings.Join(c.Alias, ", ") + ")"
		}
		fmt.Fprintf(&b, "  %-14s %s\n", name, c.Usage)
	}
	b.WriteString("\n上記コマンド以外は、そのまま入力してください。質問でも対応行動でも構いません。\n")
	b.WriteString("\n" + announceText + "\n")
	return b.String()
}

// matchCommand は入力に対応する組み込みコマンドを返します。一致しなければ nil。
func matchCommand(trimmed string) *Command {
	for i := range Commands {
		c := &Commands[i]
		if isCommand(trimmed, c.Name) {
			return c
		}
		for _, a := range c.Alias {
			if isCommand(trimmed, a) {
				return c
			}
		}
	}
	return nil
}

// HumanMessage は /help 以外の人間入力をIPCメッセージに変換します。
// 制御コマンド（Commands の Control:true）は type:"control"、それ以外はすべて
// type:"input" とし、質問か行動かの判定はGMに委ねます（スラッシュの有無では決めません）。
func HumanMessage(text string) ttxipc.Message {
	trimmed := strings.TrimSpace(text)
	if cmd := matchCommand(trimmed); cmd != nil && cmd.Control {
		return ttxipc.Message{Type: "control", Text: trimmed}
	}
	return ttxipc.Message{Type: "input", Text: trimmed}
}

// isCommand は trimmed が cmd と完全一致するか、cmd の直後が空白
// （例: "/giveup もう無理です"）のときだけ真を返します。
// "/giveupnow" のような別語は誤判定せず input に落ちます。
func isCommand(trimmed, cmd string) bool {
	if trimmed == cmd {
		return true
	}
	rest, ok := strings.CutPrefix(trimmed, cmd)
	return ok && rest != "" && unicode.IsSpace([]rune(rest)[0])
}

// readLine は接続から JSONL 1行を読み、Message として返します。
// 行のフレーミングは jsonl に委譲し、ここでは Message の unmarshal だけを行います。
func readLine(r io.Reader) (ttxipc.Message, error) {
	line, err := jsonl.ReadLine(r)
	if err != nil {
		return ttxipc.Message{}, err
	}
	var msg ttxipc.Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return ttxipc.Message{}, fmt.Errorf("agentメッセージをJSONとして読めません: %w", err)
	}
	return msg, nil
}

func writeJSONLine(w io.Writer, msg ttxipc.Message) error {
	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return jsonl.WriteLine(w, encoded)
}
