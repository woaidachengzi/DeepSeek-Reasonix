package recovery

import (
	"encoding/json"
	"testing"
)

func TestArgsSummaryKeepsCompleteCharacters(t *testing.T) {
	for _, field := range []string{"command", "path", "file_path"} {
		args, _ := json.Marshal(map[string]string{field: "👩‍💻中文"})
		if got := ArgsSummary(args, 2); got != "👩‍💻中…" {
			t.Fatalf("%s: %q", field, got)
		}
	}
	if got := ArgsSummary([]byte(`{"command":"中文"}`), 2); got != "中文" {
		t.Fatalf("unnecessary truncation: %q", got)
	}
}
