package cmd

import "testing"

func TestUnescapeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			// シェルのシングルクォートで素通しされたリテラル \n を改行へ展開する（本来の目的）。
			name: "リテラル \\n を改行に展開",
			in:   `1. おすすめ\n2. 部署から\n3. 一覧`,
			want: "1. おすすめ\n2. 部署から\n3. 一覧",
		},
		{
			name: "リテラル \\t をタブに展開",
			in:   `a\tb`,
			want: "a\tb",
		},
		{
			// \\ は \ に畳まれるので、二重化すればリテラルな \n を残せる（--text 内の逃げ道）。
			name: "二重バックスラッシュでリテラル \\n を残す",
			in:   `a\\nb`,
			want: `a\nb`,
		},
		{
			// すでに実改行（LF）を含む入力は壊さず素通しする。
			name: "実改行はそのまま素通し",
			in:   "line1\nline2",
			want: "line1\nline2",
		},
		{
			name: "Unicode エスケープを展開（Go unquote 相当）",
			in:   `あ\x41`,
			want: "あA",
		},
		{
			// 不正なエスケープは本文を壊さないよう、全体を生のまま返す（fail-open）。
			name: "不正エスケープは生のまま",
			in:   `a\zb`,
			want: `a\zb`,
		},
		{
			name: "末尾の単独バックスラッシュは生のまま",
			in:   `abc\`,
			want: `abc\`,
		},
		{
			// 未エスケープの引用符はそのまま通す（quote=0 の意図）。
			name: "未エスケープの引用符はそのまま",
			in:   `彼は "v2.3.1" と言った`,
			want: `彼は "v2.3.1" と言った`,
		},
		{
			name: "バックスラッシュを含まない本文は不変",
			in:   "演習を開始します。",
			want: "演習を開始します。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unescapeText(tc.in); got != tc.want {
				t.Fatalf("unescapeText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
