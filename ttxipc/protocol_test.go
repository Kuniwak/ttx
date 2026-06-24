package ttxipc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageThinkingMarshaling(t *testing.T) {
	// thinking:true は明示的に出力される。
	b, err := json.Marshal(Message{Type: "notification", Text: "x", Thinking: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); !strings.Contains(got, `"thinking":true`) {
		t.Fatalf("thinking:true が出力されていない: %s", got)
	}

	// thinking 既定（false）は omitempty で省略される（従来メッセージの出力を変えない）。
	b, err = json.Marshal(Message{Type: "answer", Text: "y"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); strings.Contains(got, "thinking") {
		t.Fatalf("thinking 既定は省略されるべき: %s", got)
	}

	// 受信側でも往復する。
	var m Message
	if err := json.Unmarshal([]byte(`{"type":"notification","text":"x","thinking":true}`), &m); err != nil {
		t.Fatal(err)
	}
	if !m.Thinking {
		t.Fatalf("unmarshal で Thinking=true にならない: %#v", m)
	}
}

func TestParseSendType(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    SendableType
		wantErr bool
	}{
		{"answer は許可", "answer", TypeAnswer, false},
		{"notification は許可", "notification", TypeNotification, false},
		{"question は許可", "question", TypeQuestion, false},
		{"postmortem は許可", "postmortem", TypePostmortem, false},
		{"error は許可", "error", TypeError, false},
		{"input は拒否（参加者起点）", TypeInput, "", true},
		{"control は拒否（参加者起点）", TypeControl, "", true},
		{"recv は拒否（制御req）", TypeRecv, "", true},
		{"未知の type は拒否", "foo", "", true},
		{"空文字は拒否", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSendType(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSendType(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ParseSendType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
