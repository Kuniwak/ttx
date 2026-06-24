// Package ttxipc は ttx-human（会話用プロセス）と GM 側ツール
// （ttx-ai-send / ttx-ai-recv）が共有する IPC プロトコルの定義と
// GM クライアント操作（Send / Recv）です。
//
// メッセージは Unix Domain Socket 上の JSONL（1行1メッセージ）で、
// type と text の2フィールドだけを持ちます。接続は1回の req/resp で完結します。
// 行のフレーミング自体は汎用の jsonl パッケージに委譲します。
package ttxipc

import "fmt"

// Message は IPC で交換する唯一のメッセージ型です。type と text のほかに、
// 表示後の応答待ち表示（思考中インジケータ）の状態を運ぶ thinking を持ちます。
type Message struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// Thinking は GM→人間メッセージを表示した後の応答待ち表示の状態です。
	// true なら表示後も思考中インジケータ（スピナー）を点けたままにします
	// （演習開始通知など、直後に長い準備が続く場合に使う）。既定 false＝表示で解除。
	// 次のフラグなし（thinking:false）GM メッセージの表示で解除されます。
	// omitempty により、従来どおり thinking を使わないメッセージには現れません。
	Thinking bool `json:"thinking,omitempty"`
}

// SendableType は GM→人間方向に送信できる（=人間に表示される）メッセージ種別です。
// ただの文字列ではなくドメインの列挙なので独立した型にします。
type SendableType string

// GM→人間（ttx-human が表示する）メッセージ種別。
const (
	TypeAnswer       SendableType = "answer"       // 質問への回答
	TypeNotification SendableType = "notification" // 能動通知・開示
	TypeQuestion     SendableType = "question"     // 設定選択など参加者へ回答を求める問い（回答は input として返る）
	TypePostmortem   SendableType = "postmortem"   // ポストモーテム
	TypeError        SendableType = "error"        // エラー表示
)

// 人間→GM（ttx-human が recv 応答として返す）メッセージ種別。
const (
	TypeInput   = "input"   // 参加者の通常入力（質問・行動）
	TypeControl = "control" // 演習制御（例: /giveup）
)

// TypeRecv は GM クライアントが「次の参加者入力をくれ」と要求する制御 req の種別です。
// 人間には表示されず、public_log にも記録しません。
// text=="wait" のときサーバは入力が来るまでブロックして返します。
const TypeRecv = "recv"

// RecvWait は recv 制御 req の text に入れる待機フラグです。
const RecvWait = "wait"

// TypeTimeout は recv 待機がアイドルタイムアウトに達したことをサーバが GM へ知らせる
// recv 応答専用の種別です（server→GM 方向のみ）。人間には表示されず、送信もできません
// （sendableTypes に含めない）。GM 側は ttx-ai-recv の終了コードで打ち切りを受け取ります。
const TypeTimeout = "timeout"

// sendableTypes は送信可能な SendableType の集合です。
var sendableTypes = map[SendableType]struct{}{
	TypeAnswer:       {},
	TypeNotification: {},
	TypeQuestion:     {},
	TypePostmortem:   {},
	TypeError:        {},
}

// ParseSendType は文字列を送信可能な SendableType に検証して変換します。
// input/control/recv など、参加者起点・制御用の type は送信できません。
func ParseSendType(s string) (SendableType, error) {
	t := SendableType(s)
	if _, ok := sendableTypes[t]; !ok {
		return "", fmt.Errorf("不正なtype: %q（送信できるのは answer|notification|question|postmortem|error）", s)
	}
	return t, nil
}
