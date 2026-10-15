package plugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"reasonix/internal/tool"
)

// Opt-in acceptance of the exact browser-use preset with real Chrome and MCP.
// Headless is added only for the test; the product opens a visible window.
func TestPlaywrightMCPLive(t *testing.T) {
	if os.Getenv("REASONIX_LIVE_PLAYWRIGHT_MCP") != "1" {
		t.Skip("set REASONIX_LIVE_PLAYWRIGHT_MCP=1; requires npx and Google Chrome")
	}
	oldPATH := stdioShellPATH
	stdioShellPATH = func(context.Context) string { return os.Getenv("PATH") }
	t.Cleanup(func() { stdioShellPATH = oldPATH })
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><title>Reasonix browser acceptance</title>
<label>Name <input id="name"></label><button onclick="document.querySelector('output').textContent='Hello '+document.querySelector('input').value">Greet</button><output>Waiting</output>`))
	}))
	defer fixture.Close()
	connect := func() (*Host, []tool.Tool) {
		t.Helper()
		host := NewHost()
		t.Cleanup(func() { host.Close() })
		result, err := host.InstallAndConnect(ctx, Spec{
			Name: "reasonix-browser", Command: "npx",
			Args:       []string{"-y", "@playwright/mcp@0.0.83", "--browser", "chrome", "--isolated", "--caps", "vision", "--no-webmcp", "--headless"},
			Authorized: true, ProcessMode: MCPProcessHost, StateDir: t.TempDir(), WorkspaceRoot: t.TempDir(),
			StartupTimeout: 120 * time.Second, CallTimeout: 90 * time.Second,
		})
		if err != nil || result.State != "ready" {
			t.Fatalf("connect browser MCP: result=%+v err=%v", result, err)
		}
		tools, err := host.ToolsFor(ctx, "reasonix-browser")
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"browser_navigate", "browser_snapshot", "browser_click", "browser_type", "browser_evaluate", "browser_take_screenshot", "browser_mouse_click_xy"} {
			if findLiveMCPTool(tools, name) == nil {
				t.Fatalf("required tool %s missing from %v", name, toolNames(tools))
			}
		}
		t.Logf("connected real Playwright MCP: %d tools", len(tools))
		return host, tools
	}
	call := func(host *Host, name string, args map[string]any) string {
		t.Helper()
		data, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		for attempt := range 5 {
			tools, err := host.ToolsFor(ctx, "reasonix-browser")
			if err != nil {
				t.Fatal(err)
			}
			result, err := findLiveMCPTool(tools, name).Execute(ctx, data)
			if err == nil {
				return result
			}
			// These adapter errors occur before dispatch. Browser startup can
			// change the catalog; rediscover rather than reuse stale authorization.
			if !strings.Contains(err.Error(), "changed its tool catalog") && !strings.Contains(err.Error(), "after this call was authorized") {
				t.Fatalf("%s: %v", name, err)
			}
			if attempt == 4 {
				t.Fatalf("%s catalog never settled: %v", name, err)
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
		return ""
	}
	firstHost, first := connect()
	out := call(firstHost, "browser_navigate", map[string]any{"url": fixture.URL})
	if !strings.Contains(out, "Reasonix browser acceptance") {
		t.Fatalf("navigation did not expose the fixture: %s", out)
	}
	snapshot := call(firstHost, "browser_snapshot", map[string]any{})
	reference := func(label string) string {
		t.Helper()
		matches := regexp.MustCompile(label + `[^\n]*\[ref=([^\]]+)\]`).FindStringSubmatch(snapshot)
		if len(matches) != 2 {
			t.Fatalf("snapshot lacks element reference for %s: %s", label, snapshot)
		}
		return matches[1]
	}
	// Pinned releases have used both ref and target. Read the live schema so the
	// test passes the actual reference argument, just as capability discovery does.
	target := func(name, ref string) map[string]any {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(findLiveMCPTool(first, name).Schema(), &schema); err != nil {
			t.Fatal(err)
		}
		key := "ref"
		if _, ok := schema.Properties["target"]; ok {
			key = "target"
		}
		return map[string]any{key: ref}
	}
	typeArgs := target("browser_type", reference(`textbox "Name"`))
	typeArgs["text"] = "Reasonix"
	call(firstHost, "browser_type", typeArgs)
	snapshot = call(firstHost, "browser_snapshot", map[string]any{})
	call(firstHost, "browser_click", target("browser_click", reference(`button "Greet"`)))
	out = call(firstHost, "browser_snapshot", map[string]any{})
	if !strings.Contains(out, "Hello Reasonix") {
		t.Fatalf("click did not produce the expected greeting: %s", out)
	}
	t.Log("navigate → snapshot → type → click → verified greeting")
	freshTools, err := firstHost.ToolsFor(ctx, "reasonix-browser")
	if err != nil {
		t.Fatal(err)
	}
	shot := findLiveMCPTool(freshTools, "browser_take_screenshot")
	imageTool, ok := shot.(tool.ImageTool)
	if !ok {
		t.Fatal("screenshot tool is not image-capable")
	}
	_, images, err := imageTool.ExecuteWithImages(ctx, json.RawMessage(`{"type":"png"}`))
	if err != nil || len(images) != 1 || !strings.HasPrefix(images[0], "data:image/png;base64,") {
		t.Fatalf("screenshot must return one PNG image: count=%d err=%v", len(images), err)
	}
	imageBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(images[0], "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	dimensions, err := png.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil || dimensions.Width < 100 || dimensions.Height < 100 {
		t.Fatalf("invalid screenshot dimensions: %+v err=%v", dimensions, err)
	}
	t.Logf("MCP image channel returned a valid %dx%d PNG", dimensions.Width, dimensions.Height)
	call(firstHost, "browser_evaluate", map[string]any{"function": `() => { localStorage.setItem('reasonix-isolation', 'session-one'); return true; }`})
	secondHost, _ := connect()
	call(secondHost, "browser_navigate", map[string]any{"url": fixture.URL})
	out = call(secondHost, "browser_evaluate", map[string]any{"function": `() => ({saved: localStorage.getItem('reasonix-isolation'), greeting: document.querySelector('output').textContent})`})
	if !strings.Contains(out, `"saved": null`) || !strings.Contains(out, "Waiting") {
		t.Fatalf("second session inherited browser state: %s", out)
	}
	out = call(firstHost, "browser_snapshot", map[string]any{})
	if !strings.Contains(out, "Hello Reasonix") {
		t.Fatalf("second session changed first session's page: %s", out)
	}
	t.Log("two simultaneous MCP hosts have independent pages and local storage")
}
