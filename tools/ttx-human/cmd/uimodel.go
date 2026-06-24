package cmd

import "strings"

// uiModel は対話UIの純粋な状態。Update/View は端末に一切触れないため、
// ターミナル無しでユニットテスト・ゴールデンテストできる（ttyFrontend が描画する）。
type uiModel struct {
	input      []rune // 編集中の入力バッファ
	cursor     int    // input 内のrune位置
	thinking   bool   // GM応答待ち（スピナー表示）
	spinnerIdx int    // スピナーのフレーム位置
	width      int    // 端末の桁数（hairline 長に使う）
}

// event はレンダループが受け取る入力イベント。キー入力・GMメッセージ・思考トグル・
// スピナー更新・リサイズをすべて単一チャネルに集約することで、編集中行への
// 非同期出力の再描画が安全に行え、モデルへのデータ競合が起きない。
type event interface{ isEvent() }

type keyKind int

const (
	keyRune keyKind = iota
	keyEnter
	keyBackspace
	keyLeft
	keyRight
	keyHome   // Ctrl+A
	keyEnd    // Ctrl+E
	keyEOF    // Ctrl+D（前方削除。空入力時のみ終端 EOF）
	keyDelete // Del キー（カーソル位置の文字を前方削除）
)

type keyEvent struct {
	kind keyKind
	r    rune // kind==keyRune のときの文字
}
type agentEvent struct{ text string } // GM メッセージ
type thinkingEvent struct{ on bool }  // 思考インジケータ ON/OFF
type spinnerTickEvent struct{}        // スピナー1コマ進める
type resizeEvent struct{ width int }  // 端末幅変更（SIGWINCH）

func (keyEvent) isEvent()         {}
func (agentEvent) isEvent()       {}
func (thinkingEvent) isEvent()    {}
func (spinnerTickEvent) isEvent() {}
func (resizeEvent) isEvent()      {}

// effect は Update が返す副作用指示。レンダループ（および renderScript）が解釈する。
type effect interface{ isEffect() }

// effectPrintAbove はメッセージをスクロールバックへ出力する。prefix は先頭行に付ける
// 箇条書き記号（GM/ヘルプ="● "、自分の送信エコー="> "）。継続行は renderBlock が揃える。
type effectPrintAbove struct {
	prefix string
	text   string
}
type effectEmitLine struct{ text string } // 確定した人間の入力行（ReadLine へ返す）
type effectEOF struct{}                   // 入力終端（Ctrl+D 空入力）
type effectBell struct{}                  // ベル

func (effectPrintAbove) isEffect() {}
func (effectEmitLine) isEffect()   {}
func (effectEOF) isEffect()        {}
func (effectBell) isEffect()       {}

const (
	bulletPrefix = "● " // GM/ヘルプの箇条書き記号
	echoPrefix   = "> " // 自分の送信行のエコー
	promptPrefix = "> " // 入力プロンプト
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Update は1イベントを適用し、新しいモデルと副作用を返す純粋関数。
func (m uiModel) Update(ev event) (uiModel, []effect) {
	switch e := ev.(type) {
	case keyEvent:
		return m.handleKey(e)
	case agentEvent:
		// いずれのGMメッセージ到着でも待機表示を解除する（IPCは応答相関を持たない）。
		m.thinking = false
		m.spinnerIdx = 0
		if e.text == "" {
			return m, nil
		}
		return m, []effect{effectPrintAbove{prefix: bulletPrefix, text: e.text}}
	case thinkingEvent:
		m.thinking = e.on
		if !e.on {
			m.spinnerIdx = 0
		}
		return m, nil
	case spinnerTickEvent:
		if m.thinking {
			m.spinnerIdx = (m.spinnerIdx + 1) % len(spinnerFrames)
		}
		return m, nil
	case resizeEvent:
		if e.width > 0 {
			m.width = e.width
		}
		return m, nil
	}
	return m, nil
}

func (m uiModel) handleKey(k keyEvent) (uiModel, []effect) {
	switch k.kind {
	case keyRune:
		// 文字を挿入する。コマンド（/help・別名 ? ・/giveup 等）の判定は送信後に
		// readHumanLoop が行うため、ここでは ?/？ も通常文字として扱う。
		m.input = insertRune(m.input, m.cursor, k.r)
		m.cursor++
		return m, nil
	case keyEnter:
		if len(m.input) == 0 {
			return m, nil // 空送信は無視
		}
		text := string(m.input)
		m.input = nil
		m.cursor = 0
		// 自分の入力をエコーしてから IPC へ送る（raw mode はローカルエコーが無いため）。
		return m, []effect{
			effectPrintAbove{prefix: echoPrefix, text: text},
			effectEmitLine{text: text},
		}
	case keyBackspace:
		return m.backwardDelete(), nil
	case keyLeft:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case keyRight:
		if m.cursor < len(m.input) {
			m.cursor++
		}
		return m, nil
	case keyHome:
		m.cursor = 0
		return m, nil
	case keyEnd:
		m.cursor = len(m.input)
		return m, nil
	case keyDelete: // Del: カーソル位置の文字を前方削除
		return m.forwardDelete(), nil
	case keyEOF: // Ctrl+D: 前方削除。ただし空入力時は終端
		if len(m.input) == 0 {
			return m, []effect{effectEOF{}}
		}
		return m.forwardDelete(), nil
	}
	return m, nil
}

// backwardDelete はカーソル左の文字を1つ削除し、カーソルを左へ動かす。
// カーソルが先頭で削除対象が無いときは何もしない。Backspace / Ctrl+H が使う。
func (m uiModel) backwardDelete() uiModel {
	if m.cursor > 0 {
		m.input = append(m.input[:m.cursor-1], m.input[m.cursor:]...)
		m.cursor--
	}
	return m
}

// forwardDelete はカーソル位置の文字（カーソルの右側）を1つ削除する。
// カーソルが末尾で削除対象が無いときは何もしない。Del / Ctrl+D が使う。
func (m uiModel) forwardDelete() uiModel {
	if m.cursor < len(m.input) {
		m.input = append(m.input[:m.cursor], m.input[m.cursor+1:]...)
	}
	return m
}

// View は入力リージョン（hairline＋プロンプト＋hairline＋ヒント/スピナー）を描画する。
// スクロールバック（● / > 行）は effectPrintAbove 側で出力されるため View には含めない。
// 各メッセージの後に空行を1つ入れる（printAbove の描画側）ので、最後のメッセージと
// 入力リージョンの間にも空行が入り、入力と出力が見分けやすくなる。
// リージョンは hairline＋プロンプト行群＋hairline＋ヒントの構成。プロンプトが端末幅を超えると
// wrapPrompt が複数行へ折り返すため、リージョン高は入力長で変わる（ttyFrontend が折り返し行数を
// 見てカーソル移動量を決める）。
func (m uiModel) View() string {
	hair := dim(strings.Repeat("─", m.hairlineWidth()))
	rows, _, _ := m.wrapPrompt()
	var hint string
	if m.thinking {
		hint = dim("考え中… " + spinnerFrames[m.spinnerIdx])
	} else {
		hint = dim("? でヘルプ")
	}
	lines := make([]string, 0, len(rows)+3)
	lines = append(lines, hair)
	lines = append(lines, rows...)
	lines = append(lines, hair, "  "+hint)
	return strings.Join(lines, "\n")
}

func (m uiModel) hairlineWidth() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

// wrapPrompt は promptPrefix+input を hairlineWidth に収まる行へ折り返し、入力カーソルが
// 乗る (行, 桁) も返す。端末のオートラップに頼らず自前で折り返すことで、View と ttyFrontend の
// カーソル計算を決定的にする（両者の唯一の真実源）。折り返し継続行はインデントせず桁0から
// 始める（readline/シェル同様で、カーソル計算も単純に保てる）。常に1行以上を返す。
func (m uiModel) wrapPrompt() (rows []string, cursorRow, cursorCol int) {
	max := m.hairlineWidth()
	prefix := []rune(promptPrefix)
	combined := append(append([]rune{}, prefix...), m.input...)
	cursorIdx := len(prefix) + m.cursor // 表示上のカーソル位置（combined 内の rune 位置）

	var b strings.Builder
	curW := 0
	cursorSet := false
	for i, r := range combined {
		w := runeWidth(r)
		// この文字を置くと現在行が max を超えるなら次行へ送る（現在行が空でない時のみ）。
		if curW+w > max && b.Len() > 0 {
			rows = append(rows, b.String())
			b.Reset()
			curW = 0
		}
		// カーソルは combined[cursorIdx] の直前にある。その文字の置かれる位置を記録する。
		if i == cursorIdx {
			cursorRow = len(rows)
			cursorCol = curW
			cursorSet = true
		}
		b.WriteRune(r)
		curW += w
	}
	rows = append(rows, b.String())
	if !cursorSet { // カーソルは入力末尾
		if curW >= max {
			// 末尾行がちょうど満幅。空行を足してその先頭にカーソルを置く
			// （pending-wrap による桁ずれや次回 redraw の崩れを避ける）。
			rows = append(rows, "")
			cursorRow = len(rows) - 1
			cursorCol = 0
		} else {
			cursorRow = len(rows) - 1
			cursorCol = curW
		}
	}
	return rows, cursorRow, cursorCol
}

// renderBlock は effectPrintAbove を表示文字列に整形する。先頭行に prefix を付け、
// 改行を含む複数行は継続行を2スペースでインデントして揃える（GM/ヘルプ共通）。
func renderBlock(prefix, text string) string {
	styledPrefix := prefix
	if strings.HasPrefix(prefix, "●") {
		styledPrefix = green("●") + prefix[len("●"):] // ●だけ着色し、後続の空白は残す
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	b.WriteString(styledPrefix + lines[0])
	for _, ln := range lines[1:] {
		b.WriteString("\n  " + ln)
	}
	return b.String()
}

func insertRune(s []rune, idx int, r rune) []rune {
	s = append(s, 0)
	copy(s[idx+1:], s[idx:])
	s[idx] = r
	return s
}

const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
	ansiGreen = "\x1b[32m"
)

func dim(s string) string   { return ansiDim + s + ansiReset }
func green(s string) string { return ansiGreen + s + ansiReset }

// displayWidth は端末表示幅を返す（全角=2, 半角=1）。ttyFrontend がカーソル位置を
// 計算するのに使う。日本語入力で ←/→ がずれないために必要。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// runeWidth は1文字の端末表示幅を返す（全角=2, 半角=1）。
func runeWidth(r rune) int {
	if isWide(r) {
		return 2
	}
	return 1
}

// isWide は East Asian Wide / Fullwidth の概略判定（日本語＋絵文字を主対象）。
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E,   // CJK部首補助〜CJK記号
		r >= 0x3041 && r <= 0x33FF,   // ひらがな・カタカナ・CJK記号など
		r >= 0x3400 && r <= 0x4DBF,   // CJK拡張A
		r >= 0x4E00 && r <= 0x9FFF,   // CJK統合漢字
		r >= 0xA000 && r <= 0xA4CF,   // 彝文字
		r >= 0xAC00 && r <= 0xD7A3,   // ハングル音節
		r >= 0xF900 && r <= 0xFAFF,   // CJK互換漢字
		r >= 0xFE10 && r <= 0xFE19,   // 縦書き形
		r >= 0xFE30 && r <= 0xFE6F,   // CJK互換形
		r >= 0xFF00 && r <= 0xFF60,   // 全角形（？＝FF1F を含む）
		r >= 0xFFE0 && r <= 0xFFE6,   // 全角記号
		r >= 0x1F300 && r <= 0x1FAFF, // 絵文字
		r >= 0x20000 && r <= 0x3FFFD: // CJK拡張B以降
		return true
	}
	return false
}
