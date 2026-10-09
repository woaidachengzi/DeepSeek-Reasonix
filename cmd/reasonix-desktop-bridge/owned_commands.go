package main

import (
	"context"
	"encoding/json"

	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	remotecontroller "reasonix/internal/remote/controller"
)

var _ desktopbridge.RuntimeOwnedCommands = (*controllerRuntime)(nil)
var _ desktopbridge.RuntimeOwnedPromptReader = (*controllerRuntime)(nil)

func (r *controllerRuntime) ReadOwnedPrompt(ctx context.Context, scope desktopbridge.OwnedCommandScope, prompt desktopbridge.OwnedPrompt) (json.RawMessage, error) {
	return r.controller.ReadPromptScopedContext(ctx, control.PromptResolveScope{SessionPath: scope.SessionPath, RuntimeEpoch: scope.RuntimeEpoch, TurnID: prompt.TurnID}, control.PromptIdentity{PromptID: prompt.ID, TurnID: prompt.TurnID, RuntimeEpoch: prompt.RuntimeEpoch, Kind: control.PromptKind(prompt.Kind)})
}

func (r *controllerRuntime) ActivateOwnedEvents(scope desktopbridge.OwnedCommandScope) {
	if r.lifecycleSink != nil && r.lifecycleSink.ownedSource != nil {
		r.lifecycleSink.ownedSource.Activate(scope)
	}
}

func (r *controllerRuntime) CommandSnapshot() (string, desktopbridge.OwnedCommandState) {
	state := r.controller.RuntimeStateSnapshot()
	view := desktopbridge.OwnedCommandState{Revision: state.Revision, Phase: state.Phase, Running: state.Running,
		TurnID: state.TurnID, PendingPrompt: state.PendingPrompt, CancelRequested: state.CancelRequested}
	// Prompt identities can advance after this read. Exact resolution checks
	// again under the core's prompt mutex; never treat this list as authority.
	for _, prompt := range r.controller.PendingPromptIdentities() {
		if prompt.TurnID == state.TurnID && prompt.PromptID != "" && state.Running && !state.CancelRequested {
			view.Pending = append(view.Pending, desktopbridge.OwnedPrompt{ID: prompt.PromptID, Kind: string(prompt.Kind), TurnID: prompt.TurnID, RuntimeEpoch: prompt.RuntimeEpoch})
		}
	}
	return state.RuntimeEpoch, view
}

func (r *controllerRuntime) SubmitOwnedContext(ctx context.Context, view desktopbridge.OwnedCommandView, input string) error {
	return r.controller.SubmitScopedContext(ctx, control.TurnSubmitScope{SessionPath: view.Scope.SessionPath,
		RuntimeEpoch: view.Scope.RuntimeEpoch, Revision: view.State.Revision}, input)
}

func (r *controllerRuntime) ResolveOwnedContext(ctx context.Context, scope desktopbridge.OwnedCommandScope, prompt desktopbridge.OwnedPrompt, raw json.RawMessage) error {
	input := remotecontroller.SessionPromptRequest{SessionPromptScope: remotecontroller.SessionPromptScope{
		SessionPath: scope.SessionPath, RuntimeEpoch: scope.RuntimeEpoch, TurnID: prompt.TurnID,
		PromptID: prompt.ID, PromptRuntimeEpoch: prompt.RuntimeEpoch, Kind: prompt.Kind}, Answer: raw}
	answer, err := remotecontroller.DecodeSessionPromptAnswer(input)
	if err != nil {
		return err
	}
	questions := make([]event.AskAnswer, len(answer.Questions))
	for i, question := range answer.Questions {
		questions[i] = event.AskAnswer{QuestionID: question.QuestionID, Selected: append([]string(nil), question.Selected...)}
	}
	return r.controller.ResolvePromptScopedContext(ctx, control.PromptResolveScope{SessionPath: scope.SessionPath,
		RuntimeEpoch: scope.RuntimeEpoch, TurnID: prompt.TurnID}, control.PromptIdentity{PromptID: prompt.ID,
		TurnID: prompt.TurnID, RuntimeEpoch: prompt.RuntimeEpoch, Kind: control.PromptKind(prompt.Kind)},
		control.PromptAnswer{Questions: questions, Allow: answer.Allow, Session: answer.Session, Persist: answer.Persist,
			Action: answer.Action, Feedback: answer.Feedback, Content: answer.Content})
}
