//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// setProcessGroup は子プロセスを独立したプロセスグループのリーダーにします（Setpgid）。
// これにより、子（codex）がさらに起動する孫プロセスごと、グループ宛シグナルで確実に終了できます。
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminateGroup はプロセスグループ全体へ sig を送ります。子はグループリーダーなので
// プロセスグループID == 子の PID。既に終了していれば ESRCH 等を返しますが無害です。
func terminateGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, sig)
}

// groupExists は pid をリーダーとするプロセスグループが存在するかを返します（signal 0）。
// クラッシュ復旧時、既に消えた／未起動の記録に対する無駄なシグナル送信を避けるために使います。
func groupExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(-pid, 0) == nil
}

// killGroupByPID は *exec.Cmd ハンドルを持たない（前回起動の孤児）プロセスグループへ
// PID 指定でシグナルを送ります。
func killGroupByPID(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, sig)
}
