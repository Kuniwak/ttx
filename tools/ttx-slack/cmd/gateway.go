package cmd

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kuniwak/ttx/ttxserver"
)

// MentionEvent は正規化済みの @ttx メンションです（slack_client が組み立てる）。
// bot 自身の投稿やメンションを含まないメッセージは、ここへ到達する前に除外されます。
type MentionEvent struct {
	ChannelID string
	ThreadTS  string // スレッド内なら親スレッドの ts。チャンネル直下なら空。
	MessageTS string // このメッセージ自身の ts。
	UserText  string // 先頭の bot メンションを除いた本文。
}

// SlackClient は Slack への投稿を抽象化します（実体は slack_client.go に隔離）。
type SlackClient interface {
	// PostToThread は channelID の threadTS スレッドへ text を投稿します。
	PostToThread(channelID, threadTS, text string) error
}

// session は1スレッド＝1演習の状態です。
type session struct {
	key        string // = threadTS（スレッド根の ts）
	channelID  string
	socketPath string
	fe         *slackFrontend
	cancel     context.CancelFunc

	sawPostmortem atomic.Bool
	createdAt     time.Time

	mu         sync.Mutex
	codex      CodexProcess
	lastActive time.Time
}

func (s *session) setCodex(c CodexProcess) {
	s.mu.Lock()
	s.codex = c
	s.mu.Unlock()
}

func (s *session) getCodex() CodexProcess {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.codex
}

func (s *session) touch(now time.Time) {
	s.mu.Lock()
	s.lastActive = now
	s.mu.Unlock()
}

func (s *session) idle(now time.Time, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Sub(s.lastActive) > ttl
}

func (s *session) tooOld(now time.Time, max time.Duration) bool {
	return now.Sub(s.createdAt) > max
}

// gateway は単一プロセスで複数の演習スレッドを多重化します。
type gateway struct {
	client      SlackClient
	spawner     CodexSpawner
	socketDir   string
	stateDir    string
	maxSessions int
	idleTTL     time.Duration
	maxRuntime  time.Duration
	recvTimeout time.Duration
	clock       func() time.Time

	mu       sync.Mutex
	sessions map[string]*session
	sem      chan struct{}
}

func newGateway(client SlackClient, spawner CodexSpawner, socketDir, stateDir string, maxSessions int, idleTTL, maxRuntime, recvTimeout time.Duration, clock func() time.Time) *gateway {
	if client == nil || spawner == nil {
		panic("newGateway: client と spawner は必須です")
	}
	if maxSessions <= 0 {
		panic("newGateway: maxSessions は1以上である必要があります")
	}
	if clock == nil {
		panic("newGateway: clock は必須です（本番は time.Now を明示的に渡す。nil フォールバックは設けない）")
	}
	return &gateway{
		client:      client,
		spawner:     spawner,
		socketDir:   socketDir,
		stateDir:    stateDir,
		maxSessions: maxSessions,
		idleTTL:     idleTTL,
		maxRuntime:  maxRuntime,
		recvTimeout: recvTimeout,
		clock:       clock,
		sessions:    make(map[string]*session),
		sem:         make(chan struct{}, maxSessions),
	}
}

func (g *gateway) now() time.Time { return g.clock() }

// HandleMention は @ttx メンション1件を処理します（それ以外のメッセージは
// slack_client で除外済み）。スレッド外の新規メンションで演習を開始し、既存演習の
// スレッド内メンションは GM 入力として流します。未管理スレッドのメンションは無視します。
func (g *gateway) HandleMention(ev MentionEvent) {
	key := rootTS(ev)

	g.mu.Lock()
	s, exists := g.sessions[key]
	g.mu.Unlock()

	switch {
	case exists:
		if isAbortCommand(ev.UserText) {
			// abort は postmortem を生成しない。終了メッセージだけ返して終了する。
			g.finishSession(key, "演習を終了しました。")
			return
		}
		s.fe.Submit(ev.UserText)
		s.touch(g.now())
	case ev.ThreadTS == "":
		// チャンネル直下の新規メンション → 演習開始。
		g.startSession(ev, key)
	default:
		// 未管理スレッド（他人のスレッド・終了済み等）でのメンションは無視する。
	}
}

// startSession は新規演習を開始します。同時実行上限に達していれば断りを返します。
func (g *gateway) startSession(ev MentionEvent, key string) {
	// スロット確保（billing DoS 防止）。取れなければ断る。
	select {
	case g.sem <- struct{}{}:
	default:
		_ = g.client.PostToThread(ev.ChannelID, key,
			fmt.Sprintf("現在 %d 件の演習が実行中です。ほかの演習が終わってから @ttx で開始してください。", g.maxSessions))
		return
	}

	// 二重起動の排除（イベント再配信などで同じ key が並行しても1回だけ起動）。
	g.mu.Lock()
	if _, dup := g.sessions[key]; dup {
		g.mu.Unlock()
		<-g.sem
		return
	}
	poster := func(text string) error { return g.client.PostToThread(ev.ChannelID, key, text) }
	var s *session
	fe := newSlackFrontend(poster, func() {
		// postmortem 受信＝正常終了。postmortem 本文は DisplayAgent が既に投稿済み。
		s.sawPostmortem.Store(true)
		g.finishSession(key, "")
	})
	ctx, cancel := context.WithCancel(context.Background())
	now := g.now()
	s = &session{
		key:        key,
		channelID:  ev.ChannelID,
		socketPath: g.socketPathFor(key),
		fe:         fe,
		cancel:     cancel,
		createdAt:  now,
		lastActive: now,
	}
	g.sessions[key] = s
	g.mu.Unlock()

	// socket を先に立ててから codex を起動する（codex は TTX_SOCKET 経由で接続するため）。
	go func() {
		// Slack ではログは各スレッドが記録であり、接続先バナーも不要。
		_ = ttxserver.Serve(ctx, s.socketPath, fe, ttxserver.Hooks{
			LogWriter:   io.Discard,
			OnListening: func(string) {},
		}, g.recvTimeout)
	}()

	codex, err := g.spawner.Spawn(ctx, s.socketPath, parseScenarioArg(ev.UserText))
	if err != nil {
		_ = g.client.PostToThread(ev.ChannelID, key, "演習を開始できませんでした。しばらくしてからもう一度お試しください。")
		g.finishSession(key, "")
		return
	}
	s.setCodex(codex)
	// クラッシュ復旧用に状態を永続化（孤児 Codex の刈り取り・再起動通知に使う）。
	_ = writeRecord(g.stateDir, sessionRecord{
		Key:        key,
		ChannelID:  ev.ChannelID,
		SocketPath: s.socketPath,
		CodexPID:   codex.PID(),
		StartedAt:  g.now().Unix(),
	})
	// codex の終了（自発 exit / クラッシュ）も終了の権威の1つ。
	go func() {
		_ = codex.Wait()
		g.finishSession(key, codexExitFarewell(s))
	}()

	_ = g.client.PostToThread(ev.ChannelID, key,
		"障害対応机上演習を開始します。このスレッドで @ttx に質問や対応行動を伝えてください。中断する場合は @ttx /abort、お手上げは @ttx /giveup です。")
	// codex 起動〜最初の設定質問まで 1〜2 分の思考時間があり、その間スレッドに何も流れず
	// 参加者を不安にさせる。待つだけでよいことを明示して空白を埋める。
	_ = g.client.PostToThread(ev.ChannelID, key,
		"🛠️ 準備中です…（最初の状況提示まで1〜2分ほど。それまで操作は不要です）")
}

// finishSession は演習を終了し、後始末してスロットを解放します。
// 終了契機（postmortem 受信 / abort / codex 終了 / idle）が複数あっても、map からの
// 削除をロック下で行うことで、実体の後始末は1回だけ実行されます。
func (g *gateway) finishSession(key, farewell string) {
	g.mu.Lock()
	s, ok := g.sessions[key]
	if !ok {
		g.mu.Unlock()
		return
	}
	delete(g.sessions, key)
	g.mu.Unlock()

	if farewell != "" {
		_ = g.client.PostToThread(s.channelID, s.key, farewell)
	}
	_ = s.fe.Close() // 入力ポンプ（ReadLine）を解除
	s.cancel()       // ttxserver.Serve を畳む（accept ループ・recv 解除・socket 削除）
	if codex := s.getCodex(); codex != nil {
		_ = codex.Kill() // プロセスグループごと終了（孤児を残さない）
	}
	_ = removeRecord(g.stateDir, key) // 正常終了したので復旧対象から外す
	<-g.sem                           // スロット解放
}

// reapStale は放置（idleTTL 超過）または規定の最大実行時間（maxRuntime 超過）の
// セッションを終了します。放置や暴走によるスロット・課金の占有を防ぐ多重防御です。
// maxRuntime が 0 以下なら最大実行時間ガードは無効。
func (g *gateway) reapStale() {
	now := g.now()
	g.mu.Lock()
	idle := []string{}
	expired := []string{}
	for key, s := range g.sessions {
		switch {
		case g.maxRuntime > 0 && s.tooOld(now, g.maxRuntime):
			expired = append(expired, key)
		case s.idle(now, g.idleTTL):
			idle = append(idle, key)
		}
	}
	g.mu.Unlock()

	for _, key := range idle {
		g.finishSession(key, "長時間操作がないため演習を終了しました。")
	}
	for _, key := range expired {
		g.finishSession(key, "規定の最大時間に達したため演習を終了しました。")
	}
}

// reapInterval は放置・最大実行時間の点検間隔です。
const reapInterval = time.Minute

// reapLoop は ctx がキャンセルされるまで定期的に reapStale を呼びます。
func (g *gateway) reapLoop(ctx context.Context) {
	t := time.NewTicker(reapInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.reapStale()
		}
	}
}

// shutdown は全セッションを終了します（graceful stop / テストの後始末用）。
func (g *gateway) shutdown() {
	g.mu.Lock()
	keys := make([]string, 0, len(g.sessions))
	for key := range g.sessions {
		keys = append(keys, key)
	}
	g.mu.Unlock()
	for _, key := range keys {
		g.finishSession(key, "")
	}
}

// sessionCount は現在アクティブな演習数を返します。
func (g *gateway) sessionCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.sessions)
}

func (g *gateway) socketPathFor(key string) string {
	// key（スレッド ts、例 "1700000000.123456"）をそのままファイル名へ。UDS のパス長
	// 制限（~104）を避けるため socketDir は短いパスを使う想定。
	return filepath.Join(g.socketDir, "ttx-"+key+".sock")
}

// rootTS は session キー（スレッド根の ts）を返します。スレッド内ならその親、
// チャンネル直下なら自身の ts。
func rootTS(ev MentionEvent) string {
	if ev.ThreadTS != "" {
		return ev.ThreadTS
	}
	return ev.MessageTS
}

// isAbortCommand は本文が /abort（単体、または "/abort 理由" の形）かを最小判定します。
// 終了シグナルとしての検知のみで、コマンドの完全な分類は ttxserver 側が担います。
func isAbortCommand(text string) bool {
	t := strings.TrimSpace(text)
	return t == "/abort" || strings.HasPrefix(t, "/abort ")
}

// parseScenarioArg は本文から SCN- で始まるシナリオ ID を1つ取り出します。
// 無ければ空文字（自動選定）。
func parseScenarioArg(text string) string {
	for _, f := range strings.Fields(text) {
		if strings.HasPrefix(strings.ToUpper(f), "SCN-") {
			return strings.ToUpper(f)
		}
	}
	return ""
}

// codexExitFarewell は codex プロセス終了が「最初の終了契機」だったときに出す文言です。
// postmortem 済みなら DisplayAgent が本文を投稿済みなので空（重複投稿しない）。
func codexExitFarewell(s *session) string {
	if s.sawPostmortem.Load() {
		return ""
	}
	return "演習を終了しました。"
}
