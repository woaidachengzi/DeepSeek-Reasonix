package agent

import (
	"strings"
	"testing"
)

// Independent pre-fast-path implementation, including repeated stripping and
// the exact order of suffix removal and whitespace normalization.
func legacyStripTransientForTest(content string) string {
	s := unwrapMemoryCompilerExecution(content)
	for {
		next := reTransientUserBlock.ReplaceAllStringFunc(s, func(string) string { return "" })
		if next == s {
			break
		}
		s = next
	}
	s = stripTrailingDeliveryRuntime(s)
	s = reTrailingExecutionPolicy.ReplaceAllString(s, "")
	s = stripTrailingMemoryRecall(s)
	return strings.TrimLeft(s, " \t\r\n")
}

func TestPlainUserTextPreviewFastPathParity(t *testing.T) {
	for _, content := range []string{
		"", " \t\r\n", "exact user text  ", " \t中文😀\r\n", "\u2003 unicode whitespace ",
		"question\x00with\xffinvalid bytes", " \n# " + executorHandoffMarker + "\nOriginal task:\nquestion\n\nPlanner output:\nprivate",
		strings.Repeat("a plain historical question ", 1000),
		"<response-language>zh</response-language>\nquestion", "a < b and c > d",
		"<memory-compiler-execution>{\"source_event\":\"question\"}</memory-compiler-execution>",
		"question\n<execution-policy mode=\"safe\">private</execution-policy>",
	} {
		if got, want := StripTransientUserBlocks(content), legacyStripTransientForTest(content); got != want {
			t.Fatalf("preview mismatch for %q: got %q want %q", content, got, want)
		}
	}
	handoff := "# " + executorHandoffMarker + "\nOriginal task:\n exact task \n\nPlanner output:\nprivate"
	if got := UserPreviewText(handoff); got != "exact task" {
		t.Fatalf("plain-text fast path bypassed handoff extraction: %q", got)
	}
	text := "plain user text"
	if allocations := testing.AllocsPerRun(100, func() { StripTransientUserBlocks(text) }); allocations != 0 {
		t.Fatalf("plain preview allocated %g times", allocations)
	}
}
