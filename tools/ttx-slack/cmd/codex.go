package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// CodexProcess は起動済みの GM プロセス（codex exec）のハンドルです。
type CodexProcess interface {
	// Wait はプロセスの終了を待ちます。終了の権威の1つ（自発 exit / クラッシュ）。
	Wait() error
	// Kill はプロセスグループへ SIGTERM→猶予→SIGKILL を送り、子孫ごと確実に終了させます。
	Kill() error
	// PID はプロセス ID を返します（未起動なら 0）。クラッシュ復旧時に状態ファイルへ記録し、
	// 再起動後の孤児プロセスグループ刈り取りに使います（プロセスグループ ID == このリーダー PID）。
	PID() int
}

// CodexSpawner は演習1件ごとに GM プロセスを起動します。
type CodexSpawner interface {
	// Spawn は per-thread の socketPath を TTX_SOCKET として渡して GM を起動します。
	// scenarioArg が非空ならそのシナリオを、空なら自動選定を指示します。
	Spawn(ctx context.Context, socketPath, scenarioArg string) (CodexProcess, error)
}

// codexKillGrace は SIGTERM から SIGKILL までの既定猶予です。
const codexKillGrace = 5 * time.Second

// execCodexSpawner は実際に codex exec をサブプロセスとして起動する CodexSpawner です。
type execCodexSpawner struct {
	bin      string   // codex バイナリのパス
	baseArgs []string // exec とプロンプトの間に置く引数。先頭は "exec"、続いて呼び出し側が与えた追加引数列（--codex-exec-arg。既定は -s danger-full-access -c model_reasoning_effort）。
	repoDir  string   // 演習リポジトリの作業ディレクトリ
	grace    time.Duration
}

// newExecCodexSpawner は codex exec 起動器を作ります。extraArgs は "exec" の後ろ・プロンプトの前へ
// そのまま挿入され、--model や -s などスパイクで判明したフラグを再ビルド無しに渡すために使います。
func newExecCodexSpawner(bin, repoDir string, extraArgs []string) *execCodexSpawner {
	baseArgs := append([]string{"exec"}, extraArgs...)
	return &execCodexSpawner{bin: bin, baseArgs: baseArgs, repoDir: repoDir, grace: codexKillGrace}
}

func (s *execCodexSpawner) Spawn(ctx context.Context, socketPath, scenarioArg string) (CodexProcess, error) {
	cmd := buildCodexCmd(ctx, s.bin, s.baseArgs, s.repoDir, socketPath, headlessPrompt(scenarioArg), s.grace)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex を起動できません: %w", err)
	}
	return newExecCodexProcess(cmd, s.grace), nil
}

// buildCodexCmd は codex exec の *exec.Cmd を組み立てます（起動はしません）。
// per-thread socket を TTX_SOCKET 環境変数で渡し、別プロセスグループに置きます。
// ctx キャンセル時は Cancel フックでグループへ SIGTERM を送り、WaitDelay 経過後に
// exec が強制終了させます。
func buildCodexCmd(ctx context.Context, bin string, baseArgs []string, repoDir, socketPath, prompt string, grace time.Duration) *exec.Cmd {
	args := append(append([]string{}, baseArgs...), prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "TTX_SOCKET="+socketPath)
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return terminateGroup(cmd, syscall.SIGTERM) }
	if grace > 0 {
		cmd.WaitDelay = grace
	}
	return cmd
}

// execCodexProcess は *exec.Cmd を CodexProcess として包みます。
// 起動時に内部で1度だけ cmd.Wait を回し、done のクローズで終了を周知します。
type execCodexProcess struct {
	cmd     *exec.Cmd
	grace   time.Duration
	done    chan struct{}
	waitErr error
}

func newExecCodexProcess(cmd *exec.Cmd, grace time.Duration) *execCodexProcess {
	p := &execCodexProcess{cmd: cmd, grace: grace, done: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()
	return p
}

func (p *execCodexProcess) Wait() error {
	<-p.done
	return p.waitErr
}

func (p *execCodexProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *execCodexProcess) Kill() error {
	select {
	case <-p.done:
		return nil // 既に終了済み
	default:
	}
	// グループへ SIGTERM → 猶予内に終了しなければ SIGKILL。
	_ = terminateGroup(p.cmd, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(p.grace):
		_ = terminateGroup(p.cmd, syscall.SIGKILL)
	}
	return nil
}

var _ CodexSpawner = (*execCodexSpawner)(nil)
var _ CodexProcess = (*execCodexProcess)(nil)

// headlessPrompt は codex exec へ渡す起動プロンプトを組み立てます。
// 別窓起動・画面隠し・カウントダウンといった物理画面前提のセットアップは Slack では
// 不要なため省略しますが、訓練モード（全体／個別）とシナリオの選択は CLI と同じく
// IPC（ttx-ai-send --type question / ttx-ai-recv）で参加者へ尋ねます。
// socket は既に立っている前提（TTX_SOCKET）。
func headlessPrompt(scenarioArg string) string {
	var b strings.Builder
	b.WriteString("このリポジトリの障害対応机上演習をヘッドレス（非対話）で進めてください。\n")
	b.WriteString("会話用の Unix Domain Socket は既に起動済みで、環境変数 TTX_SOCKET にパスがあります。\n")
	b.WriteString("ttx-ai-recv / ttx-ai-send はこの TTX_SOCKET を使ってください。\n")
	b.WriteString("ttx-ai-recv が接続失敗（socket が無い／接続できない）または「入力なし（キュー閉鎖）」で戻ったら、")
	b.WriteString("会話プロセスが終了した＝ゲートウェイが演習を打ち切ったことを意味します。再接続・再試行・ポストモーテム作成をせず、ただちに終了してください（/abort 相当）。\n")
	b.WriteString("別窓での ttx-human 起動・画面隠し・カウントダウン等の物理画面前提の対話的セットアップは行いません。")
	b.WriteString("ただし訓練モード（全体／個別）の選択とシナリオ選択は省略せず、ttx-ai-send（--type question）で参加者へ尋ねて ttx-ai-recv で回答を受け取ってください（.ai/skills/00-run-exercise のステップ3.5「設定質疑フェーズ」）。\n")
	b.WriteString(".ai/skills/00-run-exercise のヘッドレス手順に従って演習本体（設定質疑→タイムライン生成→質疑ループ→ポストモーテム送信）を進めてください。\n")
	if scenarioArg != "" {
		fmt.Fprintf(&b, "シナリオは %s を使うので、シナリオ選択の質問は省略してかまいません。ただし訓練モード（全体／個別）の選択は ttx-ai-send（--type question）で必ず参加者へ尋ねてください。\n", scenarioArg)
	} else {
		b.WriteString("シナリオIDは指定されていないので、シナリオ選択も訓練モード選択と同じく ttx-ai-send（--type question）で参加者へ尋ねてください。\n")
	}
	return b.String()
}
