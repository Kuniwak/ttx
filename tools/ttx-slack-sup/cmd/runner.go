package cmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
)

// execRunner は target（コマンドと引数）を1回起動し、終了まで待つ childRunner を返します。
// 子の標準出力は素通しし、標準エラーは末尾を保持して通報に含めます。
func execRunner(target []string) childRunner {
	return func(ctx context.Context) (error, string) {
		cmd := exec.CommandContext(ctx, target[0], target[1:]...)
		cmd.Stdout = os.Stdout
		tail := &tailBuffer{max: 4096}
		cmd.Stderr = io.MultiWriter(os.Stderr, tail)
		err := cmd.Run()
		return err, tail.String()
	}
}

// tailBuffer は書き込まれた末尾 max バイトだけを保持する並行安全な io.Writer です。
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
