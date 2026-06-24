//go:build !windows

package cmd

import (
	"os/exec"
	"testing"
	"time"
)

// TestReapOrphanGroupKillsRealProcess は reapOrphanGroup が PID 指定でプロセスグループを
// 停止できることを実プロセスで確認する。クラッシュ復旧で孤児 Codex を確実に止める中核挙動。
func TestReapOrphanGroupKillsRealProcess(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("テスト用プロセスを起動できない: %v", err)
	}
	pid := cmd.Process.Pid

	reapOrphanGroup(pid)

	waited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waited) }()
	select {
	case <-waited:
		// 孤児が停止して Wait が返った。
	case <-time.After(3 * time.Second):
		t.Fatal("reapOrphanGroup 後にプロセスが停止しない")
	}
}
