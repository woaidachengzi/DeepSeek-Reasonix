package agent

import (
	"context"
	"encoding/json"
	"strings"

	"reasonix/internal/provider"
)

const computerUseRefusalPrefix = "computer_use refused input: "

func computerUseRefusal(name, result string) string {
	if name != "computer_use" {
		return ""
	}
	body, _ := computerUseResult(result)
	if string(body["dispatched"]) != "false" || string(body["dispatch_unknown"]) == "true" {
		return ""
	}
	var reason string
	_ = json.Unmarshal(body["blocked_reason"], &reason)
	return reason
}

func computerUseInputProgress(name, result string) bool {
	if name != "computer_use" {
		return false
	}
	body, _ := computerUseResult(result)
	return string(body["dispatched"]) == "true" || string(body["input_verification"]) == `"confirmed"`
}

func (a *Agent) computerUseRefusalImages(ctx context.Context, reason, text string, images []string) imageResult {
	if reason == "" {
		return imageResult{text: text}
	}
	return a.processToolImages(ctx, text, images)
}

func markComputerUseRefusal(out *toolOutcome, reason string, images []string, summary *provider.VisionSummary) {
	if reason == "" {
		return
	}
	out.blocked = true
	out.runState = provider.ToolRunNotStarted
	out.images = images
	out.visionSummary = summary
}

// Fresh screenshots and receipt tokens are observations, not input progress.
// Three refused input attempts without a dispatched action get one bounded
// finalization round. Only a new user turn starts a fresh attempt budget.
func (a *Agent) applyComputerUseProgress(ctx context.Context, calls []provider.ToolCall, outcomes []toolOutcome, receiptMark int) {
	for i, out := range outcomes {
		if i >= len(calls) || (!computerUseCall(calls[i]) && out.resolvedName != "computer_use") {
			continue
		}
		if out.blocked && strings.HasPrefix(out.errMsg, computerUseRefusalPrefix) {
			a.turn.computerUseRefusals++
		} else if out.computerUseInputProgress {
			a.turn.computerUseRefusals = 0
		}
		if a.turn.computerUseRefusals >= stormBreakThreshold {
			a.armLoopGuardPass(receiptMark)
			a.armFinalizationRound(ctx, &a.turn, landCause{kind: "computer_use_blocked", detail: "three desktop input refusals without a dispatched action; inspection did not reset progress"})
			return
		}
	}
}
