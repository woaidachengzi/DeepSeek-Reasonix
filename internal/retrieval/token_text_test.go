package retrieval

import (
	"strings"
	"testing"
)

func TestTokenTextVocabularyParity(t *testing.T) {
	for _, text := range []string{"", "_", "Hello_WORLD42 FooBar", "中a日 한語", "中文日本語 한글", "x\xffy", "İΣÉ e\u0301 １２３", "中文,中文,中", strings.Repeat("中ABC_42.한글 ", 10000)} {
		assertTokenTextParity(t, text)
	}
}

func FuzzTokenTextVocabularyParity(f *testing.F) {
	for _, seed := range []string{"", "hello WORLD42", "中文日本語 한글 a中b", "A\xffB", "İΣÉ"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) { assertTokenTextParity(t, text) })
}

func assertTokenTextParity(t *testing.T, text string) {
	t.Helper()
	tokens := Tokens(text)
	got, count := TokenText(text)
	if got != strings.Join(tokens, " ") || count != len(tokens) {
		t.Fatalf("token vocabulary or count changed for %q", text)
	}
}
