package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kuniwak/ttx/ttxserver"
)

// renderScript は固定幅 width で uiModel にイベント列を適用し、実際の表示と同じ
// 「全画面イメージ」（スクロールバック＋入力リージョン、ANSIエスケープ込み）を再構成する。
// 実レンダループ（ttyFrontend.redraw）と同じ renderBlock / View を使い、メッセージ間に
// 空行を1つ入れる整形も揃えるため、見た目の乖離が起きない。端末に一切触れないので
// CI / 非TTY でも安定動作する。
func renderScript(width int, events []event) string {
	m := uiModel{width: width}
	out := ""
	for _, ev := range events {
		var effs []effect
		m, effs = m.Update(ev)
		for _, e := range effs {
			if pa, ok := e.(effectPrintAbove); ok {
				out += renderBlock(pa.prefix, pa.text) + "\n\n"
			}
		}
	}
	out += m.View()
	return out
}

func firstEvent() event {
	return agentEvent{text: "PagerDutyで注文APIの5xx増加アラートが発火しました。"}
}

type visualCase struct {
	name   string
	width  int
	events []event
}

func visualCases() []visualCase {
	return []visualCase{
		{
			// ① 初期（最初のイベント開示後・空入力）
			name:   "01_initial",
			width:  80,
			events: []event{firstEvent()},
		},
		{
			// ② 入力中(スクロールバック無し)
			name:   "02_typing",
			width:  80,
			events: typeRunes("直近のリリースはありますか？"),
		},
		{
			// ③ 送信直後(自分の入力をエコー＋思考インジケータ)
			name:  "03_thinking",
			width: 80,
			events: append(append([]event{firstEvent()}, typeRunes("直近のリリースはありますか？")...),
				keyEvent{kind: keyEnter}, thinkingEvent{on: true}),
		},
		{
			// ④ GM 応答後
			name:  "04_answered",
			width: 80,
			events: append(append([]event{firstEvent()}, typeRunes("直近のリリースはありますか？")...),
				keyEvent{kind: keyEnter}, thinkingEvent{on: true},
				agentEvent{text: "本日午前に v2.3.1 をデプロイしています。"}),
		},
		{
			// ⑤ 入力中に非同期通知が到着(入力は保持され再描画)
			name:  "05_async_notify",
			width: 80,
			events: append(typeRunes("エラー率を確認"),
				agentEvent{text: "10:02 注文APIのエラー率がさらに上昇しています。"}),
		},
		{
			// ⑥ ヘルプ（/help または別名 ? → readHumanLoop がローカル応答し ● メッセージで表示）
			name:   "06_help",
			width:  80,
			events: []event{agentEvent{text: ttxserver.HelpText()}},
		},
		{
			// ⑦ 端末幅を超える長い入力（幅20）。プロンプトが複数行に折り返され、
			//    上下の hairline が折り返し行の上下に正しく配置されること（重なり防止）。
			name:   "07_wrap",
			width:  20,
			events: typeRunes("直近のデプロイ履歴と現在のエラー率を確認したい"),
		},
		{
			// ⑧ 起動バナー（listening on …）が接続先案内とともに ● の
			//    スクロールバック行として入力欄の上に揃って表示される。
			name:   "08_listening",
			width:  80,
			events: []event{agentEvent{text: listeningBanner("/tmp/ttx.sock")}},
		},
		{
			// ⑨ 開始通知（thinking:true）。メッセージを表示した後も思考インジケータが回る。
			//    server が thinking:true のメッセージで行う「表示（agentEvent）→ その後
			//    SetThinking(true)（thinkingEvent）」を再現する。表示自体は思考中を一旦
			//    false にするが、直後の thinkingEvent{on:true} で再点灯する。最初のイベント
			//    開示（フラグなしの agentEvent）が来ると解除される。
			name:  "09_bootstrap_thinking",
			width: 80,
			events: []event{
				agentEvent{text: "演習を開始します。最初のメッセージまで1〜2分ほどお待ちください。"},
				thinkingEvent{on: true},
			},
		},
	}
}

// TestVisualGolden は実装の描画が testdata/*.golden（＝画面の仕様）と一致することを検証する。
// ゴールデンは仕様であり、実装はそれに合わせる。仕様変更時のみ、ゴールデンを意図的に
// 編集してから実装を追従させること（実装出力でゴールデンを自動上書きする手段は用意しない）。
func TestVisualGolden(t *testing.T) {
	for _, tc := range visualCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := renderScript(tc.width, tc.events)
			golden := filepath.Join("testdata", tc.name+".golden")
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("ゴールデン(仕様)が読めません: %v", err)
			}
			if got != string(want) {
				t.Errorf("%s が仕様(ゴールデン)と不一致。実装を仕様に合わせて修正してください。"+
					"（仕様自体を変える場合のみ testdata のゴールデンを意図的に編集する）\ngot:\n%q\nwant:\n%q",
					tc.name, got, string(want))
			}
		})
	}
}
