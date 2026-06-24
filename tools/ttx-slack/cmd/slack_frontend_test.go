package cmd

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Kuniwak/ttx/ttxipc"
)

// fakePoster は投稿文を記録する試験用の投稿先。Post メソッドを threadPoster 関数として渡す。
type fakePoster struct {
	mu    sync.Mutex
	posts []string
	err   error
}

func (p *fakePoster) Post(text string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.posts = append(p.posts, text)
	return nil
}

func (p *fakePoster) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.posts...)
}

func TestSlackFrontendReadLineReturnsSubmittedInput(t *testing.T) {
	fe := newSlackFrontend((&fakePoster{}).Post, func() {})
	fe.Submit("直近のリリースは?")

	got, err := fe.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine err = %v", err)
	}
	if got != "直近のリリースは?" {
		t.Fatalf("ReadLine = %q", got)
	}
}

func TestSlackFrontendReadLineEOFOnClose(t *testing.T) {
	fe := newSlackFrontend((&fakePoster{}).Post, func() {})
	if err := fe.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fe.ReadLine(); err != io.EOF {
		t.Fatalf("Close 後の ReadLine は io.EOF を返すべき: %v", err)
	}
}

func TestSlackFrontendCloseIsIdempotent(t *testing.T) {
	fe := newSlackFrontend((&fakePoster{}).Post, func() {})
	if err := fe.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fe.Close(); err != nil {
		t.Fatalf("多重 Close は安全であるべき: %v", err)
	}
}

func TestSlackFrontendDisplayAgentPostsAnswer(t *testing.T) {
	p := &fakePoster{}
	fe := newSlackFrontend(p.Post, func() {})
	fe.DisplayAgent(ttxipc.Message{Type: string(ttxipc.TypeAnswer), Text: "本日午前に v2.3.1 をデプロイ済みです。"})

	posts := p.all()
	if len(posts) != 1 {
		t.Fatalf("投稿数 = %d, want 1", len(posts))
	}
	if !strings.Contains(posts[0], "v2.3.1") {
		t.Fatalf("投稿本文に回答が含まれていない: %q", posts[0])
	}
}

func TestSlackFrontendDisplayLocalPosts(t *testing.T) {
	p := &fakePoster{}
	fe := newSlackFrontend(p.Post, func() {})
	fe.DisplayLocal("利用可能なコマンド: ...")

	posts := p.all()
	if len(posts) != 1 || !strings.Contains(posts[0], "利用可能なコマンド") {
		t.Fatalf("DisplayLocal が投稿されていない: %#v", posts)
	}
}

func TestSlackFrontendSkipsEmptyText(t *testing.T) {
	p := &fakePoster{}
	fe := newSlackFrontend(p.Post, func() {})
	fe.DisplayAgent(ttxipc.Message{Type: string(ttxipc.TypeAnswer), Text: ""})
	fe.DisplayLocal("")

	if posts := p.all(); len(posts) != 0 {
		t.Fatalf("空テキストは投稿しないべき: %#v", posts)
	}
}

func TestSlackFrontendPostmortemTriggersFinish(t *testing.T) {
	p := &fakePoster{}
	finished := 0
	fe := newSlackFrontend(p.Post, func() { finished++ })
	fe.DisplayAgent(ttxipc.Message{Type: string(ttxipc.TypePostmortem), Text: "# ポストモーテム\n..."})

	if finished != 1 {
		t.Fatalf("postmortem 受信で onFinish が1回呼ばれるべき: finished=%d", finished)
	}
	if posts := p.all(); len(posts) != 1 {
		t.Fatalf("postmortem も投稿されるべき: %#v", posts)
	}
}

func TestSlackFrontendNonPostmortemDoesNotFinish(t *testing.T) {
	finished := 0
	fe := newSlackFrontend((&fakePoster{}).Post, func() { finished++ })
	fe.DisplayAgent(ttxipc.Message{Type: string(ttxipc.TypeAnswer), Text: "回答"})
	fe.DisplayAgent(ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "通知"})
	fe.DisplayAgent(ttxipc.Message{Type: string(ttxipc.TypeQuestion), Text: "全体訓練と個別訓練どちらにしますか？"})

	if finished != 0 {
		t.Fatalf("answer/notification/question では onFinish を呼ばないべき: finished=%d", finished)
	}
}

func TestSlackFrontendSetThinkingPosts(t *testing.T) {
	p := &fakePoster{}
	fe := newSlackFrontend(p.Post, func() {})

	fe.SetThinking(true)
	posts := p.all()
	if len(posts) != 1 || !strings.Contains(posts[0], "思考中") {
		t.Fatalf("SetThinking(true) で思考中メッセージが投稿されていない: %#v", posts)
	}

	// 解除時は何も投稿しない（次の GM メッセージが答えになる）。
	fe.SetThinking(false)
	if len(p.all()) != 1 {
		t.Fatalf("SetThinking(false) で追加投稿してはいけない: %#v", p.all())
	}
}

func TestFormatForSlackByType(t *testing.T) {
	tests := []struct {
		name     string
		msg      ttxipc.Message
		contains []string
	}{
		{"answer はそのまま", ttxipc.Message{Type: string(ttxipc.TypeAnswer), Text: "回答本文"}, []string{"回答本文"}},
		{"notification は見出し付き", ttxipc.Message{Type: string(ttxipc.TypeNotification), Text: "アラート発火"}, []string{"📣", "アラート発火"}},
		{"question は問い装飾付き", ttxipc.Message{Type: string(ttxipc.TypeQuestion), Text: "全体訓練と個別訓練どちらにしますか？"}, []string{"❓", "全体訓練と個別訓練どちらにしますか？"}},
		{"postmortem は見出しと終了セパレータ", ttxipc.Message{Type: string(ttxipc.TypePostmortem), Text: "本文"}, []string{"ポストモーテム", "本文", "演習を終了しました"}},
		{"error は警告付き", ttxipc.Message{Type: string(ttxipc.TypeError), Text: "不正なtype"}, []string{"⚠️", "不正なtype"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatForSlack(tt.msg)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("formatForSlack(%s) = %q, %q を含むべき", tt.msg.Type, got, want)
				}
			}
		})
	}
}
