package cmd

import (
	"bufio"
	"fmt"
	"io"
	"sync"

	"github.com/Kuniwak/ttx/ttxipc"
	"github.com/Kuniwak/ttx/ttxserver"
)

// newFrontend は stdin/stdout が対話端末（TTY）のときだけ対話UIを返し、
// それ以外（パイプ・リダイレクト・テストの strings.Reader/bytes.Buffer 等）は
// 従来挙動の dumbFrontend を返す。これにより非対話実行は現状と完全に同じになる。
func newFrontend(stdin io.Reader, stdout io.Writer) ttxserver.Frontend {
	if fe, ok := tryTTYFrontend(stdin, stdout); ok {
		return fe
	}
	return newDumbFrontend(stdin, stdout)
}

// dumbFrontend は従来挙動（bufio.Scanner で1行読み、メッセージを Fprintln）と等価。
// 表示は人間入力ポンプ（ローカル /help）と各接続の handleConn（GM）から
// 並行に呼ばれ得るので、stdout 書き込みを mu で直列化する。
//
// stdin は専用の readLoop goroutine で読み、確定行を lines チャネルへ流す。ReadLine は
// lines と done の select で待つので、stdin がブロック中でも done のクローズ（Close）で
// 解除できる（ttyFrontend と同じく Close で ReadLine が io.EOF を返す契約に揃える）。
type dumbFrontend struct {
	out io.Writer
	mu  sync.Mutex

	lines     chan string   // readLoop → ReadLine（確定行）。stdin 終端でクローズ。
	done      chan struct{} // Close で閉じ、ReadLine と readLoop を解除する。
	closeOnce sync.Once
	scanErr   error // stdin の非 EOF 読み取りエラー（readLoop が close(lines) 前に格納）。
}

func newDumbFrontend(stdin io.Reader, stdout io.Writer) *dumbFrontend {
	d := &dumbFrontend{
		out:   stdout,
		lines: make(chan string),
		done:  make(chan struct{}),
	}
	go d.readLoop(stdin)
	return d
}

// readLoop は stdin の唯一の読み手。確定行を lines へ送り、stdin が尽きたら lines を閉じて
// ReadLine に io.EOF を返させる。done が閉じられたら（Close）読み取りを止める。
func (d *dumbFrontend) readLoop(stdin io.Reader) {
	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		select {
		case d.lines <- scanner.Text():
		case <-d.done:
			return
		}
	}
	// 非 EOF エラーは ReadLine が surface できるよう、close(lines) の前に格納する
	// （close→受信の happens-before により ReadLine からの参照は同期する）。
	d.scanErr = scanner.Err()
	close(d.lines)
}

func (d *dumbFrontend) ReadLine() (string, error) {
	select {
	case line, ok := <-d.lines:
		if !ok {
			if d.scanErr != nil {
				return "", d.scanErr
			}
			return "", io.EOF
		}
		return line, nil
	case <-d.done:
		return "", io.EOF
	}
}

func (d *dumbFrontend) DisplayAgent(msg ttxipc.Message) {
	if msg.Text != "" {
		d.DisplayLocal(msg.Text)
	}
}

func (d *dumbFrontend) DisplayLocal(text string) {
	d.mu.Lock()
	fmt.Fprintln(d.out, text)
	d.mu.Unlock()
}

func (d *dumbFrontend) SetThinking(on bool) {}

// Finish は表示が同期（Fprintln）済みなので done を閉じるだけでよい（ReadLine が io.EOF を返す）。
func (d *dumbFrontend) Finish() {
	d.closeOnce.Do(func() { close(d.done) })
}

func (d *dumbFrontend) Close() error {
	d.closeOnce.Do(func() { close(d.done) })
	return nil
}
