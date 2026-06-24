package cmd

import (
	"context"
	"fmt"
	"time"
)

// childRunner は監視対象を1回起動し、終了まで待って (終了理由, 直近の標準エラー) を返します。
type childRunner func(ctx context.Context) (error, string)

// alertFunc は監視停止時の通報を行います（Slack incoming webhook など）。
type alertFunc func(message string) error

// supervisePolicy はクラッシュループ判定と再起動の方針です。
type supervisePolicy struct {
	maxRestarts int           // window 内でこの回数終了が続いたらクラッシュループと判断
	window      time.Duration // 終了回数を数える時間窓
	backoff     time.Duration // 再起動前の待機
}

// supervise は子プロセスを起動し、終了するたびに再起動します。ただし window 内で
// maxRestarts 回の終了が続いたらクラッシュループとみなし、alert で通報して監視を
// 停止します（無限再起動で計算機資源を浪費しないため）。ctx キャンセル時は意図した
// 停止として通報せず正常終了します。
func supervise(ctx context.Context, run childRunner, alert alertFunc, now func() time.Time, sleep func(time.Duration), policy supervisePolicy) {
	var exits []time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		exitErr, stderrTail := run(ctx)
		if ctx.Err() != nil {
			return // ctx キャンセル＝意図した停止。通報しない。
		}

		t := now()
		exits = append(filterAfter(exits, t.Add(-policy.window)), t)
		if len(exits) >= policy.maxRestarts {
			_ = alert(fmt.Sprintf(
				"ttx-slack がクラッシュループに陥ったため監視を停止しました（%s 以内に %d 回終了）。"+
					"原因を確認のうえ手動で再起動してください。\n最後の終了: %v\n直近の標準エラー:\n%s",
				policy.window, len(exits), exitErr, stderrTail))
			return
		}
		sleep(policy.backoff)
	}
}

// filterAfter は cutoff より後の時刻だけを残します（時間窓の刈り込み）。入力スライスを再利用します。
func filterAfter(ts []time.Time, cutoff time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}
