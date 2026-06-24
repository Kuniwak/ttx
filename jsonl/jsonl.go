// Package jsonl は JSONL（1行1レコード）の行フレーミングを提供する汎用ユーティリティです。
// ドメイン型に依存せず、1行分のバイト列の読み書きだけを担います
// （各レコードの JSON マーシャル/アンマーシャルは呼び出し側が行います）。
package jsonl

import (
	"bufio"
	"fmt"
	"io"
)

// MaxLineBytes は1行の上限です。ポストモーテム等の長文に備えて広めに取ります。
const MaxLineBytes = 4 << 20 // 4 MiB

// WriteLine は line を1行（line + "\n"）として w へ書き込みます。
func WriteLine(w io.Writer, line []byte) error {
	_, err := fmt.Fprintf(w, "%s\n", line)
	return err
}

// ReadLine は r から1行を読み、改行を除いたバイト列を返します。行が無ければ io.EOF。
// 1回の呼び出しで1行だけを読む用途（1接続1メッセージの req/resp 等）を想定し、
// 内部で都度 Scanner を作るため、同じ r へ複数行をパイプラインする用途には使いません。
func ReadLine(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	// scanner.Bytes() は次の Scan で上書きされうるため、コピーして返す。
	line := scanner.Bytes()
	out := make([]byte, len(line))
	copy(out, line)
	return out, nil
}
