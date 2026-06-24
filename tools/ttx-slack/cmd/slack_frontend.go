package cmd

import (
	"io"
	"sync"

	"github.com/Kuniwak/ttx/ttxipc"
	"github.com/Kuniwak/ttx/ttxserver"
)

// threadPoster は特定の Slack スレッドへ1件のメッセージを投稿する関数です。
// 実体（slack-go）は slack_client.go 側のクロージャに隔離し、ここはネットワークに依存しません。
// 投稿失敗時のログ・リトライは実装側の責務とします（Frontend は表示結果を伝播できないため）。
type threadPoster func(text string) error

// slackFrontend は ttxserver.Frontend を Slack スレッド向けに実装します。
// 1スレッド＝1演習に対応し、参加者のメンションを input へ、GM メッセージをスレッドへ流します。
type slackFrontend struct {
	poster   threadPoster
	onFinish func() // postmortem 受信＝演習正常終了をゲートウェイへ通知する

	input     chan string
	done      chan struct{}
	closeOnce sync.Once
}

var _ ttxserver.Frontend = (*slackFrontend)(nil)

// newSlackFrontend は poster と onFinish を必須依存として受け取ります（nil は wiring バグなので panic）。
func newSlackFrontend(poster threadPoster, onFinish func()) *slackFrontend {
	if poster == nil {
		panic("newSlackFrontend: poster は必須です")
	}
	if onFinish == nil {
		panic("newSlackFrontend: onFinish は必須です（不要なら no-op func(){} を渡す）")
	}
	return &slackFrontend{
		poster:   poster,
		onFinish: onFinish,
		input:    make(chan string, 16),
		done:     make(chan struct{}),
	}
}

// Submit はスレッド内メンションのテキストを入力として渡します。ゲートウェイが呼びます。
// Close 済みなら捨てる（ブロックしない）。
func (f *slackFrontend) Submit(text string) {
	select {
	case f.input <- text:
	case <-f.done:
	}
}

// ReadLine は次の参加者入力を待ちます。Close で io.EOF を返します（ttyFrontend と同契約）。
func (f *slackFrontend) ReadLine() (string, error) {
	select {
	case line := <-f.input:
		return line, nil
	case <-f.done:
		return "", io.EOF
	}
}

// DisplayAgent は GM メッセージを type 別に整形してスレッドへ投稿します。
// postmortem の受信は演習の正常終了シグナルなので onFinish を呼びます。
func (f *slackFrontend) DisplayAgent(msg ttxipc.Message) {
	if text := formatForSlack(msg); text != "" {
		_ = f.poster(text) // 投稿失敗は poster 実装側でログ・リトライする
	}
	if msg.Type == string(ttxipc.TypePostmortem) {
		f.onFinish()
	}
}

// DisplayLocal は会話プロセスのローカル応答（/help など）をスレッドへ投稿します。
func (f *slackFrontend) DisplayLocal(text string) {
	if text == "" {
		return
	}
	_ = f.poster(text)
}

// SetThinking は応答待ち表示の切り替えです。Slack には常駐インジケータが無いため、
// 思考開始時に「思考中です…」をスレッドへ投稿して受領を伝えます（解除時は次の GM
// メッセージが答えになるため何もしません）。
func (f *slackFrontend) SetThinking(on bool) {
	if on {
		_ = f.poster("思考中です…")
	}
}

// Finish は演習終了で入力待ちを解除します。Slack は表示が同期投稿で完結し、終了の後始末は
// ゲートウェイ（postmortem の onFinish・codex 終了の finishSession）が担うため、done を
// 閉じるだけでよい（Close と同じ）。
func (f *slackFrontend) Finish() {
	f.closeOnce.Do(func() { close(f.done) })
}

// Close は入力待ちを解除します（ReadLine が io.EOF を返す）。多重呼び出し安全。
func (f *slackFrontend) Close() error {
	f.closeOnce.Do(func() { close(f.done) })
	return nil
}

// formatForSlack は GM メッセージ種別ごとに Slack 表示用の装飾を施します。
// プロトコルは {type,text} のみなので、種別の switch だけで出し分けが完結します。
func formatForSlack(msg ttxipc.Message) string {
	switch msg.Type {
	case string(ttxipc.TypeNotification):
		return "📣 " + msg.Text
	case string(ttxipc.TypeQuestion):
		return "❓ " + msg.Text
	case string(ttxipc.TypePostmortem):
		return "📋 *ポストモーテム*\n\n" + msg.Text + "\n\n— 演習を終了しました —"
	case string(ttxipc.TypeError):
		return "⚠️ " + msg.Text
	default: // answer ほか
		return msg.Text
	}
}
