package agent

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type previewApprover struct{ calls int }

func (p *previewApprover) ApproveComputerUse(context.Context, tool.ComputerUseTarget) (bool, error) {
	p.calls++
	return true, nil
}

type previewTool struct{ fakeImageTool }

func (*previewTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	approver := tool.ComputerUseApproverFrom(ctx)
	if approver == nil {
		return "missing approver", nil
	}
	target := tool.ComputerUseTarget{PID: 42, Name: "Fixture", BundleID: "example.fixture", LaunchTime: 100}
	if allow, err := approver.ApproveComputerUse(ctx, target); err != nil || !allow {
		return "denied", err
	}
	tool.PublishComputerUsePreview(ctx, tool.ComputerUsePreview{Target: target, WindowID: 11, Action: "observe", Image: "data:image/jpeg;base64,AAAA"})
	return "observed", nil
}

// Override ImageTool dispatch so the real execution path exercises the context.
func (p *previewTool) ExecuteWithImages(ctx context.Context, args json.RawMessage) (string, []string, error) {
	s, err := p.Execute(ctx, args)
	return s, nil, err
}

func TestAgentRoutesAppConsentAndPreviewToOwningTool(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(&previewTool{})
	prov := &scriptedProvider{name: "fixture", turns: [][]provider.Chunk{{toolCallChunk("preview-call", "shot", `{}`), {Type: provider.ChunkDone}}, {{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}}}}
	var previews []event.Event
	a := New(prov, reg, NewSession(""), Options{}, event.FuncSink(func(e event.Event) {
		if e.Tool.ComputerUsePreview != nil {
			previews = append(previews, e)
		}
	}))
	approver := &previewApprover{}
	a.SetComputerUseApprover(approver)
	if err := a.Run(t.Context(), "observe fixture"); err != nil {
		t.Fatal(err)
	}
	if approver.calls != 1 || len(previews) != 1 || previews[0].Kind != event.ToolProgress || previews[0].Tool.ID != "preview-call" {
		t.Fatalf("preview routing failed: calls=%d events=%+v", approver.calls, previews)
	}
	wire := eventwire.ToWire(previews[0])
	raw, err := json.Marshal(wire)
	if err != nil || wire.Tool.ComputerUsePreview.AppName != "Fixture" || len(raw) > 64<<10 {
		t.Fatalf("invalid desktop wire: %s %v", raw, err)
	}
}
