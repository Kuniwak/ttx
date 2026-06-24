package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Kuniwak/ttx/ttxserver"
)

// Config は ttx-human の起動設定です。
type Config struct {
	SocketPath  string
	PublicLog   string
	RecvTimeout time.Duration // recv 待機のアイドルタイムアウト（0 で無期限）
}

// serveSocket は ttx-human 用に frontend（TTY/dumb）と公開ログを用意し、サーバ中核
// ttxserver.Serve へ委譲します。接続が閉じても終了せず、人間側入力（stdin）の EOF で終了します。
func serveSocket(cfg Config, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	// 対話端末なら新UI、非TTY（パイプ/リダイレクト/テスト）なら従来挙動の dumbFrontend。
	fe := newFrontend(stdin, stdout)
	defer fe.Close()

	logFile, err := os.OpenFile(cfg.PublicLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("公開ログを開けません: %w", err)
	}
	defer logFile.Close()

	// ttx-human は stdin（frontend）の EOF で終了するため、外部キャンセルは使わない。
	return ttxserver.Serve(context.Background(), cfg.SocketPath, fe, ttxserver.Hooks{
		LogWriter:   logFile,
		OnListening: func(socketPath string) { fe.DisplayLocal(listeningBanner(socketPath)) },
	}, cfg.RecvTimeout)
}

// listeningBanner は起動時に入力欄の上へ表示する接続先案内テキストを組み立てます。
// 1行目に listen 中の socket パス、2行目に接続できないときの対処を添えます。
// フロントエンドが ● のスクロールバック行として表示します（dumb は素のテキスト）。
func listeningBanner(socketPath string) string {
	return "listening on " + socketPath +
		"\n接続できないときは、このパスを --socket か TTX_SOCKET に指定してください。"
}
