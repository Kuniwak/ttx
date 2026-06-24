package cmd

import (
	"slices"
	"testing"

	"github.com/Kuniwak/ttx/cli"
)

func TestParseOptionsCodexExecArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "省略時は既定（-s danger-full-access -c model_reasoning_effort=low）",
			args: nil,
			want: []string{"-s", "danger-full-access", "-c", `model_reasoning_effort="low"`},
		},
		{
			name: "指定時は指定値のみ（既定は付与せず完全上書き・順序保持）",
			args: []string{
				"--codex-exec-arg", "-s",
				"--codex-exec-arg", "workspace-write",
				"--codex-exec-arg", "--model",
				"--codex-exec-arg", "gpt-5-codex",
			},
			want: []string{"-s", "workspace-write", "--model", "gpt-5-codex"},
		},
		{
			name: "空文字指定は空引数1個（既定クリアではない）",
			args: []string{"--codex-exec-arg", ""},
			want: []string{""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := ParseOptions(tt.args, cli.StubProcInout())
			if err != nil {
				t.Fatalf("ParseOptions error: %v", err)
			}
			if !slices.Equal(opts.CodexExecArgs, tt.want) {
				t.Fatalf("CodexExecArgs = %v, want %v", opts.CodexExecArgs, tt.want)
			}
		})
	}
}
