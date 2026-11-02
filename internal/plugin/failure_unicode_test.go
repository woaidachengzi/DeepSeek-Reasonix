package plugin

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFailureSummaryKeepsUnicodeAndRedactsCredentials(t *testing.T) {
	text := strings.Repeat("中文👩‍💻", 200)
	got := summarizeFailureError(errors.New(text))
	if !utf8.ValidString(got) || got != strings.Repeat("中文👩‍💻", 166)+"中文..." {
		t.Fatal("failure summary split a character or grapheme")
	}
	got = summarizeFailureError(errors.New("启动失败 Authorization: Bearer summary-secret-value"))
	if strings.Contains(got, "summary-secret-value") || !strings.Contains(got, "启动失败") {
		t.Fatalf("credential redaction regressed: %q", got)
	}
}
