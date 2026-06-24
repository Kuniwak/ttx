//go:build windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup は Windows ではプロセスグループ操作が Unix と異なるため何もしません。
// ttx-slack ゲートウェイは Unix サーバでの運用を前提とし、Windows はビルド互換のための stub です。
func setProcessGroup(cmd *exec.Cmd) {}

// terminateGroup は Windows では子へのグレースフル終了手段が異なるため、
// SIGKILL 相当のときのみプロセスを強制終了します（best-effort）。
func terminateGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	if sig == syscall.SIGKILL {
		return cmd.Process.Kill()
	}
	return nil
}

// groupExists は Windows では FindProcess が常に成功するため判定できません。
// 復旧時の刈り取りを試みるべく true を返します（best-effort）。
func groupExists(pid int) bool {
	return pid > 0
}

// killGroupByPID は Windows では PID 指定でプロセスを強制終了します（グループ単位ではない best-effort）。
func killGroupByPID(pid int, sig syscall.Signal) error {
	if pid <= 0 || sig != syscall.SIGKILL {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
