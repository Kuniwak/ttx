package ttxserver

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kuniwak/ttx/ttxipc"
)

func TestHumanMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want ttxipc.Message
	}{
		{
			name: "質問はinput（GMが分類する）",
			in:   "直近のリリースはありますか？",
			want: ttxipc.Message{Type: "input", Text: "直近のリリースはありますか？"},
		},
		{
			name: "スラッシュ無しの行動もinput",
			in:   "復旧宣言します",
			want: ttxipc.Message{Type: "input", Text: "復旧宣言します"},
		},
		{
			name: "スラッシュ付きの行動もinput（/はもう行動を意味しない）",
			in:   "/復旧宣言します",
			want: ttxipc.Message{Type: "input", Text: "/復旧宣言します"},
		},
		{
			name: "giveupはcontrol",
			in:   "/giveup",
			want: ttxipc.Message{Type: "control", Text: "/giveup"},
		},
		{
			name: "giveupの後続文言は保持してcontrol",
			in:   "/giveup もう無理です",
			want: ttxipc.Message{Type: "control", Text: "/giveup もう無理です"},
		},
		{
			name: "giveupの別語はcontrol扱いしない",
			in:   "/giveupnow",
			want: ttxipc.Message{Type: "input", Text: "/giveupnow"},
		},
		{
			name: "claimは本文ありでcontrol",
			in:   "/claim 部長判断は不要です",
			want: ttxipc.Message{Type: "control", Text: "/claim 部長判断は不要です"},
		},
		{
			name: "claimは本文なしでもcontrol（GMが争点を尋ねる）",
			in:   "/claim",
			want: ttxipc.Message{Type: "control", Text: "/claim"},
		},
		{
			name: "claimの別語はcontrol扱いしない",
			in:   "/claimfoo",
			want: ttxipc.Message{Type: "input", Text: "/claimfoo"},
		},
		{
			name: "abortはcontrol",
			in:   "/abort",
			want: ttxipc.Message{Type: "control", Text: "/abort"},
		},
		{
			name: "abortの別語はcontrol扱いしない",
			in:   "/abortnow",
			want: ttxipc.Message{Type: "input", Text: "/abortnow"},
		},
		{
			name: "trim",
			in:   "  エラー率を確認してください  ",
			want: ttxipc.Message{Type: "input", Text: "エラー率を確認してください"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HumanMessage(tt.in)
			if got != tt.want {
				t.Fatalf("HumanMessage() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHelpTextListsCommandsAndAlias(t *testing.T) {
	h := HelpText()
	for _, want := range []string{"/help", "(?)", "/giveup", "/claim", "/abort", "本番の障害対応で使ってよいもの"} {
		if !strings.Contains(h, want) {
			t.Fatalf("HelpText() に %q が含まれていません:\n%s", want, h)
		}
	}
}

func TestWriteJSONLineOnlyTypeAndText(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSONLine(&buf, ttxipc.Message{Type: "input", Text: "直近のリリースはありますか？"}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	want := `{"type":"input","text":"直近のリリースはありますか？"}`
	if got != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
}
