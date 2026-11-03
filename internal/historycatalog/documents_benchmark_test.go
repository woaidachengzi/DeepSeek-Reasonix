package historycatalog

import (
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func BenchmarkHistoryDocuments(b *testing.B) {
	for _, name := range []string{"latin", "mixed"} {
		b.Run(name, func(b *testing.B) {
			text := strings.Repeat("Synthetic HISTORY indexing: tool_output item42. ", 128)
			if name == "mixed" {
				text = strings.Repeat("中文历史 搜索日本語 한국어 MixedCase_42 ", 128)
			}
			messages := make([]provider.Message, 1000)
			for i := range messages {
				messages[i] = provider.Message{Role: provider.RoleTool, Name: "fixture", Content: text}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if len(documents(messages)) != len(messages) {
					b.Fatal("unexpected document count")
				}
			}
		})
	}
}
