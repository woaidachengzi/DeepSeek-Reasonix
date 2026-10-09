package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/eventwire"
	remotecontroller "reasonix/internal/remote/controller"
)

const previewDesktopPromptLimit = 128
const previewDesktopPromptLifetime = 15 * time.Minute

type previewDesktopPromptTicket struct {
	route     bot.DesktopWatchRoute
	actor     string
	scope     desktopbridge.OwnedCommandScope
	prompt    desktopbridge.OwnedPrompt
	payload   json.RawMessage
	issued    time.Time
	attempted bool
}

// Tickets are per-chat/per-actor capabilities captured for display, not raw
// core IDs. A delayed IM reply never re-finds a new prompt by the old ID.
// They remain private until the full host performs audience/permission checks.
type previewDesktopPrompts struct {
	manager *desktopbridge.RuntimeManager
	mu      sync.Mutex
	tickets map[string]previewDesktopPromptTicket
	closed  bool
}

func newPreviewDesktopPrompts(manager *desktopbridge.RuntimeManager) *previewDesktopPrompts {
	return &previewDesktopPrompts{manager: manager, tickets: make(map[string]previewDesktopPromptTicket)}
}

func (p *previewDesktopPrompts) pruneLocked() {
	if p.closed || p.manager == nil {
		clear(p.tickets)
		return
	}
	view, ok := p.manager.CommandSnapshot()
	for id, ticket := range p.tickets {
		present := false
		if ok && ticket.scope == view.Scope {
			for _, prompt := range view.State.Pending {
				if prompt == ticket.prompt {
					present = true
					break
				}
			}
		}
		if !present || time.Since(ticket.issued) > previewDesktopPromptLifetime {
			delete(p.tickets, id)
		}
	}
}

func (p *previewDesktopPrompts) Issue(ctx context.Context, route bot.DesktopWatchRoute, actor string, scope desktopbridge.OwnedCommandScope, prompt desktopbridge.OwnedPrompt) (string, json.RawMessage, error) {
	if ctx == nil || ctx.Err() != nil || !validPreviewWatcher(previewDesktopWatcher{route, actor}) {
		return "", nil, errPreviewDesktopBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked()
	if p.closed || p.manager == nil {
		return "", nil, errPreviewDesktopBinding
	}
	payload, err := p.manager.ReadOwnedPrompt(ctx, scope, prompt)
	if err != nil {
		return "", nil, err
	}
	for id, ticket := range p.tickets {
		if ticket.route == route && ticket.actor == actor && ticket.scope == scope && ticket.prompt == prompt {
			if ctx.Err() != nil {
				return "", nil, errPreviewDesktopBinding
			}
			// Refreshing display never rearms an attempted/unknown decision.
			return id, append(json.RawMessage(nil), ticket.payload...), nil
		}
	}
	if len(p.tickets) >= previewDesktopPromptLimit {
		return "", nil, errPreviewDesktopBinding
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", nil, errPreviewDesktopBinding
	}
	id := "p-" + hex.EncodeToString(random[:])
	if _, exists := p.tickets[id]; exists || ctx.Err() != nil {
		return "", nil, errPreviewDesktopBinding
	}
	p.tickets[id] = previewDesktopPromptTicket{route: route, actor: actor, scope: scope, prompt: prompt, payload: append(json.RawMessage(nil), payload...), issued: time.Now()}
	return id, append(json.RawMessage(nil), payload...), nil
}

func (p *previewDesktopPrompts) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if ctx == nil || ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked()
	ticket, ok := p.tickets[command.TargetID]
	if !ok || ticket.attempted || ticket.route != command.Route || ticket.actor != command.ActorID || ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	var answer json.RawMessage
	switch command.Action {
	case "approve", "deny":
		// Plan/Recovery/MCP must use their specialized decisions, never a generic
		// allow bit. Full host registration is still to be connected.
		if ticket.prompt.Kind != "approval" {
			return "", errPreviewDesktopBinding
		}
		answer, _ = json.Marshal(struct {
			Allow bool `json:"allow"`
		}{command.Action == "approve"})
	case "answer":
		if ticket.prompt.Kind != "ask" {
			return "", errPreviewDesktopBinding
		}
		var wire eventwire.Event
		if err := json.Unmarshal(ticket.payload, &wire); err != nil || wire.Ask == nil {
			return "", errPreviewDesktopBinding
		}
		questions := make([]event.AskQuestion, len(wire.Ask.Questions))
		for i, q := range wire.Ask.Questions {
			questions[i] = event.AskQuestion{ID: q.ID, Header: q.Header, Prompt: q.Prompt, Multi: q.Multi}
			for _, option := range q.Options {
				questions[i].Options = append(questions[i].Options, event.AskOption{Label: option.Label, Description: option.Description})
			}
		}
		answers, err := bot.ParseDesktopAskAnswers(questions, command.AnswerText)
		if err != nil {
			return "", err
		}
		encoded := make([]remotecontroller.SessionPromptQuestionAnswer, len(answers))
		for i, a := range answers {
			encoded[i] = remotecontroller.SessionPromptQuestionAnswer{QuestionID: a.QuestionID, Selected: a.Selected}
		}
		answer, _ = json.Marshal(struct {
			Questions []remotecontroller.SessionPromptQuestionAnswer `json:"questions"`
		}{encoded})
	case "plan", "recovery", "mcp":
		if ticket.prompt.Kind != command.Action {
			return "", errPreviewDesktopBinding
		}
		text := strings.TrimSpace(command.AnswerText)
		fields := strings.Fields(text)
		if len(fields) == 0 {
			return "", errPreviewDesktopBinding
		}
		action := fields[0]
		remainder := strings.TrimSpace(text[len(action):])
		if command.Action == "mcp" {
			var content json.RawMessage
			if remainder != "" {
				content = json.RawMessage(remainder)
				if !json.Valid(content) {
					return "", errPreviewDesktopBinding
				}
			}
			answer, _ = json.Marshal(struct {
				Action  string          `json:"action"`
				Content json.RawMessage `json:"content,omitempty"`
			}{action, content})
		} else {
			answer, _ = json.Marshal(struct {
				Action   string `json:"action"`
				Feedback string `json:"feedback,omitempty"`
			}{action, remainder})
		}
	default:
		return "", errPreviewDesktopBinding
	}
	// Use the same strict five-kind codec before reserving a one-shot attempt.
	// Known malformed input does not burn a usable ticket or reach the manager.
	if len(answer) == 0 || len(answer) > 64<<10 {
		return "", errPreviewDesktopBinding
	}
	if _, err := remotecontroller.DecodeSessionPromptAnswer(remotecontroller.SessionPromptRequest{SessionPromptScope: remotecontroller.SessionPromptScope{SessionPath: ticket.scope.SessionPath, RuntimeEpoch: ticket.scope.RuntimeEpoch, TurnID: ticket.prompt.TurnID, PromptID: ticket.prompt.ID, PromptRuntimeEpoch: ticket.prompt.RuntimeEpoch, Kind: ticket.prompt.Kind}, Answer: answer}); err != nil {
		return "", errPreviewDesktopBinding
	}
	// Once dispatch is attempted, even an error/unknown result cannot retry this
	// capability. Invalid local parsing above leaves the draft ticket available.
	ticket.attempted = true
	p.tickets[command.TargetID] = ticket
	if err := p.manager.ResolveOwnedPrompt(ctx, ticket.scope, ticket.prompt, answer); err != nil {
		return "", err
	}
	return "已提交该提示的决定；这不表示任务或保存已完成。", nil
}

func (p *previewDesktopPrompts) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	clear(p.tickets)
}
