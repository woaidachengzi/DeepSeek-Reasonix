package builtin

import (
	"context"
	"io"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/jobs"
)

func TestRunningJobReadersExplainTimeoutWithoutPromisingWakeup(t *testing.T) {
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ctx := jobs.WithSession(jobs.WithManager(t.Context(), m), "session")
	release := make(chan struct{})
	defer close(release)
	j := m.StartForSession("session", "bash", "build", func(ctx context.Context, _ io.Writer) (string, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return "built", nil
	})
	args := []byte(`{"job_id":"` + j.ID + `"}`)
	out, err := (bashOutput{}).Execute(ctx, args)
	if err != nil || !strings.Contains(out, runningJobNote) {
		t.Fatalf("running output: %q, %v", out, err)
	}
	out, err = (waitJob{}).Execute(ctx, []byte(`{"job_ids":["`+j.ID+`"],"timeout_seconds":1}`))
	if err != nil || !strings.Contains(out, runningJobNote) || !strings.Contains(out, "does not mean failure") {
		t.Fatalf("timed out wait: %q, %v", out, err)
	}
	if !strings.Contains(out, "do not automatically start a turn") {
		t.Fatal("local host must not promise automatic continuation")
	}
	if out, _ := (bashOutput{}).Execute(jobs.WithSession(ctx, "other"), args); strings.Contains(out, runningJobNote) {
		t.Fatal("foreign job was exposed")
	}
}

func TestFinishedJobOmitsRunningAdvice(t *testing.T) {
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ctx := jobs.WithManager(t.Context(), m)
	j := m.Start("bash", "build", func(context.Context, io.Writer) (string, error) { return "built", nil })
	out, err := (waitJob{}).Execute(ctx, []byte(`{"job_ids":["`+j.ID+`"]}`))
	if err != nil || !strings.Contains(out, "built") || strings.Contains(out, runningJobNote) {
		t.Fatalf("finished wait: %q, %v", out, err)
	}
	out, err = (bashOutput{}).Execute(ctx, []byte(`{"job_id":"`+j.ID+`"}`))
	if err != nil || strings.Contains(out, runningJobNote) {
		t.Fatalf("finished output: %q, %v", out, err)
	}
}
