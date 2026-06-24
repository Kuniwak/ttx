package ttxipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/Kuniwak/ttx/jsonl"
)

// Dial は ttx-human の Unix Domain Socket へ接続します。
func Dial(socketPath string) (net.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("会話用プロセスへ接続できません（socket=%s）: %w", socketPath, err)
	}
	return conn, nil
}

// WriteLine は msg を JSONL 1行として w へ書き込みます。
// 行フレーミング（改行）は jsonl に委譲し、ここでは Message の marshal だけを行います。
func WriteLine(w io.Writer, msg Message) error {
	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return jsonl.WriteLine(w, encoded)
}

// ReadLine は r から JSONL 1行を読み、Message として返します。行が無ければ io.EOF。
func ReadLine(r io.Reader) (Message, error) {
	line, err := jsonl.ReadLine(r)
	if err != nil {
		return Message{}, err
	}
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Message{}, fmt.Errorf("メッセージをJSONとして読めません: %w", err)
	}
	return msg, nil
}

// Send は socket へ接続し、msg を1行書いて閉じます（GM→人間の1往復）。
func Send(socketPath string, msg Message) error {
	conn, err := Dial(socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := WriteLine(conn, msg); err != nil {
		return fmt.Errorf("メッセージを送信できません: %w", err)
	}
	return nil
}

// Recv は recv 制御 req を送り、応答1行を読みます。
// 戻り値の ok=false は「サーバが応答せず閉じた（--no-wait で滞留無し）」を表します。
func Recv(socketPath string, wait bool) (Message, bool, error) {
	conn, err := Dial(socketPath)
	if err != nil {
		return Message{}, false, err
	}
	defer conn.Close()

	req := Message{Type: TypeRecv}
	if wait {
		req.Text = RecvWait
	}
	if err := WriteLine(conn, req); err != nil {
		return Message{}, false, fmt.Errorf("recv要求を送信できません: %w", err)
	}

	msg, err := ReadLine(conn)
	if errors.Is(err, io.EOF) {
		// 応答なしで閉じられた = 滞留無し。
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("recv応答を受信できません: %w", err)
	}
	return msg, true, nil
}
