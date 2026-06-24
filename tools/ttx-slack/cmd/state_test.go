package cmd

import (
	"testing"
)

func TestWriteListRemoveRecord(t *testing.T) {
	dir := t.TempDir()
	rec := sessionRecord{Key: "100.1", ChannelID: "C1", SocketPath: "/tmp/x.sock", CodexPID: 1234, StartedAt: 42}

	if err := writeRecord(dir, rec); err != nil {
		t.Fatal(err)
	}

	recs, err := listRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0] != rec {
		t.Fatalf("listRecords = %#v, want [%#v]", recs, rec)
	}

	if err := removeRecord(dir, "100.1"); err != nil {
		t.Fatal(err)
	}
	recs, _ = listRecords(dir)
	if len(recs) != 0 {
		t.Fatalf("削除後も記録が残っている: %#v", recs)
	}

	// 存在しない記録の削除はエラーにしない。
	if err := removeRecord(dir, "missing"); err != nil {
		t.Fatalf("存在しない記録の削除でエラー: %v", err)
	}
}

func TestListRecordsMissingDirIsEmpty(t *testing.T) {
	recs, err := listRecords(t.TempDir()) // sessions/ がまだ無い
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("空であるべき: %#v", recs)
	}
}
