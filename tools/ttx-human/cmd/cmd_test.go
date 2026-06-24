package cmd

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kuniwak/ttx/ttxipc"
)

// TestServeSocketAcceptLoop は ttx-human の serveSocket シム（frontend 生成・公開ログ・
// 接続先バナー）が ttxserver.Serve と結線し、stdin EOF まで end-to-end で動くことを確認する。
func TestServeSocketAcceptLoop(t *testing.T) {
	sock := tempSocket(t)
	logPath := filepath.Join(t.TempDir(), "public_log.jsonl")
	pr, pw := io.Pipe()
	var stdout safeBuffer
	cfg := Config{SocketPath: sock, PublicLog: logPath}

	done := make(chan error, 1)
	go func() { done <- serveSocket(cfg, pr, &stdout, io.Discard) }()

	// 0) 起動直後、GM の最初のメッセージより前に道具の案内バナーが表示される。
	waitFor(t, func() bool {
		return strings.Contains(stdout.String(), "本番の障害対応で使ってよいもの")
	}, "起動時バナーが表示されない")

	// 1) GM が通知を送ると人間画面に表示される（送信は別接続・別ターン）。
	c1 := dialRetry(t, sock)
	if err := ttxipc.WriteLine(c1, ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "テスト通知"}); err != nil {
		t.Fatal(err)
	}
	c1.Close()
	waitFor(t, func() bool { return strings.Contains(stdout.String(), "テスト通知") }, "通知が表示されない")

	// 起動時バナーは GM の最初のメッセージより前に出る（手順0で先行を確認済みだが、
	// 表示位置の前後関係でも明示的にロックする）。
	out := stdout.String()
	if strings.Index(out, "本番の障害対応で使ってよいもの") >= strings.Index(out, "テスト通知") {
		t.Fatalf("起動時バナーは GM メッセージより前に表示されるべき:\n%s", out)
	}

	// 2) 参加者入力を打鍵 → recv が同期取得する。
	if _, err := pw.Write([]byte("直近のリリースは?\n")); err != nil {
		t.Fatal(err)
	}
	c2 := dialRetry(t, sock)
	if err := ttxipc.WriteLine(c2, ttxipc.Message{Type: ttxipc.TypeRecv, Text: ttxipc.RecvWait}); err != nil {
		t.Fatal(err)
	}
	got, err := ttxipc.ReadLine(c2)
	c2.Close()
	if err != nil {
		t.Fatal(err)
	}
	want := ttxipc.Message{Type: ttxipc.TypeInput, Text: "直近のリリースは?"}
	if got != want {
		t.Fatalf("recv = %#v, want %#v", got, want)
	}

	// 3) stdin EOF で正常終了する（接続を何度開閉しても socket は生き続けた）。
	pw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveSocket returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stdin EOF 後に serveSocket が終了しない")
	}
}

// serveSocket は起動時に listen 中の socket パスを frontend へ表示する（崩れ防止のため
// stderr へ直書きせず、dumb では stdout へ出る）。接続失敗時に操作者がパスを読み取れるよう
// 必須の情報なので、表示されることを固定で担保する。
func TestServeSocketAnnouncesListeningBanner(t *testing.T) {
	sock := tempSocket(t)
	logPath := filepath.Join(t.TempDir(), "public_log.jsonl")
	pr, pw := io.Pipe()
	var stdout safeBuffer
	cfg := Config{SocketPath: sock, PublicLog: logPath}

	done := make(chan error, 1)
	go func() { done <- serveSocket(cfg, pr, &stdout, io.Discard) }()

	want := "listening on " + sock
	waitFor(t, func() bool { return strings.Contains(stdout.String(), want) },
		"起動バナーが stdout に表示されない")

	// stdin EOF で正常終了する。
	pw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveSocket returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stdin EOF 後に serveSocket が終了しない")
	}
}

// --- テスト補助 ---

func tempSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ttx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func dialRetry(t *testing.T, sock string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial timeout: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
