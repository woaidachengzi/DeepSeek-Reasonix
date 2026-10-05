package provider

import "testing"

func TestRequestUsageIsLocalMetadata(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: "answer", RequestUsage: &Usage{TotalTokens: 123}}}
	if got := ModelMessages(messages); got[0].RequestUsage != nil {
		t.Fatal("request usage entered provider projection")
	}
	if messages[0].RequestUsage == nil || ProjectionMessages(messages)[0].RequestUsage == nil {
		t.Fatal("provider projection mutated or compaction discarded accounting")
	}
}
