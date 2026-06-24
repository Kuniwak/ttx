package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReconcileNotifiesAndCleans(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	// 前回起動の残骸を捏造: stale socket ファイル + 記録（CodexPID=0 なので kill は no-op）。
	sockDir, err := os.MkdirTemp("/tmp", "ttxsock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	stale := filepath.Join(sockDir, "ttx-100.1.sock")
	if err := os.WriteFile(stale, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := sessionRecord{Key: "100.1", ChannelID: "C1", SocketPath: stale, CodexPID: 0, StartedAt: 1}
	if err := writeRecord(g.stateDir, rec); err != nil {
		t.Fatal(err)
	}

	g.Reconcile()

	if !containsSubstr(client.threadTexts("100.1"), "再起動") {
		t.Fatalf("再起動通知が投稿されていない: %#v", client.threadTexts("100.1"))
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale socket が削除されていない: stat err=%v", err)
	}
	recs, _ := listRecords(g.stateDir)
	if len(recs) != 0 {
		t.Fatalf("記録が削除されていない: %#v", recs)
	}
}

func TestReconcileNoRecordsIsNoop(t *testing.T) {
	client := &fakeSlackClient{}
	g := newTestGateway(t, client, &fakeSpawner{}, 3, time.Hour, time.Now)

	g.Reconcile()

	if client.total() != 0 {
		t.Fatalf("記録が無いのに投稿された: %d", client.total())
	}
}

func TestStartSessionWritesRecord(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})

	recs, _ := listRecords(g.stateDir)
	if len(recs) != 1 || recs[0].Key != "100.1" || recs[0].ChannelID != "C1" {
		t.Fatalf("開始時に状態ファイルが書かれていない: %#v", recs)
	}
}

func TestFinishRemovesRecord(t *testing.T) {
	client := &fakeSlackClient{}
	spawner := &fakeSpawner{}
	g := newTestGateway(t, client, spawner, 3, time.Hour, time.Now)

	g.HandleMention(MentionEvent{ChannelID: "C1", MessageTS: "100.1", UserText: "start"})
	g.HandleMention(MentionEvent{ChannelID: "C1", ThreadTS: "100.1", MessageTS: "100.2", UserText: "/abort"})

	recs, _ := listRecords(g.stateDir)
	if len(recs) != 0 {
		t.Fatalf("終了後も状態ファイルが残っている: %#v", recs)
	}
}
