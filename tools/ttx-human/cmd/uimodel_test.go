package cmd

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// applyAll は複数イベントを順に Update し、最終モデルと全副作用を返すテストヘルパ。
func applyAll(m uiModel, evs ...event) (uiModel, []effect) {
	var all []effect
	for _, ev := range evs {
		var effs []effect
		m, effs = m.Update(ev)
		all = append(all, effs...)
	}
	return m, all
}

func typeRunes(s string) []event {
	evs := make([]event, 0, len(s))
	for _, r := range s {
		evs = append(evs, keyEvent{kind: keyRune, r: r})
	}
	return evs
}

func TestLineEditing(t *testing.T) {
	m := uiModel{width: 80}
	// "abc" を入力 → カーソル左へ2 → Backspace（'b'削除）
	m, _ = applyAll(m, typeRunes("abc")...)
	if got := string(m.input); got != "abc" || m.cursor != 3 {
		t.Fatalf("after typing: input=%q cursor=%d", got, m.cursor)
	}
	m, _ = applyAll(m, keyEvent{kind: keyLeft}, keyEvent{kind: keyLeft})
	if m.cursor != 1 {
		t.Fatalf("after 2x left: cursor=%d, want 1", m.cursor)
	}
	m, _ = m.Update(keyEvent{kind: keyBackspace}) // 'a' を削除（カーソル左の文字）
	if got := string(m.input); got != "bc" || m.cursor != 0 {
		t.Fatalf("after backspace: input=%q cursor=%d", got, m.cursor)
	}
	// Ctrl+H（keyBackspace と同義）はカーソル0なので何もしない
	m, _ = m.Update(keyEvent{kind: keyBackspace})
	if got := string(m.input); got != "bc" {
		t.Fatalf("backspace at col0 changed input: %q", got)
	}
	// Right で末尾へ → 文字挿入
	m, _ = applyAll(m, keyEvent{kind: keyRight}, keyEvent{kind: keyRight}, keyEvent{kind: keyRune, r: 'd'})
	if got := string(m.input); got != "bcd" {
		t.Fatalf("after right+insert: input=%q", got)
	}
	// Home / End
	m, _ = m.Update(keyEvent{kind: keyHome})
	if m.cursor != 0 {
		t.Fatalf("home: cursor=%d", m.cursor)
	}
	m, _ = m.Update(keyEvent{kind: keyEnd})
	if m.cursor != 3 {
		t.Fatalf("end: cursor=%d", m.cursor)
	}
}

func TestInsertAtCursor(t *testing.T) {
	m := uiModel{width: 80}
	m, _ = applyAll(m, typeRunes("ac")...)
	m, _ = m.Update(keyEvent{kind: keyLeft}) // カーソルは 'c' の前
	m, _ = m.Update(keyEvent{kind: keyRune, r: 'b'})
	if got := string(m.input); got != "abc" {
		t.Fatalf("insert at cursor: input=%q, want abc", got)
	}
}

func TestSubmitEmitsAndEchoesAndClears(t *testing.T) {
	m := uiModel{width: 80}
	m, _ = applyAll(m, typeRunes("直近のリリースはありますか？")...)
	m, effs := m.Update(keyEvent{kind: keyEnter})
	if len(m.input) != 0 || m.cursor != 0 {
		t.Fatalf("buffer not cleared: input=%q cursor=%d", string(m.input), m.cursor)
	}
	var echoed, emitted string
	for _, e := range effs {
		switch ef := e.(type) {
		case effectPrintAbove:
			echoed = ef.prefix + ef.text
		case effectEmitLine:
			emitted = ef.text
		}
	}
	if echoed != "> 直近のリリースはありますか？" {
		t.Fatalf("echo = %q", echoed)
	}
	if emitted != "直近のリリースはありますか？" {
		t.Fatalf("emit = %q", emitted)
	}
}

func TestEmptySubmitIgnored(t *testing.T) {
	m := uiModel{width: 80}
	_, effs := m.Update(keyEvent{kind: keyEnter})
	if len(effs) != 0 {
		t.Fatalf("empty submit produced effects: %#v", effs)
	}
}

func TestQuestionMarkIsNormalChar(t *testing.T) {
	// ?/？ はUIではコマンド扱いせず常に通常文字として挿入する。
	// /help（別名 ?）の判定は送信後に readHumanLoop が行う。
	for _, r := range []rune{'?', '？'} {
		m := uiModel{width: 80}
		m2, effs := m.Update(keyEvent{kind: keyRune, r: r})
		if len(effs) != 0 {
			t.Fatalf("空入力の %q で副作用が出てはいけない: %#v", string(r), effs)
		}
		if string(m2.input) != string(r) {
			t.Fatalf("%q が挿入されていない: %q", string(r), string(m2.input))
		}
	}
	// 非空入力でも通常挿入
	m := uiModel{width: 80}
	m, _ = m.Update(keyEvent{kind: keyRune, r: 'a'})
	m, effs := m.Update(keyEvent{kind: keyRune, r: '?'})
	if len(effs) != 0 || string(m.input) != "a?" {
		t.Fatalf("非空入力での ? 挿入失敗: input=%q effs=%#v", string(m.input), effs)
	}
}

func TestAsyncAgentMessagePreservesInput(t *testing.T) {
	m := uiModel{width: 80}
	m, _ = applyAll(m, typeRunes("エラー率を確認")...)
	before := string(m.input)
	curBefore := m.cursor
	m, effs := m.Update(agentEvent{text: "10:02 注文APIのエラー率がさらに上昇しています。"})
	if string(m.input) != before || m.cursor != curBefore {
		t.Fatalf("input changed by async msg: input=%q cursor=%d", string(m.input), m.cursor)
	}
	if len(effs) != 1 {
		t.Fatalf("agent msg effects=%#v", effs)
	}
	pa, ok := effs[0].(effectPrintAbove)
	if !ok || pa.prefix != bulletPrefix || pa.text != "10:02 注文APIのエラー率がさらに上昇しています。" {
		t.Fatalf("agent printAbove = %#v", effs[0])
	}
}

func TestThinkingAndSpinner(t *testing.T) {
	m := uiModel{width: 80}
	m, _ = m.Update(thinkingEvent{on: true})
	if !strings.Contains(m.View(), "考え中…") {
		t.Fatalf("view lacks thinking indicator: %q", m.View())
	}
	if !strings.Contains(m.View(), spinnerFrames[0]) {
		t.Fatalf("view lacks spinner frame 0")
	}
	m, _ = m.Update(spinnerTickEvent{})
	if m.spinnerIdx != 1 || !strings.Contains(m.View(), spinnerFrames[1]) {
		t.Fatalf("spinner did not advance: idx=%d", m.spinnerIdx)
	}
	// GMメッセージ到着で思考解除
	m, _ = m.Update(agentEvent{text: "本日午前に v2.3.1 をデプロイしています。"})
	if m.thinking {
		t.Fatalf("thinking not cleared by agent message")
	}
	if !strings.Contains(m.View(), "? でヘルプ") {
		t.Fatalf("hint not restored: %q", m.View())
	}
	// spinnerTick は思考中でなければ進めない
	m, _ = m.Update(spinnerTickEvent{})
	if m.spinnerIdx != 0 {
		t.Fatalf("spinner advanced while not thinking: idx=%d", m.spinnerIdx)
	}
}

// TestDeleteKeys は keyDelete（物理 Del）と keyEOF（Ctrl+D）の前方削除を検証する。
// 両者の違いは空入力時のみ: keyDelete は何もしない、keyEOF は EOF を送る（セッション終了）。
func TestDeleteKeys(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		cursor     int
		key        keyKind
		wantInput  string
		wantCursor int
		wantEOF    bool
	}{
		// 物理 Del: カーソル位置の文字を前方削除（カーソルは動かない）。空入力でも EOF にしない。
		{"del_mid", "abc", 1, keyDelete, "ac", 1, false},
		{"del_start", "abc", 0, keyDelete, "bc", 0, false},
		{"del_at_end_noop", "abc", 3, keyDelete, "abc", 3, false},
		{"del_empty_noop", "", 0, keyDelete, "", 0, false},
		// Ctrl+D: 非空なら前方削除、空入力なら EOF（終端）。
		{"eof_mid_deletes", "abc", 0, keyEOF, "bc", 0, false},
		{"eof_at_end_noop", "abc", 3, keyEOF, "abc", 3, false},
		{"eof_empty_emits", "", 0, keyEOF, "", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, effs := modelWith(80, c.input, c.cursor).Update(keyEvent{kind: c.key})
			if got := string(m.input); got != c.wantInput || m.cursor != c.wantCursor {
				t.Fatalf("input=%q cursor=%d, want %q/%d", got, m.cursor, c.wantInput, c.wantCursor)
			}
			gotEOF := false
			for _, e := range effs {
				if _, ok := e.(effectEOF); ok {
					gotEOF = true
				}
			}
			if gotEOF != c.wantEOF {
				t.Fatalf("EOF=%v, want %v (effs=%#v)", gotEOF, c.wantEOF, effs)
			}
			// 前方削除も EOF も、それ以外の想定外の副作用は出さない
			if !c.wantEOF && len(effs) != 0 {
				t.Fatalf("unexpected effects: %#v", effs)
			}
		})
	}
}

func TestViewContainsRegionElements(t *testing.T) {
	m := uiModel{width: 40}
	v := m.View()
	if !strings.Contains(v, "─") {
		t.Fatalf("view lacks hairline: %q", v)
	}
	if !strings.Contains(v, "> ") {
		t.Fatalf("view lacks prompt: %q", v)
	}
	if !strings.Contains(v, "? でヘルプ") {
		t.Fatalf("view lacks help hint: %q", v)
	}
}

func TestResizeChangesHairline(t *testing.T) {
	m := uiModel{width: 80}
	m, _ = m.Update(resizeEvent{width: 40})
	if m.width != 40 {
		t.Fatalf("resize width=%d", m.width)
	}
	if strings.Count(m.View(), "─") != 40*2 {
		// hairline 2本 × 40桁
		t.Fatalf("hairline width not updated: %d", strings.Count(m.View(), "─"))
	}
}

func TestNewFrontendDispatchesToDumb(t *testing.T) {
	fe := newFrontend(strings.NewReader(""), &bytes.Buffer{})
	if _, ok := fe.(*dumbFrontend); !ok {
		t.Fatalf("non-TTY input should dispatch to dumbFrontend, got %T", fe)
	}
}

// modelWith は折り返しテスト用に入力・カーソル・幅を持つモデルを作る。
func modelWith(width int, input string, cursor int) uiModel {
	return uiModel{width: width, input: []rune(input), cursor: cursor}
}

func atEnd(input string) int { return len([]rune(input)) }

func TestWrapPrompt(t *testing.T) {
	cases := []struct {
		name     string
		width    int
		input    string
		cursor   int
		wantRows []string
		wantRow  int
		wantCol  int
	}{
		{
			// 空入力: プロンプトのみの1行、カーソルは "> " の直後
			name: "empty", width: 80, input: "", cursor: 0,
			wantRows: []string{"> "}, wantRow: 0, wantCol: 2,
		},
		{
			// 幅80に収まる ASCII: 1行、カーソル末尾
			name: "ascii_fits", width: 80, input: "hello", cursor: atEnd("hello"),
			wantRows: []string{"> hello"}, wantRow: 0, wantCol: 7,
		},
		{
			// ASCII が幅10で折り返し（末尾カーソル）
			name: "ascii_wrap", width: 10, input: "abcdefghijkl", cursor: atEnd("abcdefghijkl"),
			wantRows: []string{"> abcdefgh", "ijkl"}, wantRow: 1, wantCol: 4,
		},
		{
			// 全角は桁跨ぎで分割せず次行へ送る（幅5）
			name: "wide_no_split", width: 5, input: "あいう", cursor: atEnd("あいう"),
			wantRows: []string{"> あ", "いう"}, wantRow: 1, wantCol: 4,
		},
		{
			// カーソル先頭（"> " の直後＝入力の前）
			name: "cursor_start", width: 80, input: "abc", cursor: 0,
			wantRows: []string{"> abc"}, wantRow: 0, wantCol: 2,
		},
		{
			// 折り返し行の途中にカーソル（'j' の直前）
			name: "cursor_mid_wrapped", width: 10, input: "abcdefghijkl", cursor: 9,
			wantRows: []string{"> abcdefgh", "ijkl"}, wantRow: 1, wantCol: 1,
		},
		{
			// 末尾行がちょうど満幅＋末尾カーソル → 空行を足しその先頭へ
			name: "exact_full_end", width: 4, input: "ab", cursor: atEnd("ab"),
			wantRows: []string{"> ab", ""}, wantRow: 1, wantCol: 0,
		},
		{
			// 満幅行でもカーソルが末尾でなければ空行は足さない
			name: "exact_full_mid", width: 4, input: "ab", cursor: 1,
			wantRows: []string{"> ab"}, wantRow: 0, wantCol: 3,
		},
		{
			// ASCII＋全角の境界で折り返し
			name: "mixed_boundary", width: 4, input: "aあb", cursor: atEnd("aあb"),
			wantRows: []string{"> a", "あb"}, wantRow: 1, wantCol: 3,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, row, col := modelWith(c.width, c.input, c.cursor).wrapPrompt()
			if !slices.Equal(rows, c.wantRows) {
				t.Errorf("rows=%q, want %q", rows, c.wantRows)
			}
			if row != c.wantRow || col != c.wantCol {
				t.Errorf("cursor=(%d,%d), want (%d,%d)", row, col, c.wantRow, c.wantCol)
			}
		})
	}
}

// TestWrapPromptDegenerate は極小幅でも panic/無限ループせず1行以上を返すことを確認する。
func TestWrapPromptDegenerate(t *testing.T) {
	for _, w := range []int{1, 2, 3} {
		rows, row, col := modelWith(w, "あ", 1).wrapPrompt()
		if len(rows) < 1 {
			t.Fatalf("width=%d: rows must be >=1, got %q", w, rows)
		}
		if row < 0 || row >= len(rows) || col < 0 {
			t.Fatalf("width=%d: invalid cursor (%d,%d) for rows=%q", w, row, col, rows)
		}
	}
}

// TestWrapPromptAlwaysAtLeastOneRow は空入力でも常に1行返ることを保証する。
func TestWrapPromptAlwaysAtLeastOneRow(t *testing.T) {
	rows, _, _ := (uiModel{}).wrapPrompt() // width 0 → hairlineWidth は80にフォールバック
	if len(rows) != 1 || rows[0] != "> " {
		t.Fatalf("default model wrapPrompt rows=%q", rows)
	}
}

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 3},
		{"あいう", 6},
		{"a?", 2},
		{"？", 2}, // 全角クエスチョン
		{"直近の", 6},
		{"> あ", 4}, // '>'=1, ' '=1, 'あ'=2
	}
	for _, c := range cases {
		if got := displayWidth(c.in); got != c.want {
			t.Errorf("displayWidth(%q)=%d, want %d", c.in, got, c.want)
		}
	}
}
