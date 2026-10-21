package control

import "reasonix/internal/provider"

func mergeInterruptedRecovery(dst, src *provider.InterruptedTurnRecovery) {
	dst.CompletedTools = append(dst.CompletedTools, src.CompletedTools...)
	dst.InterruptedTools = append(dst.InterruptedTools, src.InterruptedTools...)
	dst.NotStartedTools = append(dst.NotStartedTools, src.NotStartedTools...)
	dst.UnknownTools = append(dst.UnknownTools, src.UnknownTools...)
	if src.TerminalStatus != "" {
		dst.TerminalStatus = src.TerminalStatus
	}
	if src.FailureDiagnostic != nil {
		diagnostic := *src.FailureDiagnostic
		dst.FailureDiagnostic = &diagnostic
	}
}

func recordInterruptedAssistantRecovery(r *provider.InterruptedTurnRecovery, msgs []provider.Message, i int) {
	results := make(map[string]provider.Message)
	for j := i + 1; j < len(msgs) && msgs[j].Role == provider.RoleTool && !msgs[j].LocalOnly; j++ {
		result := msgs[j]
		results[result.ToolCallID+"\x00"+result.Name] = result
	}
	for _, call := range msgs[i].ToolCalls {
		state := provider.ToolRunUnknown
		if result, ok := results[call.ID+"\x00"+call.Name]; ok {
			state = provider.ToolResultRunState(result)
		}
		provider.RecordToolRecovery(r, interruptedToolSummary(call), state)
	}
}
