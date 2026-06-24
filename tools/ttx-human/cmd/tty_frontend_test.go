//go:build !windows

package cmd

import (
	"bufio"
	"strings"
	"testing"
)

// TestDecodeEscape は ESC 後続バイト列の解析を検証する。入力は ESC を除いた後続バイト
// （'[' または 'O' から始まる）を与える。矢印/Home/End に加え、Delete("[3~") を確認し、
// 未対応シーケンス（Insert="[2~"、上="[A" 等）は ok=false になることを保証する。
func TestDecodeEscape(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want keyKind
		ok   bool
	}{
		{"right", "[C", keyRight, true},
		{"left", "[D", keyLeft, true},
		{"home", "[H", keyHome, true},
		{"end", "[F", keyEnd, true},
		{"home_ss3", "OH", keyHome, true},
		{"delete", "[3~", keyDelete, true},
		{"delete_modified", "[3;5~", keyDelete, true},
		{"insert_ignored", "[2~", 0, false},
		{"pgup_ignored", "[5~", 0, false},
		{"up_ignored", "[A", 0, false},
		{"down_ignored", "[B", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(c.in))
			k, ok := decodeEscape(br)
			if ok != c.ok || (ok && k != c.want) {
				t.Fatalf("decodeEscape(%q)=(%v,%v), want (%v,%v)", c.in, k, ok, c.want, c.ok)
			}
		})
	}
}
