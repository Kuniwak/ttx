package cmd

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// sessionRecord は1演習の最小の永続状態です。クラッシュ（OOM 等のハード終了）後、
// 新しいゲートウェイが孤児 Codex を停止し、該当スレッドへ再起動を通知するために使います。
type sessionRecord struct {
	Key        string `json:"key"`         // = threadTS
	ChannelID  string `json:"channel_id"`  // 再起動通知の投稿先
	SocketPath string `json:"socket_path"` // 後始末で削除する stale socket
	CodexPID   int    `json:"codex_pid"`   // 孤児プロセスグループ刈り取り（pgid == pid）
	StartedAt  int64  `json:"started_at"`  // 起動時刻（unix 秒）
}

func sessionsDir(stateDir string) string {
	return filepath.Join(stateDir, "sessions")
}

func recordPath(stateDir, key string) string {
	return filepath.Join(sessionsDir(stateDir), key+".json")
}

// writeRecord は演習開始時に状態ファイルを書き出します。
func writeRecord(stateDir string, rec sessionRecord) error {
	if err := os.MkdirAll(sessionsDir(stateDir), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(recordPath(stateDir, rec.Key), data, 0o644)
}

// removeRecord は演習終了時に状態ファイルを削除します。存在しなくてもエラーにしません。
func removeRecord(stateDir, key string) error {
	err := os.Remove(recordPath(stateDir, key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// listRecords は残存する状態ファイル（＝前回起動で後始末されなかった演習）を読み出します。
// ディレクトリが無ければ空を返します。壊れたファイルはスキップします。
func listRecords(stateDir string) ([]sessionRecord, error) {
	entries, err := os.ReadDir(sessionsDir(stateDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []sessionRecord
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir(stateDir), e.Name()))
		if err != nil {
			continue
		}
		var rec sessionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		recs = append(recs, rec)
	}
	return recs, nil
}
