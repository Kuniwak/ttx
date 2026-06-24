package cmd

import (
	"strconv"
	"strings"
)

// unescapeText は --text に書かれた Go 形式のエスケープ（\n, \t, \\, \uXXXX など）を
// 実際の文字へ展開します。シェルのシングルクォート引数では \n が解釈されないため、
// GM が改行のつもりで書いた \n をここで改行に変換するのが目的です。
//
// quote=0 を使うので、未エスケープの " や ' は文字としてそのまま通ります（散文中の
// 引用符を壊さない）。その代わり \" は展開されず、下記の fail-open で生のまま残ります。
//
// 解釈に失敗した場合（不正なエスケープ・末尾の単独バックスラッシュなど）は、本文を
// 壊さないよう元の文字列をそのまま返します（部分展開はしない）。すでに含まれる実改行
// （LF）はエスケープではないのでそのまま通ります。
func unescapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	rest := s
	for len(rest) > 0 {
		c, _, tail, err := strconv.UnquoteChar(rest, 0)
		if err != nil {
			return s // 解釈不能なら生のまま返す（fail-open）
		}
		b.WriteRune(c)
		rest = tail
	}
	return b.String()
}
