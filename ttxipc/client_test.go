package ttxipc

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestWriteLineOnlyTypeAndText(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteLine(&buf, Message{Type: string(TypeAnswer), Text: "本日午前に v2.3.1 をデプロイしています。"}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	want := `{"type":"answer","text":"本日午前に v2.3.1 をデプロイしています。"}`
	if got != want {
		t.Fatalf("WriteLine = %s, want %s", got, want)
	}
}

func TestWriteReadLineRoundTrip(t *testing.T) {
	// 改行を含む長文 text が1行に収まり、往復で復元できることを確認する。
	original := Message{Type: string(TypePostmortem), Text: "1行目\n2行目\n3行目"}
	var buf bytes.Buffer
	if err := WriteLine(&buf, original); err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.TrimSpace(buf.String()), "\n") != 0 {
		t.Fatalf("JSONL は1行であるべき: %q", buf.String())
	}
	got, err := ReadLine(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got != original {
		t.Fatalf("round trip = %#v, want %#v", got, original)
	}
}

func TestSendDelivers(t *testing.T) {
	sock := tempSocket(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	got := make(chan Message, 1)
	errc := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errc <- err
			return
		}
		defer conn.Close()
		msg, err := ReadLine(conn)
		if err != nil {
			errc <- err
			return
		}
		got <- msg
	}()

	want := Message{Type: string(TypeAnswer), Text: "本日 v2.3.1 をデプロイ済みです。"}
	if err := Send(sock, want); err != nil {
		t.Fatal(err)
	}

	select {
	case m := <-got:
		if m != want {
			t.Fatalf("server received %#v, want %#v", m, want)
		}
	case err := <-errc:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: server did not receive the message")
	}
}

func TestRecvReturnsInput(t *testing.T) {
	sock := tempSocket(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	gotReq := make(chan Message, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		req, err := ReadLine(conn)
		if err != nil {
			return
		}
		gotReq <- req
		_ = WriteLine(conn, Message{Type: TypeInput, Text: "直近のリリースはありますか？"})
	}()

	msg, ok, err := Recv(sock, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false, want true")
	}
	want := Message{Type: TypeInput, Text: "直近のリリースはありますか？"}
	if msg != want {
		t.Fatalf("recv = %#v, want %#v", msg, want)
	}

	select {
	case req := <-gotReq:
		if req.Type != TypeRecv || req.Text != RecvWait {
			t.Fatalf("server received req %#v, want recv/wait", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading request")
	}
}

func TestRecvEmptyOnClose(t *testing.T) {
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
		// 要求を読んだら応答せずに閉じる = 滞留無し。
		_, _ = ReadLine(conn)
		conn.Close()
	}()

	_, ok, err := Recv(sock, false)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true, want false (empty close)")
	}
}

func TestRecvDialError(t *testing.T) {
	// 存在しない socket への接続は失敗する。
	_, _, err := Recv("/tmp/ttx-nonexistent.sock", false)
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}
