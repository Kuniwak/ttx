package cmd

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestBuildCodexCmdArgsEnvDir(t *testing.T) {
	ctx := context.Background()
	cmd := buildCodexCmd(ctx, "codex", []string{"exec"}, "/repo", "/tmp/ttx-thread.sock", "PROMPT", 0)

	// 引数は [bin, baseArgs..., prompt] の順。
	wantArgs := []string{"codex", "exec", "PROMPT"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("Args = %v, want %v", cmd.Args, wantArgs)
	}
	if cmd.Dir != "/repo" {
		t.Fatalf("Dir = %q, want /repo", cmd.Dir)
	}
	// per-thread の TTX_SOCKET が環境変数で渡る。
	if !slices.Contains(cmd.Env, "TTX_SOCKET=/tmp/ttx-thread.sock") {
		t.Fatalf("Env に TTX_SOCKET が無い: %v", cmd.Env)
	}
}

func TestNewExecCodexSpawnerComposesBaseArgs(t *testing.T) {
	tests := []struct {
		name      string
		extraArgs []string
		wantBase  []string // execCodexSpawner.baseArgs
		wantArgs  []string // buildCodexCmd の最終引数 [bin, baseArgs..., prompt]
	}{
		{
			name:      "追加引数なしは exec のみ",
			extraArgs: nil,
			wantBase:  []string{"exec"},
			wantArgs:  []string{"codex", "exec", "PROMPT"},
		},
		{
			name:      "追加引数は exec とプロンプトの間に順序保持で挿入",
			extraArgs: []string{"--model", "gpt-5-codex", "-s", "danger-full-access"},
			wantBase:  []string{"exec", "--model", "gpt-5-codex", "-s", "danger-full-access"},
			wantArgs:  []string{"codex", "exec", "--model", "gpt-5-codex", "-s", "danger-full-access", "PROMPT"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newExecCodexSpawner("codex", "/repo", tt.extraArgs)
			if !slices.Equal(s.baseArgs, tt.wantBase) {
				t.Fatalf("baseArgs = %v, want %v", s.baseArgs, tt.wantBase)
			}
			cmd := buildCodexCmd(context.Background(), s.bin, s.baseArgs, s.repoDir, "/tmp/ttx-thread.sock", "PROMPT", 0)
			if !slices.Equal(cmd.Args, tt.wantArgs) {
				t.Fatalf("Args = %v, want %v", cmd.Args, tt.wantArgs)
			}
		})
	}
}

func TestHeadlessPromptMentionsKeyConstraints(t *testing.T) {
	// シナリオ未指定。socket 接続断時に再試行せず終了する指示（/abort 相当）に加え、
	// 訓練モード・シナリオを IPC（ttx-ai-send/ttx-ai-recv）で尋ねる指示も含む。
	auto := headlessPrompt("")
	for _, want := range []string{"TTX_SOCKET", "00-run-exercise", "再試行", "/abort", "訓練モード", "ttx-ai-send", "ttx-ai-recv"} {
		if !strings.Contains(auto, want) {
			t.Fatalf("headlessPrompt(\"\") に %q が含まれていない:\n%s", want, auto)
		}
	}

	// シナリオ指定ありなら ID をプロンプトに含める。シナリオ選択質問は省略してよいが、
	// 訓練モードは依然 IPC で尋ねる（明示指定ショートカット）。
	withID := headlessPrompt("SCN-007")
	for _, want := range []string{"SCN-007", "訓練モード"} {
		if !strings.Contains(withID, want) {
			t.Fatalf("headlessPrompt(\"SCN-007\") に %q が含まれていない:\n%s", want, withID)
		}
	}
}
