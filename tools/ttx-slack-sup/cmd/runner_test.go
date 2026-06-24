package cmd

import (
	"strings"
	"testing"
)

func TestTailBufferKeepsTail(t *testing.T) {
	b := &tailBuffer{max: 8}
	if _, err := b.Write([]byte("0123456789ABCDEF")); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if got != "89ABCDEF" {
		t.Fatalf("末尾 8 バイトを保持すべき: %q", got)
	}
}

func TestTailBufferUnderMax(t *testing.T) {
	b := &tailBuffer{max: 8}
	_, _ = b.Write([]byte("abc"))
	_, _ = b.Write([]byte("de"))
	if got := b.String(); got != "abcde" {
		t.Fatalf("max 未満はそのまま保持すべき: %q", got)
	}
	if !strings.HasPrefix(b.String(), "abc") {
		t.Fatal("先頭が消えてはいけない")
	}
}
