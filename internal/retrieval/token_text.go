package retrieval

import (
	"strings"
	"unicode"
)

// TokenText joins the V1 index vocabulary without allocating each term and
// an intermediate slice. Term order and count stay identical to Tokens.
func TokenText(s string) (string, int) {
	var out strings.Builder
	var prev rune
	count, cjkRun := 0, 0
	inWord := false
	begin := func() {
		if count > 0 {
			out.WriteByte(' ')
		}
		count++
	}
	endCJK := func() {
		if cjkRun == 1 {
			begin()
			out.WriteRune(prev)
		}
		cjkRun = 0
	}
	for _, r := range s {
		switch {
		case isCJK(r):
			inWord = false
			if cjkRun > 0 {
				begin()
				out.WriteRune(prev)
				out.WriteRune(r)
			}
			prev = r
			cjkRun++
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			endCJK()
			if !inWord {
				begin()
				inWord = true
			}
			out.WriteRune(unicode.ToLower(r))
		default:
			inWord = false
			endCJK()
		}
	}
	endCJK()
	return out.String(), count
}
