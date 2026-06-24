package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// webhookAlerter は Slack incoming webhook へ生 HTTP（stdlib のみ）で通報する alertFunc を
// 返します。slack-go SDK を使わないのは、SDK / Slack API の変更で通報経路まで道連れに
// しないためです（監視停止という最重要イベントは最小依存で確実に届けたい）。
func webhookAlerter(url string) alertFunc {
	return func(message string) error {
		body, err := json.Marshal(map[string]string{"text": message})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("webhook が異常応答を返しました: %d", resp.StatusCode)
		}
		return nil
	}
}

// stderrAlerter は webhook 未設定時に標準エラーへ通報する alertFunc を返します。
func stderrAlerter(w io.Writer) alertFunc {
	return func(message string) error {
		fmt.Fprintln(w, "[ttx-slack-sup ALERT] "+message)
		return nil
	}
}
