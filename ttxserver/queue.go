package ttxserver

import (
	"sync"
	"time"

	"github.com/Kuniwak/ttx/ttxipc"
)

// popOutcome は待機つき取り出し（PopWaitTimeout）の結果種別です。
// 「メッセージを取得」「キュー閉鎖」「アイドルタイムアウト」を呼び出し側が
// 区別できるよう、bool ではなく明示の列挙にします。
type popOutcome int

const (
	popMessage popOutcome = iota // 入力を1件取り出した
	popClosed                    // キューが閉鎖され、もう入力は来ない
	popTimeout                   // アイドルタイムアウト（規定時間内に入力が来なかった）
)

// inputQueue は未回収の参加者入力を貯める並行安全な FIFO です。
// 状態は会話プロセスのメモリに集約し、GM 側はファイルやカーソルを持ちません。
type inputQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []ttxipc.Message
	closed bool
	// lastActivity は会話の最後のアクティビティ時刻（参加者入力 Push と GM 送信 Touch）です。
	// PopWaitTimeout のアイドルタイムアウトはここを基準に締切を測るため、GM が recv を
	// 何度再実行しても基準は不変で、総アイドル時間で打ち切れます。
	lastActivity time.Time
}

func newInputQueue() *inputQueue {
	q := &inputQueue{lastActivity: time.Now()}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Touch はアイドル基準時刻を現在へ更新します（GM 送信などの会話アクティビティで呼ぶ）。
func (q *inputQueue) Touch() {
	q.mu.Lock()
	q.lastActivity = time.Now()
	q.mu.Unlock()
}

// Push は末尾に積みます。
func (q *inputQueue) Push(m ttxipc.Message) {
	q.mu.Lock()
	q.items = append(q.items, m)
	q.lastActivity = time.Now()
	q.cond.Signal()
	q.mu.Unlock()
}

// Unpop は先頭へ戻します（配信失敗時の取りこぼし防止）。
func (q *inputQueue) Unpop(m ttxipc.Message) {
	q.mu.Lock()
	q.items = append([]ttxipc.Message{m}, q.items...)
	q.cond.Signal()
	q.mu.Unlock()
}

// PopWait は入力が来るまで無期限にブロックします。キューが閉鎖され空なら ok=false を返します。
func (q *inputQueue) PopWait() (ttxipc.Message, bool) {
	msg, outcome := q.PopWaitTimeout(0)
	return msg, outcome == popMessage
}

// PopWaitTimeout は入力が来るまでブロックし、結果種別を返します。
// timeout<=0 のときは無期限（PopWait と同じ）。timeout>0 のときは lastActivity からの
// アイドル経過が timeout を超えると popTimeout を返します（締切は lastActivity 基準なので、
// 呼び出しを繰り返しても総アイドル時間で打ち切れます）。
func (q *inputQueue) PopWaitTimeout(timeout time.Duration) (ttxipc.Message, popOutcome) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		if timeout <= 0 {
			q.cond.Wait()
			continue
		}
		remaining := time.Until(q.lastActivity.Add(timeout))
		if remaining <= 0 {
			return ttxipc.Message{}, popTimeout
		}
		// sync.Cond は待機にタイムアウトを持てないため、締切で起床させるタイマを仕掛ける。
		// 起床後はループ先頭で締切を再判定する（Touch でアイドル基準が延びていれば待ち直す）。
		timer := time.AfterFunc(remaining, func() {
			q.mu.Lock()
			q.cond.Broadcast()
			q.mu.Unlock()
		})
		q.cond.Wait()
		timer.Stop()
	}
	if len(q.items) == 0 {
		return ttxipc.Message{}, popClosed
	}
	return q.popLocked(), popMessage
}

// PopNow は先頭があれば返し、無ければ即 ok=false を返します。
func (q *inputQueue) PopNow() (ttxipc.Message, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return ttxipc.Message{}, false
	}
	return q.popLocked(), true
}

func (q *inputQueue) popLocked() ttxipc.Message {
	m := q.items[0]
	q.items = q.items[1:]
	return m
}

// Close はキューを閉じ、PopWait のブロックを解除します。
func (q *inputQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}
