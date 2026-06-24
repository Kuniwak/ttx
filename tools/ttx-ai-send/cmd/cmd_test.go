package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/ttxipc"
)

// tempSocket は短いパスの Unix Domain Socket を用意します。
// macOS の sun_path 長制限(約104B)を避けるため /tmp 直下に作ります。
func tempSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ttx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func TestRunRejectsInvalidType(t *testing.T) {
	// input は参加者起点の type なので送信できない。dial 前に exit 3 で弾く。
	spy := cli.SpyProcInout("")
	code := MainCommandByArgs([]string{"--socket", "/tmp/does-not-matter.sock", "--type", "input", "--text", "x"}, spy.NewProcInout())
	if code != exitUsage {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitUsage, spy.Stderr.String())
	}
}

func TestRunRejectsEmptyText(t *testing.T) {
	spy := cli.SpyProcInout("")
	code := MainCommandByArgs([]string{"--socket", "/tmp/x.sock", "--type", "answer", "--text", "   "}, spy.NewProcInout())
	if code != exitUsage {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitUsage, spy.Stderr.String())
	}
}

func TestRunRejectsBothTextAndTextFile(t *testing.T) {
	spy := cli.SpyProcInout("")
	code := MainCommandByArgs([]string{"--socket", "/tmp/x.sock", "--type", "answer", "--text", "a", "--text-file", "b.txt"}, spy.NewProcInout())
	if code != exitUsage {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitUsage, spy.Stderr.String())
	}
}

func TestRunRejectsUnknownFlagIsIO(t *testing.T) {
	// 不正な CLI フラグは IPC 障害と同じ終了コード（2）で弾く（移植元と同一契約）。
	spy := cli.SpyProcInout("")
	code := MainCommandByArgs([]string{"--bogus"}, spy.NewProcInout())
	if code != exitIO {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitIO, spy.Stderr.String())
	}
}

func TestRunSendsMessage(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string // --socket は runSendCapture が補う
		want  ttxipc.Message
	}{
		{
			// 本文を --text/--text-file で与えなければ stdin から読む。
			name:  "本文を stdin から送る",
			stdin: "10:02 エラー率がさらに上昇しています。",
			args:  []string{"--type", "notification"},
			want:  ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "10:02 エラー率がさらに上昇しています。"},
		},
		{
			// --thinking を付けると Message.Thinking=true で送られる。
			name:  "--thinking で Thinking=true を送る",
			stdin: "",
			args:  []string{"--type", "notification", "--thinking", "--text", "演習を開始します。"},
			want:  ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "演習を開始します。", Thinking: true},
		},
		{
			// --text のリテラル \n（バックスラッシュ+n）は実改行へ展開して送る。
			// args は シェルを介さず直接渡るので、シングルクォート引数の「生の \n」を再現できる。
			name:  "--text のリテラル \\n を改行へ展開して送る",
			stdin: "",
			args:  []string{"--type", "question", "--text", `1. おすすめ\n2. 部署から`},
			want:  ttxipc.Message{Type: string(ttxipc.TypeQuestion), Text: "1. おすすめ\n2. 部署から"},
		},
		{
			// stdin 経路は byte-faithful。リテラル \n は展開せずそのまま保持する。
			name:  "stdin のリテラル \\n は展開せず保持する",
			stdin: `1. おすすめ\n2. 部署から`,
			args:  []string{"--type", "question"},
			want:  ttxipc.Message{Type: string(ttxipc.TypeQuestion), Text: `1. おすすめ\n2. 部署から`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runSendCapture(t, tc.stdin, tc.args...)
			if got != tc.want {
				t.Fatalf("server received %#v, want %#v", got, tc.want)
			}
		})
	}
}

// runSendCapture は一時 socket に listener を立て、ttx-ai-send を stdin と args で実行し、
// サーバが受け取った1メッセージを返します。args に --socket は含めません（本ヘルパが補う）。
func runSendCapture(t *testing.T, stdin string, args ...string) ttxipc.Message {
	t.Helper()
	sock := tempSocket(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	got := make(chan ttxipc.Message, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		msg, err := ttxipc.ReadLine(conn)
		if err != nil {
			return
		}
		got <- msg
	}()

	spy := cli.SpyProcInout(stdin)
	code := MainCommandByArgs(append([]string{"--socket", sock}, args...), spy.NewProcInout())
	if code != exitOK {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitOK, spy.Stderr.String())
	}
	select {
	case m := <-got:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
		return ttxipc.Message{}
	}
}
