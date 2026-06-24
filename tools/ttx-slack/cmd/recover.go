package cmd

import (
	"os"
	"syscall"
)

// Reconcile は起動時に1度呼び、前回起動で後始末されなかった演習（クラッシュ・OOM 等の
// ハード終了で残った状態ファイル）を回収します。進行中演習は再開せず、安全側に終了・通知します。
//
//   - 孤児 Codex をプロセスグループごと停止（課金停止＝billing 保護の要）
//   - stale な socket ファイルを削除
//   - 該当スレッドへ再起動を通知
//   - 状態ファイルを削除
//
// 注: PID 再利用の可能性はゼロではありませんが、プロセスグループの存在確認（signal 0）で
// 既に消えた記録への送信を避けつつ、短いスーパーバイザ再起動間隔と Codex の最大実行時間
// ガード（後続）で実害を抑えます。
func (g *gateway) Reconcile() {
	recs, err := listRecords(g.stateDir)
	if err != nil || len(recs) == 0 {
		return
	}
	for _, rec := range recs {
		reapOrphanGroup(rec.CodexPID)
		_ = os.Remove(rec.SocketPath)
		_ = g.client.PostToThread(rec.ChannelID, rec.Key,
			"ゲートウェイが再起動したため、この演習は終了しました。新しい演習はチャンネルで @ttx とメンションして開始してください。")
		_ = removeRecord(g.stateDir, rec.Key)
	}
}

// reapOrphanGroup は前回起動の孤児 Codex をプロセスグループごと停止します。
// グループが存在するときだけ SIGTERM→SIGKILL を送ります（孤児なのでグレースは設けません）。
func reapOrphanGroup(pid int) {
	if pid <= 0 || !groupExists(pid) {
		return
	}
	_ = killGroupByPID(pid, syscall.SIGTERM)
	_ = killGroupByPID(pid, syscall.SIGKILL)
}
