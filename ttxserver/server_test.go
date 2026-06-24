package ttxserver

import (
	"bytes"
	"context"
	"encoding/json"
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

// --- 人間入力ポンプ ---

func TestPumpHelpIsLocalNotQueued(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()

	fe := newFakeFrontend()
	fe.feed("/help")
	fe.closeInput()
	if err := pumpHumanInput(fe, queue, &logBuf, &mu); err != nil && err != io.EOF {
		t.Fatal(err)
	}

	if _, ok := queue.PopNow(); ok {
		t.Fatal("/help はキューへ積んではいけません")
	}
	if !strings.Contains(fe.output(), "/giveup") {
		t.Fatalf("出力にヘルプ本文が出ていません: %q", fe.output())
	}
	var logged ttxipc.Message
	if err := json.Unmarshal(bytes.TrimSpace([]byte(logBuf.String())), &logged); err != nil {
		t.Fatal(err)
	}
	if logged != (ttxipc.Message{Type: "help", Text: "/help"}) {
		t.Fatalf("public_log の記録 = %#v", logged)
	}
}

func TestPumpForwardsInputToQueue(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()

	// 空行はスキップし、通常入力は type:"input" としてキューへ積む。
	fe := newFakeFrontend()
	fe.feed("", "復旧宣言します")
	fe.closeInput()
	if err := pumpHumanInput(fe, queue, &logBuf, &mu); err != nil && err != io.EOF {
		t.Fatal(err)
	}

	got, ok := queue.PopNow()
	if !ok {
		t.Fatal("入力がキューへ積まれていません")
	}
	if got != (ttxipc.Message{Type: "input", Text: "復旧宣言します"}) {
		t.Fatalf("queued = %#v", got)
	}
}

func TestPumpGiveupToQueue(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()

	fe := newFakeFrontend()
	fe.feed("/giveup")
	fe.closeInput()
	if err := pumpHumanInput(fe, queue, &logBuf, &mu); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	got, ok := queue.PopNow()
	if !ok {
		t.Fatal("giveup がキューへ積まれていません")
	}
	if got != (ttxipc.Message{Type: "control", Text: "/giveup"}) {
		t.Fatalf("queued = %#v", got)
	}
}

func TestPumpClaimToQueue(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()

	fe := newFakeFrontend()
	fe.feed("/claim 部長判断は不要です")
	fe.closeInput()
	if err := pumpHumanInput(fe, queue, &logBuf, &mu); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	got, ok := queue.PopNow()
	if !ok {
		t.Fatal("claim がキューへ積まれていません")
	}
	if got != (ttxipc.Message{Type: "control", Text: "/claim 部長判断は不要です"}) {
		t.Fatalf("queued = %#v", got)
	}
}

// TestPumpAbortDeliversAndFinishes は、/abort が (1) GM 向けに control としてキューへ積まれ、
// (2) ローカルの別れ文言が表示され、(3) Finish が呼ばれて入力ポンプが抜ける（io.EOF）ことを確認する。
func TestPumpAbortDeliversAndFinishes(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()

	fe := newFakeFrontend()
	fe.feed("/abort")
	// closeInput しない: Finish 経由で ReadLine が EOF を返し pump が抜けることを確認する。
	if err := pumpHumanInput(fe, queue, &logBuf, &mu); err != nil && err != io.EOF {
		t.Fatal(err)
	}

	got, ok := queue.PopNow()
	if !ok {
		t.Fatal("/abort が GM 向けキューへ積まれていない")
	}
	if got != (ttxipc.Message{Type: "control", Text: "/abort"}) {
		t.Fatalf("queued = %#v", got)
	}
	if !strings.Contains(fe.output(), "演習を中断しました") {
		t.Fatalf("中断の別れ文言が表示されていない: %q", fe.output())
	}
	if !fe.finishedCalled() {
		t.Fatal("/abort 受付後に Finish が呼ばれるべき")
	}
}

// --- handleConn 分岐 ---

func TestHandleConnDisplaysAgentMessage(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()
	fe := newFakeFrontend()

	client, server := net.Pipe()
	go handleConn(server, fe, queue, &logBuf, &mu, 0)

	if err := ttxipc.WriteLine(client, ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "公開メッセージ"}); err != nil {
		t.Fatal(err)
	}
	// 送信接続は応答を返さない。handleConn が表示・ログ後に閉じると EOF になる。
	if _, err := ttxipc.ReadLine(client); err != io.EOF {
		t.Fatalf("EOF を期待: %v", err)
	}
	client.Close()

	if got := strings.TrimSpace(fe.output()); got != "公開メッセージ" {
		t.Fatalf("出力 = %q", got)
	}
	var logged ttxipc.Message
	if err := json.Unmarshal(bytes.TrimSpace([]byte(logBuf.String())), &logged); err != nil {
		t.Fatal(err)
	}
	if logged != (ttxipc.Message{Type: "notification", Text: "公開メッセージ"}) {
		t.Fatalf("logged = %#v", logged)
	}
}

// TestHandleConnPostmortemFinishes は、ポストモーテム表示後に会話プロセスを自動終了させる
// ため Finish が呼ばれることを確認する（演習終了の検知）。
func TestHandleConnPostmortemFinishes(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()
	fe := newFakeFrontend()

	client, server := net.Pipe()
	go handleConn(server, fe, queue, &logBuf, &mu, 0)

	if err := ttxipc.WriteLine(client, ttxipc.Message{Type: string(ttxipc.TypePostmortem), Text: "# ポストモーテム"}); err != nil {
		t.Fatal(err)
	}
	// 送信接続は応答を返さない。handleConn が表示・Finish 後に閉じると EOF になる。
	if _, err := ttxipc.ReadLine(client); err != io.EOF {
		t.Fatalf("EOF を期待: %v", err)
	}
	client.Close()

	if got := strings.TrimSpace(fe.output()); got != "# ポストモーテム" {
		t.Fatalf("ポストモーテムが表示されていない: %q", got)
	}
	if !fe.finishedCalled() {
		t.Fatal("ポストモーテム表示後に Finish が呼ばれるべき")
	}
}

func TestHandleConnThinkingFollowsMessageFlag(t *testing.T) {
	cases := []struct {
		name     string
		thinking bool
	}{
		{"thinking:true は表示後も思考中表示を維持する", true},
		{"thinking 既定（false）は表示で思考中表示を解除する", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logBuf safeBuffer
			var mu sync.Mutex
			queue := newInputQueue()
			fe := newFakeFrontend()

			client, server := net.Pipe()
			go handleConn(server, fe, queue, &logBuf, &mu, 0)

			if err := ttxipc.WriteLine(client, ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "x", Thinking: tc.thinking}); err != nil {
				t.Fatal(err)
			}
			// 送信接続は応答を返さない。handleConn が表示・思考中設定・ログ後に閉じると EOF。
			if _, err := ttxipc.ReadLine(client); err != io.EOF {
				t.Fatalf("EOF を期待: %v", err)
			}
			client.Close()

			if got := fe.lastThinking(); got != tc.thinking {
				t.Fatalf("最終的な思考中表示 = %v, want %v", got, tc.thinking)
			}
			// 思考中設定は必ず表示の後（表示自体が思考中を解除するため、後に確定させる必要がある）。
			if order := fe.eventOrder(); len(order) != 2 || order[0] != "display" || order[1] != "thinking" {
				t.Fatalf("表示→思考中設定 の順を期待: %v", order)
			}
		})
	}
}

func TestHandleConnRecvReturnsQueuedInput(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()
	queue.Push(ttxipc.Message{Type: "input", Text: "直近のリリースはありますか？"})
	fe := newFakeFrontend()

	client, server := net.Pipe()
	go handleConn(server, fe, queue, &logBuf, &mu, 0)

	if err := ttxipc.WriteLine(client, ttxipc.Message{Type: ttxipc.TypeRecv, Text: ttxipc.RecvWait}); err != nil {
		t.Fatal(err)
	}
	got, err := ttxipc.ReadLine(client)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	want := ttxipc.Message{Type: ttxipc.TypeInput, Text: "直近のリリースはありますか？"}
	if got != want {
		t.Fatalf("recv = %#v, want %#v", got, want)
	}
}

func TestHandleConnRecvTimesOut(t *testing.T) {
	var logBuf safeBuffer
	var mu sync.Mutex
	queue := newInputQueue()
	fe := newFakeFrontend()

	client, server := net.Pipe()
	// 滞留無し・短い recvTimeout。入力が来ないのでアイドルタイムアウトする。
	go handleConn(server, fe, queue, &logBuf, &mu, 30*time.Millisecond)

	if err := ttxipc.WriteLine(client, ttxipc.Message{Type: ttxipc.TypeRecv, Text: ttxipc.RecvWait}); err != nil {
		t.Fatal(err)
	}
	got, err := ttxipc.ReadLine(client)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if got.Type != ttxipc.TypeTimeout {
		t.Fatalf("アイドル時の recv 応答は timeout であるべき: %#v", got)
	}
	// timeout 応答後、会話プロセスを自動終了させるため Finish が呼ばれる（handleConn が非同期に
	// 呼ぶため waitFor で待つ）。
	waitFor(t, fe.finishedCalled, "アイドルタイムアウト後に Finish が呼ばれるべき")
}

// --- accept ループ統合 ---

func TestServeAcceptLoop(t *testing.T) {
	sock := tempSocket(t)
	fe := newFakeFrontend()
	var logBuf safeBuffer

	done := make(chan error, 1)
	go func() {
		done <- Serve(context.Background(), sock, fe, Hooks{
			LogWriter:   &logBuf,
			OnListening: func(p string) { fe.DisplayLocal("listening on " + p) },
		}, 0)
	}()

	// 0) 起動直後、GM の最初のメッセージより前に道具の案内バナーが表示される。
	waitFor(t, func() bool {
		return strings.Contains(fe.output(), "本番の障害対応で使ってよいもの")
	}, "起動時バナーが表示されない")

	// OnListening フックで接続先バナーが表示される。
	waitFor(t, func() bool {
		return strings.Contains(fe.output(), "listening on "+sock)
	}, "接続先バナーが表示されない")

	// 1) GM が通知を送ると参加者画面に表示される（送信は別接続・別ターン）。
	c1 := dialRetry(t, sock)
	if err := ttxipc.WriteLine(c1, ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "テスト通知"}); err != nil {
		t.Fatal(err)
	}
	c1.Close()
	waitFor(t, func() bool { return strings.Contains(fe.output(), "テスト通知") }, "通知が表示されない")

	// 起動時バナーは GM の最初のメッセージより前に表示される。
	out := fe.output()
	if strings.Index(out, "本番の障害対応で使ってよいもの") >= strings.Index(out, "テスト通知") {
		t.Fatalf("起動時バナーは GM メッセージより前に表示されるべき:\n%s", out)
	}

	// 2) 参加者入力を投入 → recv が同期取得する。
	fe.feed("直近のリリースは?")
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

	// 3) ReadLine の EOF で正常終了する（接続を何度開閉しても socket は生き続けた）。
	fe.closeInput()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EOF 後に Serve が終了しない")
	}
}

// TestServeAutoExitsOnEndConditions は、stdin EOF が来なくても、演習終了の各条件
// （ポストモーテム表示／ /abort 入力／ recv アイドルタイムアウト）を契機に Finish 経由で
// Serve が正常終了することを確認する。trigger が終了条件を起こし、wantOutput が空でなければ
// その文言が参加者画面に出ていることも確認する（idle は別れ文言を出さない＝検証なし）。
func TestServeAutoExitsOnEndConditions(t *testing.T) {
	cases := []struct {
		name        string
		recvTimeout time.Duration
		trigger     func(t *testing.T, sock string, fe *fakeFrontend)
		wantOutput  string // 空なら出力検証をスキップ
	}{
		{
			name: "postmortem",
			trigger: func(t *testing.T, sock string, fe *fakeFrontend) {
				c := dialRetry(t, sock)
				if err := ttxipc.WriteLine(c, ttxipc.Message{Type: string(ttxipc.TypePostmortem), Text: "# ポストモーテム"}); err != nil {
					t.Fatal(err)
				}
				c.Close()
			},
			wantOutput: "ポストモーテム",
		},
		{
			name: "abort",
			trigger: func(t *testing.T, sock string, fe *fakeFrontend) {
				fe.feed("/abort") // 入力チャネルはバッファ付きなので listen 前でも取りこぼさない
			},
			wantOutput: "演習を中断しました",
		},
		{
			name:        "idle_timeout",
			recvTimeout: 30 * time.Millisecond,
			trigger: func(t *testing.T, sock string, fe *fakeFrontend) {
				// GM が recv --wait。入力が来ないのでアイドルタイムアウトし timeout 応答が返る。
				c := dialRetry(t, sock)
				if err := ttxipc.WriteLine(c, ttxipc.Message{Type: ttxipc.TypeRecv, Text: ttxipc.RecvWait}); err != nil {
					t.Fatal(err)
				}
				got, err := ttxipc.ReadLine(c)
				if err != nil {
					t.Fatal(err)
				}
				c.Close()
				if got.Type != ttxipc.TypeTimeout {
					t.Fatalf("recv 応答は timeout であるべき: %#v", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sock := tempSocket(t)
			fe := newFakeFrontend() // closeInput しない＝ stdin EOF は起こさない
			var logBuf safeBuffer

			done := make(chan error, 1)
			go func() {
				done <- Serve(context.Background(), sock, fe, Hooks{
					LogWriter:   &logBuf,
					OnListening: func(string) {},
				}, tc.recvTimeout)
			}()

			tc.trigger(t, sock, fe)

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Serve returned %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("演習終了条件の後に Serve が終了しない")
			}
			if tc.wantOutput != "" && !strings.Contains(fe.output(), tc.wantOutput) {
				t.Fatalf("期待する文言が表示されていない: want=%q got=%q", tc.wantOutput, fe.output())
			}
		})
	}
}

// TestServeContextCancelStops は、Frontend が EOF にならなくても ctx のキャンセルだけで
// Serve が停止する（accept ループを抜け、socket を後始末して戻る）ことを確認する。
// Slack ゲートウェイが演習終了を検知して cancel() する経路に相当する。
func TestServeContextCancelStops(t *testing.T) {
	sock := tempSocket(t)
	fe := newFakeFrontend() // closeInput しない＝ReadLine はブロックし続ける
	defer fe.closeInput()   // 後始末でポンプ goroutine も解除する

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, sock, fe, Hooks{LogWriter: io.Discard, OnListening: func(string) {}}, 0)
	}()

	// listen 開始まで待つ（接続できれば listen 済み）。
	c := dialRetry(t, sock)
	c.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ctx キャンセル後に Serve が終了しない")
	}

	// 後始末: socket ファイルは Serve の defer で削除される。
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket ファイルが残っている: stat err=%v", err)
	}
}

// --- テスト補助 ---

// fakeFrontend は Frontend を実装する試験用のフロントエンドです。
// 入力は inputs チャネルから供給し（閉じると io.EOF）、表示は out へ追記して検査します。
// thinking は直近の SetThinking 値、events は表示と思考中設定の呼び出し順を記録します。
type fakeFrontend struct {
	mu       sync.Mutex
	inputs   chan string
	out      bytes.Buffer
	finished bool
	thinking bool
	events   []string

	done      chan struct{}
	closeOnce sync.Once
}

func newFakeFrontend() *fakeFrontend {
	return &fakeFrontend{inputs: make(chan string, 64), done: make(chan struct{})}
}

func (f *fakeFrontend) feed(lines ...string) {
	for _, l := range lines {
		f.inputs <- l
	}
}

func (f *fakeFrontend) closeInput() { close(f.inputs) }

// ReadLine は実フロントエンド同様、Finish/Close（done クローズ）でも io.EOF を返す。
func (f *fakeFrontend) ReadLine() (string, error) {
	select {
	case l, ok := <-f.inputs:
		if !ok {
			return "", io.EOF
		}
		return l, nil
	case <-f.done:
		return "", io.EOF
	}
}

func (f *fakeFrontend) DisplayAgent(msg ttxipc.Message) {
	f.append(msg.Text)
	f.mu.Lock()
	f.events = append(f.events, "display")
	f.mu.Unlock()
}
func (f *fakeFrontend) DisplayLocal(text string) { f.append(text) }
func (f *fakeFrontend) SetThinking(on bool) {
	f.mu.Lock()
	f.thinking = on
	f.events = append(f.events, "thinking")
	f.mu.Unlock()
}

func (f *fakeFrontend) Finish() {
	f.mu.Lock()
	f.finished = true
	f.mu.Unlock()
	f.closeOnce.Do(func() { close(f.done) })
}

func (f *fakeFrontend) Close() error {
	f.closeOnce.Do(func() { close(f.done) })
	return nil
}

// finishedCalled は Finish が呼ばれたかを返す（演習終了時の自動終了の検証用）。
func (f *fakeFrontend) finishedCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.finished
}

func (f *fakeFrontend) append(s string) {
	f.mu.Lock()
	f.out.WriteString(s)
	f.out.WriteByte('\n')
	f.mu.Unlock()
}

func (f *fakeFrontend) lastThinking() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.thinking
}

func (f *fakeFrontend) eventOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *fakeFrontend) output() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
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
