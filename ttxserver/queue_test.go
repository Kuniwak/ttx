package ttxserver

import (
	"testing"
	"time"

	"github.com/Kuniwak/ttx/ttxipc"
)

func TestInputQueueFIFOAndUnpop(t *testing.T) {
	q := newInputQueue()
	q.Push(ttxipc.Message{Type: "input", Text: "1"})
	q.Push(ttxipc.Message{Type: "input", Text: "2"})

	first, _ := q.PopNow()
	if first.Text != "1" {
		t.Fatalf("FIFO 違反: %#v", first)
	}
	q.Unpop(first) // 配信失敗の想定: 先頭へ戻す
	again, _ := q.PopNow()
	if again.Text != "1" {
		t.Fatalf("Unpop は先頭へ戻すべき: %#v", again)
	}
	second, _ := q.PopNow()
	if second.Text != "2" {
		t.Fatalf("2件目が崩れた: %#v", second)
	}
	if _, ok := q.PopNow(); ok {
		t.Fatal("空のはず")
	}
}

func TestInputQueuePopWaitBlocksThenReturns(t *testing.T) {
	q := newInputQueue()
	go func() {
		time.Sleep(20 * time.Millisecond)
		q.Push(ttxipc.Message{Type: "input", Text: "あとから来た"})
	}()
	got, ok := q.PopWait()
	if !ok || got.Text != "あとから来た" {
		t.Fatalf("PopWait = %#v ok=%v", got, ok)
	}
}

func TestInputQueueCloseUnblocksPopWait(t *testing.T) {
	q := newInputQueue()
	go func() {
		time.Sleep(20 * time.Millisecond)
		q.Close()
	}()
	if _, ok := q.PopWait(); ok {
		t.Fatal("Close 後の空キューは ok=false を返すべき")
	}
}

func TestPopWaitTimeoutFiresWhenIdle(t *testing.T) {
	q := newInputQueue()
	start := time.Now()
	msg, outcome := q.PopWaitTimeout(40 * time.Millisecond)
	elapsed := time.Since(start)
	if outcome != popTimeout {
		t.Fatalf("無入力で締切を過ぎたら popTimeout を返すべき: outcome=%v msg=%#v", outcome, msg)
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("締切前に打ち切っている: elapsed=%v", elapsed)
	}
}

func TestPopWaitTimeoutReturnsInputBeforeDeadline(t *testing.T) {
	q := newInputQueue()
	go func() {
		time.Sleep(20 * time.Millisecond)
		q.Push(ttxipc.Message{Type: "input", Text: "締切前に来た"})
	}()
	got, outcome := q.PopWaitTimeout(time.Second)
	if outcome != popMessage || got.Text != "締切前に来た" {
		t.Fatalf("締切前の入力は popMessage で返すべき: outcome=%v msg=%#v", outcome, got)
	}
}

func TestPopWaitTimeoutTouchResetsDeadline(t *testing.T) {
	q := newInputQueue()
	// 締切窓（40ms）より長くアイドルさせてから Touch でアイドル基準をリセットする。
	time.Sleep(60 * time.Millisecond)
	q.Touch()

	start := time.Now()
	_, outcome := q.PopWaitTimeout(40 * time.Millisecond)
	elapsed := time.Since(start)
	if outcome != popTimeout {
		t.Fatalf("Touch 後も無入力なら最終的に popTimeout: outcome=%v", outcome)
	}
	if elapsed < 30*time.Millisecond {
		// リセットされていなければ deadline は既に過去で、即 popTimeout になる。
		t.Fatalf("Touch でアイドル基準がリセットされていない（即タイムアウトした）: elapsed=%v", elapsed)
	}
}
