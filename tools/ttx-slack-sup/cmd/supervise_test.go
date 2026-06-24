package cmd

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestSuperviseCrashLoopAlertsAndStops(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	runs := 0
	run := func(ctx context.Context) (error, string) {
		runs++
		return errors.New("exit status 1"), "panic: boom\n"
	}
	var alerted string
	alertCount := 0
	alert := func(msg string) error { alerted = msg; alertCount++; return nil }

	supervise(context.Background(), run, alert, clk.now, clk.advance, supervisePolicy{
		maxRestarts: 5, window: 10 * time.Minute, backoff: 2 * time.Second,
	})

	if runs != 5 {
		t.Fatalf("5回終了でクラッシュループ判定すべき: runs=%d", runs)
	}
	if alertCount != 1 {
		t.Fatalf("通報は1回のみ: alertCount=%d", alertCount)
	}
	for _, want := range []string{"クラッシュループ", "boom"} {
		if !strings.Contains(alerted, want) {
			t.Fatalf("通報内容に %q が無い:\n%s", want, alerted)
		}
	}
}

func TestSuperviseStopsOnContextCancelWithoutAlert(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	ctx, cancel := context.WithCancel(context.Background())
	run := func(ctx context.Context) (error, string) {
		<-ctx.Done() // 子は ctx キャンセルで終了する想定
		return ctx.Err(), ""
	}
	alerted := false
	alert := func(msg string) error { alerted = true; return nil }

	done := make(chan struct{})
	go func() {
		supervise(ctx, run, alert, clk.now, clk.advance, supervisePolicy{
			maxRestarts: 5, window: 10 * time.Minute, backoff: time.Second,
		})
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ctx キャンセル後に supervise が終了しない")
	}
	if alerted {
		t.Fatal("意図した停止（ctx キャンセル）では通報してはいけない")
	}
}

func TestSuperviseDoesNotTripWhenExitsSpreadBeyondWindow(t *testing.T) {
	// 窓 10m に対し backoff 6m なら、窓内に残るのは最大2件 → maxRestarts=3 に到達しない
	// （＝散発的な再起動はクラッシュループ扱いしない）。無限ループ回避のため4回目で ctx を止める。
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	run := func(c context.Context) (error, string) {
		runs++
		if runs >= 4 {
			cancel()
		}
		return errors.New("exit"), ""
	}
	alerted := false
	alert := func(msg string) error { alerted = true; return nil }

	supervise(ctx, run, alert, clk.now, clk.advance, supervisePolicy{
		maxRestarts: 3, window: 10 * time.Minute, backoff: 6 * time.Minute,
	})

	if alerted {
		t.Fatal("窓を超えて散発する終了はクラッシュループ扱いしてはいけない")
	}
}

func TestFilterAfter(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	ts := []time.Time{base, base.Add(5 * time.Minute), base.Add(9 * time.Minute)}
	got := filterAfter(ts, base.Add(4*time.Minute)) // 4分より後だけ残す
	if len(got) != 2 {
		t.Fatalf("filterAfter = %v, 2件残るべき", got)
	}
}
