package ttxserver

import "github.com/Kuniwak/ttx/ttxipc"

// Frontend は参加者向けの入出力を抽象化する。IPC/ログ/分類のコア（HumanMessage,
// writeJSONLine, public_log）はこの抽象の外側で不変のまま動く。端末 UI・Slack など
// 表示の実体はこのインターフェースの実装側に閉じ込める。
type Frontend interface {
	// ReadLine は参加者が1行確定するまでブロックする。入力終端で io.EOF を返す。
	ReadLine() (string, error)
	// DisplayAgent は GM メッセージを表示する。agent goroutine から並行に呼ばれる。
	DisplayAgent(msg ttxipc.Message)
	// DisplayLocal は会話プロセスがローカルに返すメッセージ（/help の出力など。GM由来でない）を表示する。
	DisplayLocal(text string)
	// SetThinking は応答待ちインジケータを切り替える。
	SetThinking(on bool)
	// Finish は演習終了を受けて入力受付を終端する。保留中の表示（直前に渡した
	// postmortem や別れの文言）を出し切ってから ReadLine が io.EOF を返すようにする
	// （= 入力ポンプが抜けて Serve が正常終了する）。Close と異なり「描画を待ってから
	// 終端する」graceful な終了で、多重呼び出し安全。
	Finish()
	// Close は表示資源（端末状態など）を解放する。多重呼び出し安全。
	Close() error
}
