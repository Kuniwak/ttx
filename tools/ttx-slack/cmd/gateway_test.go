package cmd

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kuniwak/ttx/ttxipc"
)

// --- fakes ---

type postRecord struct{ channel, thread, text string }

type fakeSlackClient struct {
	mu    sync.Mutex
	posts []postRecord
}

func (c *fakeSlackClient) PostToThread(channel, thread, text string) error {
	c.mu.Lock()
	c.posts = append(c.posts, postRecord{channel, thread, text})
	c.mu.Unlock()
	return nil
}

func (c *fakeSlackClient) threadTexts(thread string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, p := range c.posts {
		if p.thread == thread {
			out = append(out, p.text)
		}
	}
	return out
}

func (c *fakeSlackClient) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.posts)
}

type fakeCodex struct {
	pid    int
	killed atomic.Bool
	done   chan struct{}
	once   sync.Once
}

func newFakeCodex() *fakeCodex { return &fakeCodex{done: make(chan struct{})} }

func (c *fakeCodex) Wait() error { <-c.done; return nil }
func (c *fakeCodex) Kill() error { c.killed.Store(true); c.finish(); return nil }
func (c *fakeCodex) PID() int    { return c.pid }
func (c *fakeCodex) exitOnOwn()  { c.finish() }
func (c *fakeCodex) finish()     { c.once.Do(func() { close(c.done) }) }

type fakeSpawner struct {
	mu      sync.Mutex
	count   int
	codexes []*fakeCodex
	err     error
}

func (s *fakeSpawner) Spawn(ctx context.Context, socketPath, scenarioArg string) (CodexProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.err != nil {
		return nil, s.err
	}
	c := newFakeCodex()
	s.codexes = append(s.codexes, c)
	return c, nil
}

func (s *fakeSpawner) spawnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func (s *fakeSpawner) lastCodex() *fakeCodex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.codexes) == 0 {
		return nil
	}
	return s.codexes[len(s.codexes)-1]
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// --- helpers ---

func newTestGateway(t *testing.T, client SlackClient, spawner CodexSpawner, max int, idleTTL time.Duration, clock func() time.Time) *gateway {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ttxg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	stateDir := t.TempDir()
	// 最大実行時間ガードは既定で実質無効（24h）。検証するテストは g.maxRuntime を直接設定する。
	// recv タイムアウトは既定で無効（0）。検証するテストは g.recvTimeout を直接設定する。
	g := newGateway(client, spawner, dir, stateDir, max, idleTTL, 24*time.Hour, 0, clock)
	t.Cleanup(g.shutdown)
	return g
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

func waitForSocket(t *testing.T, sock string) {
	t.Helper()
	waitFor(t, func() bool {
		c, err := net.Dial("unix", sock)
		if err != nil {
			return false
		}
		c.Close()
		return true
	}, "socket が立ち上がらない: "+sock)
}

func containsSubstr(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// --- tests ---

func TestNewMentionStartsSessionAndSpawns(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "演習したい"})

	if spawner.spawnCount() != 1 {
		t.Fatalf("spawn 回数 = %d, want 1", spawner.spawnCount())
	}
	if g.sessionCount() != 1 {
		t.Fatalf("session 数 = %d, want 1", g.sessionCount())
	}
	texts := client.threadTexts("100.1")
	if !containsSubstr(texts, "開始します") {
		t.Fatalf("開始メッセージが投稿されていない: %#v", texts)
	}
	// 開始バナー直後に準備中の案内も投稿される（codex の思考時間中の空白を埋める）。
	if !containsSubstr(texts, "準備中です") {
		t.Fatalf("準備中メッセージが投稿されていない: %#v", texts)
	}
}

func TestSemaphoreLimitsToMax(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 2, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "200.1", UserText: "start"})
	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "300.1", UserText: "start"}) // 3本目

	if spawner.spawnCount() != 2 {
		t.Fatalf("上限2なのに spawn 回数 = %d", spawner.spawnCount())
	}
	if g.sessionCount() != 2 {
		t.Fatalf("session 数 = %d, want 2", g.sessionCount())
	}
	if !containsSubstr(client.threadTexts("300.1"), "実行中") {
		t.Fatalf("上限超過の断りが投稿されていない: %#v", client.threadTexts("300.1"))
	}
}

func TestNonExerciseThreadMentionIgnored(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	// セッションの無いスレッド内メンション → 無視。
	g.HandleMention(MentionEvent{ChannelID: "C1", ThreadTS: "999.9", MessageTS: "999.10", UserText: "やあ"})

	if spawner.spawnCount() != 0 {
		t.Fatalf("無視すべきなのに spawn された: %d", spawner.spawnCount())
	}
	if client.total() != 0 {
		t.Fatalf("無視すべきなのに投稿された: %d", client.total())
	}
}

func TestThreadMentionRoutesToFrontend(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
	sock := g.socketPathFor("100.1")
	waitForSocket(t, sock)

	// スレッド内メンション → GM 入力として流れる。
	g.HandleMention(MentionEvent{ChannelID: "C1", ThreadTS: "100.1", MessageTS: "100.2", UserText: "直近のリリースは?"})

	// GM 役として recv すると input が取れる。
	got, ok, err := ttxipc.Recv(sock, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("recv に入力が来ない")
	}
	want := ttxipc.Message{Type: ttxipc.TypeInput, Text: "直近のリリースは?"}
	if got != want {
		t.Fatalf("recv = %#v, want %#v", got, want)
	}
}

func TestPostmortemFinishesSession(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
	sock := g.socketPathFor("100.1")
	waitForSocket(t, sock)
	codex := spawner.lastCodex()

	// GM 役として postmortem を送る → DisplayAgent が投稿し onFinish で終了。
	if err := ttxipc.Send(sock, ttxipc.Message{Type: string(ttxipc.TypePostmortem), Text: "# ポストモーテム本文"}); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return g.sessionCount() == 0 }, "postmortem 後にセッションが終了しない")
	waitFor(t, func() bool { return codex.killed.Load() }, "postmortem 後に codex が kill されない")
	if !containsSubstr(client.threadTexts("100.1"), "ポストモーテム") {
		t.Fatalf("postmortem が投稿されていない: %#v", client.threadTexts("100.1"))
	}
}

func TestAbortFinishesWithFarewell(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
	codex := spawner.lastCodex()

	g.HandleMention(MentionEvent{ChannelID: "C1", ThreadTS: "100.1", MessageTS: "100.2", UserText: "/abort"})

	if g.sessionCount() != 0 {
		t.Fatalf("/abort 後もセッションが残っている: %d", g.sessionCount())
	}
	waitFor(t, func() bool { return codex.killed.Load() }, "/abort で codex が kill されない")
	if !containsSubstr(client.threadTexts("100.1"), "演習を終了しました") {
		t.Fatalf("終了メッセージが投稿されていない: %#v", client.threadTexts("100.1"))
	}
}

func TestReapStaleClosesSession(t *testing.T) {
	tests := []struct {
		name       string
		idleTTL    time.Duration
		maxRuntime time.Duration // 0 なら既定（実質無効）のまま
		advance    time.Duration
		wantMsg    string
	}{
		{
			name:    "idle 超過で終了",
			idleTTL: 30 * time.Minute,
			advance: 31 * time.Minute,
			wantMsg: "長時間操作がない",
		},
		{
			// idle も超えるが最大実行時間を優先（暴走・終わらない codex の保険）。
			name:       "最大実行時間超過で終了",
			idleTTL:    30 * time.Minute,
			maxRuntime: 90 * time.Minute,
			advance:    91 * time.Minute,
			wantMsg:    "最大時間",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSlackClient{}
			spawner := &fakeSpawner{}
			clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
			g := newTestGateway(t, client, spawner, 3, tt.idleTTL, clk.now)
			if tt.maxRuntime > 0 {
				g.maxRuntime = tt.maxRuntime
			}

			g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
			codex := spawner.lastCodex()

			clk.advance(tt.advance)
			g.reapStale()

			if g.sessionCount() != 0 {
				t.Fatalf("回収後もセッションが残っている: %d", g.sessionCount())
			}
			waitFor(t, func() bool { return codex.killed.Load() }, "回収で codex が kill されない")
			if !containsSubstr(client.threadTexts("100.1"), tt.wantMsg) {
				t.Fatalf("終了メッセージ %q が投稿されていない: %#v", tt.wantMsg, client.threadTexts("100.1"))
			}
		})
	}
}

func TestCodexExitFinishesSession(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
	codex := spawner.lastCodex()

	// codex が自発終了（クラッシュ等）→ Wait が返り、終了の権威として finish する。
	codex.exitOnOwn()

	waitFor(t, func() bool { return g.sessionCount() == 0 }, "codex 終了後にセッションが終了しない")
	waitFor(t, func() bool { return containsSubstr(client.threadTexts("100.1"), "演習を終了しました") },
		"codex 終了の通知が投稿されない")
}

func TestStartSessionSpawnErrorReleasesSlot(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{err: errors.New("boom")}
	g := newTestGateway(t, client, spawner, 1, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})

	// spawn 失敗でセッションは残らず、スロットも解放されて次が開始できる。
	if g.sessionCount() != 0 {
		t.Fatalf("spawn 失敗後にセッションが残っている: %d", g.sessionCount())
	}
	if !containsSubstr(client.threadTexts("100.1"), "開始できませんでした") {
		t.Fatalf("失敗メッセージが投稿されていない: %#v", client.threadTexts("100.1"))
	}
	spawner.mu.Lock()
	spawner.err = nil
	spawner.mu.Unlock()
	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "200.1", UserText: "start"})
	if g.sessionCount() != 1 {
		t.Fatalf("スロットが解放されず次の演習を開始できない: %d", g.sessionCount())
	}
}

func TestParseScenarioArg(t *testing.T) {
	tests := []struct{ in, want string }{
		{"演習したい", ""},
		{"start SCN-007", "SCN-007"},
		{"scn-012 でお願い", "SCN-012"},
		{"SCN-003", "SCN-003"},
	}
	for _, tt := range tests {
		if got := parseScenarioArg(tt.in); got != tt.want {
			t.Fatalf("parseScenarioArg(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsAbortCommand(t *testing.T) {
	yes := []string{"/abort", "/abort ネタバレ回避", "  /abort  "}
	no := []string{"/abortnow", "abort", "/giveup", "リリースを確認"}
	for _, s := range yes {
		if !isAbortCommand(s) {
			t.Fatalf("isAbortCommand(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isAbortCommand(s) {
			t.Fatalf("isAbortCommand(%q) = true, want false", s)
		}
	}
}

func TestRootTS(t *testing.T) {
	if got := rootTS(MentionEvent{MessageTS: "100.1"}); got != "100.1" {
		t.Fatalf("チャンネル直下は MessageTS を根にすべき: %q", got)
	}
	if got := rootTS(MentionEvent{ThreadTS: "100.1", MessageTS: "100.2"}); got != "100.1" {
		t.Fatalf("スレッド内は ThreadTS を根にすべき: %q", got)
	}
}
