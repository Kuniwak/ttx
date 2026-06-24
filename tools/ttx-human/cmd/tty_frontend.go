//go:build !windows

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Kuniwak/ttx/ttxipc"
	"github.com/Kuniwak/ttx/ttxserver"
	"golang.org/x/term"
)

// tryTTYFrontend は stdin/stdout が両方 *os.File かつ両方 TTY のときだけ
// 対話UIを構築する。判定に失敗したら (nil,false) を返し、呼び出し側が dumb に倒す。
func tryTTYFrontend(stdin io.Reader, stdout io.Writer) (ttxserver.Frontend, bool) {
	inFile, inOK := stdin.(*os.File)
	outFile, outOK := stdout.(*os.File)
	if !inOK || !outOK {
		return nil, false
	}
	if !term.IsTerminal(int(inFile.Fd())) || !term.IsTerminal(int(outFile.Fd())) {
		return nil, false
	}
	fe, err := newTTYFrontend(inFile, outFile)
	if err != nil {
		return nil, false
	}
	return fe, true
}

const spinnerInterval = 120 * time.Millisecond

// ttyFrontend は raw mode の対話UI。端末を専有する単一のレンダループ goroutine が
// events チャネル（キー入力・GMメッセージ・思考トグル・スピナー・リサイズ）を直列処理し、
// 編集中の入力行に対する非同期GM出力の再描画を安全に行う。
type ttyFrontend struct {
	in       *os.File
	out      *os.File
	oldState *term.State

	events chan event    // 全イベントの集約先
	submit chan string   // 確定した入力行 → ReadLine
	done   chan struct{} // 終了シグナル（Close で閉じる）
	sigCh  chan os.Signal

	// cursorRowsBelowTop は直前の positionCursor がカーソルを置いた、リージョン先頭からの
	// 行数（＝1+カーソル行）。次回 redraw が消去のため先頭へ戻る量に使う。プロンプトが
	// 折り返して複数行になると入力長に依存して変わるため、固定値ではなく記録する。
	cursorRowsBelowTop int

	closeOnce sync.Once
}

func newTTYFrontend(in, out *os.File) (*ttyFrontend, error) {
	oldState, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	fe := &ttyFrontend{
		in:       in,
		out:      out,
		oldState: oldState,
		events:   make(chan event, 64),
		submit:   make(chan string, 8),
		done:     make(chan struct{}),
		sigCh:    make(chan os.Signal, 1),
	}
	signal.Notify(fe.sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	go fe.signalLoop()
	go fe.keyReadLoop()
	go fe.renderLoop()
	return fe, nil
}

// ReadLine は確定行を待つ。Close（done クローズ）で io.EOF を返す。
func (fe *ttyFrontend) ReadLine() (string, error) {
	select {
	case line := <-fe.submit:
		return line, nil
	case <-fe.done:
		return "", io.EOF
	}
}

func (fe *ttyFrontend) DisplayAgent(msg ttxipc.Message) {
	if msg.Text == "" {
		return
	}
	fe.send(agentEvent{text: msg.Text})
}

// DisplayLocal は会話プロセスがローカルに返すメッセージ（/help の出力など）を表示する。
// GM メッセージと同じく ● の箇条書きでスクロールバックへ出す。
func (fe *ttyFrontend) DisplayLocal(text string) {
	if text == "" {
		return
	}
	fe.send(agentEvent{text: text})
}

func (fe *ttyFrontend) SetThinking(on bool) {
	fe.send(thinkingEvent{on: on})
}

// finishEvent は演習終了を伝える内部イベント。uiModel は関与せず（renderLoop が Update 前に
// 横取りする）、レンダループに「保留中の表示を描画してから終了せよ」と指示する。
type finishEvent struct{}

func (finishEvent) isEvent() {}

// Finish は finishEvent をイベントチャネルへ積む。直前に積まれた表示イベント（postmortem や
// 別れの文言）は先のループ反復で描画されるため、「描画 → 終了」の順序が決定的になる
// （ctx キャンセルや即時 Close では描画と破棄が競合して取りこぼす恐れがある）。多重呼び出し安全。
func (fe *ttyFrontend) Finish() {
	fe.send(finishEvent{})
}

func (fe *ttyFrontend) Close() error {
	fe.closeOnce.Do(func() {
		signal.Stop(fe.sigCh)
		close(fe.done)
		fe.restore()
	})
	return nil
}

// send はイベントを投入する。終了中（done クローズ後）はブロックせず false を返す。
func (fe *ttyFrontend) send(ev event) bool {
	select {
	case fe.events <- ev:
		return true
	case <-fe.done:
		return false
	}
}

func (fe *ttyFrontend) signalLoop() {
	for {
		select {
		case sig := <-fe.sigCh:
			if sig == syscall.SIGWINCH {
				fe.send(resizeEvent{width: fe.termWidth()})
				continue
			}
			// SIGINT/SIGTERM（外部 kill 等）: 端末を復元してから終了。
			fe.Close()
			os.Exit(1)
		case <-fe.done:
			return
		}
	}
}

// keyReadLoop は raw stdin の唯一の読み手。バイト列を論理キーへデコードして events へ流す。
func (fe *ttyFrontend) keyReadLoop() {
	br := bufio.NewReader(fe.in)
	for {
		r, _, err := br.ReadRune()
		if err != nil {
			fe.Close() // stdin 終端 → 会話終了
			return
		}
		var ev event
		switch r {
		case '\r', '\n':
			ev = keyEvent{kind: keyEnter}
		case 0x7f, 0x08: // DEL / Backspace(Ctrl+H)
			ev = keyEvent{kind: keyBackspace}
		case 0x01: // Ctrl+A
			ev = keyEvent{kind: keyHome}
		case 0x05: // Ctrl+E
			ev = keyEvent{kind: keyEnd}
		case 0x02: // Ctrl+B
			ev = keyEvent{kind: keyLeft}
		case 0x06: // Ctrl+F
			ev = keyEvent{kind: keyRight}
		case 0x04: // Ctrl+D
			ev = keyEvent{kind: keyEOF}
		case 0x03: // Ctrl+C → 端末復元して終了
			fe.Close()
			return
		case 0x1b: // ESC: 矢印キー等の CSI シーケンス
			k, ok := decodeEscape(br)
			if !ok {
				continue // 単独ESC等は無視
			}
			ev = keyEvent{kind: k}
		default:
			if r < 0x20 {
				continue // その他の制御文字は無視
			}
			ev = keyEvent{kind: keyRune, r: r}
		}
		if !fe.send(ev) {
			return
		}
	}
}

// decodeEscape は ESC の後続を読み、矢印/Home/End を判定する。
func decodeEscape(br *bufio.Reader) (keyKind, bool) {
	b, err := br.ReadByte()
	if err != nil {
		return 0, false
	}
	if b != '[' && b != 'O' {
		_ = br.UnreadByte()
		return 0, false
	}
	c, err := br.ReadByte()
	if err != nil {
		return 0, false
	}
	switch c {
	case 'C':
		return keyRight, true
	case 'D':
		return keyLeft, true
	case 'H':
		return keyHome, true
	case 'F':
		return keyEnd, true
	case 'A', 'B':
		return 0, false // 上下（履歴）は未対応
	default:
		// 数値付き CSI（"3~"=Delete 等）。数値パラメータを読み取り末尾(~)まで進める。
		var num []byte
		for c >= '0' && c <= '9' {
			num = append(num, c)
			if c, err = br.ReadByte(); err != nil {
				return 0, false
			}
		}
		// 修飾子付き（"3;5~" 等）の残りを読み飛ばす
		for c == ';' || (c >= '0' && c <= '9') {
			if c, err = br.ReadByte(); err != nil {
				return 0, false
			}
		}
		if c == '~' && string(num) == "3" {
			return keyDelete, true // Delete キー
		}
		return 0, false
	}
}

func (fe *ttyFrontend) renderLoop() {
	defer func() {
		if r := recover(); r != nil {
			fe.restore()
			panic(r)
		}
	}()

	m := uiModel{width: fe.termWidth()}
	fe.firstDraw(m)

	var ticker *time.Ticker
	var tickCh <-chan time.Time
	stopTicker := func() {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
			tickCh = nil
		}
	}
	defer stopTicker()

	for {
		select {
		case <-fe.done:
			return
		case ev := <-fe.events:
			if _, ok := ev.(finishEvent); ok {
				// 演習終了。直前までの表示イベント（postmortem・別れの文言）は先の
				// ループ反復で描画済み。端末を復元して終了する（ReadLine が io.EOF を返す）。
				fe.Close()
				return
			}
			var effs []effect
			m, effs = m.Update(ev)
			var aboves []string
			var emits []string
			eof := false
			for _, e := range effs {
				switch ef := e.(type) {
				case effectPrintAbove:
					aboves = append(aboves, renderBlock(ef.prefix, ef.text))
				case effectEmitLine:
					emits = append(emits, ef.text)
				case effectEOF:
					eof = true
				case effectBell:
					fe.out.Write([]byte("\a"))
				}
			}
			if m.thinking && ticker == nil {
				ticker = time.NewTicker(spinnerInterval)
				tickCh = ticker.C
			} else if !m.thinking {
				stopTicker()
			}
			fe.redraw(aboves, m)
			for _, line := range emits {
				select {
				case fe.submit <- line:
				case <-fe.done:
					return
				}
			}
			if eof {
				fe.Close()
				return
			}
		case <-tickCh:
			m, _ = m.Update(spinnerTickEvent{})
			fe.redraw(nil, m)
		}
	}
}

// firstDraw は最初の入力リージョンを描画し、カーソルをプロンプト位置へ置く。
func (fe *ttyFrontend) firstDraw(m uiModel) {
	fe.write(m.View())
	fe.positionCursor(m)
}

// redraw は入力リージョンを再描画する。プロンプトは折り返すとリージョン高が変わるため、
// カーソルは直前 positionCursor が置いた位置（リージョン先頭から cursorRowsBelowTop 行下）に
// いる前提で、その行数ぶん上へ戻ってリージョン先頭(桁0)へ移り、画面末尾まで消去する。
// こうすることで折り返して押し上げられた上 hairline まで確実に消せる（重なりを防ぐ）。
// aboves があればリージョン消去後に各メッセージ＋空行を上へ出力し、永続スクロールバックにする。
func (fe *ttyFrontend) redraw(aboves []string, m uiModel) {
	var b strings.Builder
	if fe.cursorRowsBelowTop > 0 {
		b.WriteString(fmt.Sprintf("\x1b[%dA", fe.cursorRowsBelowTop)) // リージョン先頭の行へ
	}
	b.WriteString("\r\x1b[J") // 桁0→画面末尾まで消去
	for _, blk := range aboves {
		b.WriteString(blk)
		b.WriteString("\n\n") // メッセージの後に空行を1つ入れ、入力と出力を見分けやすくする
	}
	b.WriteString(m.View())
	fe.write(b.String())
	fe.positionCursor(m)
}

func (fe *ttyFrontend) positionCursor(m uiModel) {
	// View 出力直後カーソルはヒント行（リージョン最終行）末にいる。リージョン行構成は
	// hair(1)＋プロンプト(len(rows))＋hair(1)＋ヒント(1)。ヒント行からカーソルの乗る
	// プロンプト行へ上へ戻し、その行頭から桁を進める。折り返しに応じて戻り量が変わる。
	rows, cursorRow, cursorCol := m.wrapPrompt()
	up := len(rows) + 1 - cursorRow // = (len(rows)+2) - (1+cursorRow)
	out := ""
	if up > 0 {
		out += fmt.Sprintf("\x1b[%dA", up)
	}
	out += "\r"
	if cursorCol > 0 {
		out += fmt.Sprintf("\x1b[%dC", cursorCol)
	}
	fe.out.Write([]byte(out))
	fe.cursorRowsBelowTop = 1 + cursorRow // 次回 redraw の消去用に記録
}

// write は raw mode（OPOST 無効）向けに改行 \n を \r\n へ変換して書き出す。
func (fe *ttyFrontend) write(s string) {
	fe.out.Write([]byte(strings.ReplaceAll(s, "\n", "\r\n")))
}

func (fe *ttyFrontend) termWidth() int {
	w, _, err := term.GetSize(int(fe.out.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func (fe *ttyFrontend) restore() {
	// 端末モードを戻し、リージョン下の新しい行へ抜ける。
	term.Restore(int(fe.in.Fd()), fe.oldState)
	fe.out.Write([]byte("\r\n"))
}
