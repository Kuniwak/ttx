//go:build !windows

package cmd

import (
	"os/exec"
	"testing"
	"time"
)

// TestExecCodexProcessKillTerminatesGroup は Kill がプロセスグループへシグナルを送り、
// 起動したプロセスを確実に終了させる（Wait が返る）ことを確認する。孤児 Codex を
// 残さないための中核挙動（billing 保護）。
func TestExecCodexProcessKillTerminatesGroup(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("テスト用プロセスを起動できない: %v", err)
	}
	p := newExecCodexProcess(cmd, 100*time.Millisecond)

	if err := p.Kill(); err != nil {
		t.Fatalf("Kill err = %v", err)
	}

	waited := make(chan struct{})
	go func() { _ = p.Wait(); close(waited) }()
	select {
	case <-waited:
		// Kill により sleep が終了し Wait が返った。
	case <-time.After(3 * time.Second):
		t.Fatal("Kill 後に Wait が返らない（プロセスが終了していない）")
	}
}

func TestSetProcessGroupSetsSetpgid(t *testing.T) {
	cmd := exec.Command("true")
	setProcessGroup(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("Setpgid が設定されていない: %+v", cmd.SysProcAttr)
	}
}
