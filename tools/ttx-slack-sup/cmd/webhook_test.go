package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookAlerterPostsJSON(t *testing.T) {
	var gotBody map[string]string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := webhookAlerter(srv.URL)("クラッシュループ通報"); err != nil {
		t.Fatal(err)
	}
	if gotBody["text"] != "クラッシュループ通報" {
		t.Fatalf("webhook 本文 = %#v, text に通報文を入れるべき", gotBody)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
}

func TestWebhookAlerterErrorsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := webhookAlerter(srv.URL)("x"); err == nil {
		t.Fatal("異常応答ではエラーを返すべき")
	}
}

func TestStderrAlerterWrites(t *testing.T) {
	var b strings.Builder
	if err := stderrAlerter(&b)("監視停止"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "監視停止") {
		t.Fatalf("標準エラー通報に本文が無い: %q", b.String())
	}
}
