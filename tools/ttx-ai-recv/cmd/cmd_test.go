package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/ttxipc"
)

// recv プロトコル本体（Dial→recv要求→応答受信）は ttxipc.Recv のテストで検証する。
// ここでは cmd 層の引数処理と終了コード写像だけを確認する。

// TestMainCommandByArgsReturnsIO は、接続・送受信に失敗する各入力が
// IPC 障害の終了コード（2）になることを確認する。
func TestMainCommandByArgsReturnsIO(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"不正なフラグ", []string{"--bogus"}},
		{"空 socket は net 層で失敗（既定では非空）", []string{"--socket", ""}},
		{"未起動 socket への接続失敗", []string{"--socket", "/tmp/ttx-nonexistent.sock", "--no-wait"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := cli.SpyProcInout("")
			code := MainCommandByArgs(tt.args, spy.NewProcInout())
			if code != exitIO {
				t.Fatalf("code=%d want %d (stderr=%q)", code, exitIO, spy.Stderr.String())
			}
		})
	}
}

// TestMainCommandByArgsTimeoutExit5 は、サーバが timeout 応答を返したとき
// 終了コード 5（待機タイムアウト＝abort 相当）に写像され、stdout には何も出さないことを確認する。
func TestMainCommandByArgsTimeoutExit5(t *testing.T) {
	sock := tempSocket(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = ttxipc.ReadLine(conn) // recv 要求を読む
		_ = ttxipc.WriteLine(conn, ttxipc.Message{Type: ttxipc.TypeTimeout})
	}()

	spy := cli.SpyProcInout("")
	code := MainCommandByArgs([]string{"--socket", sock, "--wait"}, spy.NewProcInout())
	if code != exitTimeout {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitTimeout, spy.Stderr.String())
	}
	if out := spy.Stdout.String(); out != "" {
		t.Fatalf("タイムアウト時は stdout へ出力しない: %q", out)
	}
}

func tempSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ttx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}
