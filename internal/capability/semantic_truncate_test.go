package capability

import (
	"testing"
	"unicode/utf8"
)

func TestSemanticDescriptionKeepsCompleteCharacters(t *testing.T) {
	for _, text := range []string{"中文描述", "👩‍💻说明", "e\u0301说明"} {
		got := truncate(text, 1)
		if !utf8.ValidString(got) || got == "…" {
			t.Fatalf("invalid description: %q", got)
		}
	}
	if got := truncate(" 👩‍💻说明 ", 1); got != "👩‍💻…" {
		t.Fatalf("split emoji: %q", got)
	}
	if got := truncate(" 中文 ", 2); got != "中文" {
		t.Fatalf("unnecessary truncation: %q", got)
	}
}
