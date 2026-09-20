package protocolgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the schema's own tree: tests run from the package directory.
const repoRoot = "../../.."

// TestGenerateMatchesCommittedArtifacts makes the drift gate part of go test,
// not only of the explicit -check command CI runs.
func TestGenerateMatchesCommittedArtifacts(t *testing.T) {
	artifacts, err := Generate(repoRoot)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(artifacts))
	}
	if err := Check(repoRoot, artifacts); err != nil {
		t.Fatalf("committed mirrors drifted from the wire schema: %v", err)
	}
}

func TestGenerateEmitsBothWireMirrors(t *testing.T) {
	artifacts, err := Generate(repoRoot)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	byPath := map[string]string{}
	for _, artifact := range artifacts {
		byPath[artifact.Path] = string(artifact.Data)
	}

	rust, ok := byPath[RustArtifactPath]
	if !ok {
		t.Fatalf("%s was not generated", RustArtifactPath)
	}
	for _, want := range []string{"pub struct BridgeSession {", "pub struct BridgeEvent {", "pub sequence: u64,", "pub workspace_root: Option<String>,"} {
		if !strings.Contains(rust, want) {
			t.Errorf("rust mirror is missing %q", want)
		}
	}

	typescript, ok := byPath[TypeScriptArtifactPath]
	if !ok {
		t.Fatalf("%s was not generated", TypeScriptArtifactPath)
	}
	for _, want := range []string{"export interface BridgeSession {", "state: \"idle\" | \"running\" | \"paused\";", "workspaceRoot?: string;", "payload: Record<string, unknown>;"} {
		if !strings.Contains(typescript, want) {
			t.Errorf("typescript mirror is missing %q", want)
		}
	}
}

func TestCheckRejectsDrift(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, RustArtifactPath, "// stale\n")
	writeFile(t, root, TypeScriptArtifactPath, "// stale\n")

	artifacts, err := Generate(repoRoot)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	err = Check(root, artifacts)
	if err == nil {
		t.Fatal("Check accepted a stale artifact")
	}
	if !strings.Contains(err.Error(), "is stale") {
		t.Fatalf("Check reported %v, want a staleness error", err)
	}
	if !strings.Contains(err.Error(), artifacts[0].Path) {
		t.Fatalf("Check reported %v, want it to name %s", err, artifacts[0].Path)
	}
}

func TestCheckRejectsMissingArtifact(t *testing.T) {
	root := t.TempDir()
	artifacts, err := Generate(repoRoot)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := Check(root, artifacts); err == nil {
		t.Fatal("Check accepted a missing artifact")
	}
}

func TestGenerateKeepsStringConstLiterals(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, SchemaPath, `{"$defs":{"thing":{"type":"object","required":["status"],"properties":{"status":{"const":"ok"}}}}}`)
	artifacts, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, artifact := range artifacts {
		if artifact.Path != TypeScriptArtifactPath {
			continue
		}
		if !strings.Contains(string(artifact.Data), `status: "ok";`) {
			t.Errorf("typescript mirror lost the const literal:\n%s", artifact.Data)
		}
		return
	}
	t.Fatalf("%s was not generated", TypeScriptArtifactPath)
}

func TestGenerateRejectsUnsupportedConstructs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, SchemaPath, `{"$defs":{"thing":{"type":"array"}}}`)
	if _, err := Generate(root); err == nil {
		t.Fatal("Generate accepted an array without items")
	}

	writeFile(t, root, SchemaPath, `{"$defs":{"thing":{"type":"number"}}}`)
	if _, err := Generate(root); err == nil {
		t.Fatal("Generate accepted an unsupported type")
	}

	writeFile(t, root, SchemaPath, `{"$defs":{"thing":{"$ref":"#/other/thing"}}}`)
	if _, err := Generate(root); err == nil {
		t.Fatal("Generate accepted an unsupported $ref")
	}
}

func TestWriteReplacesStaleArtifacts(t *testing.T) {
	root := t.TempDir()
	artifacts, err := Generate(repoRoot)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, artifact := range artifacts {
		writeFile(t, root, artifact.Path, "// stale\n")
	}
	if err := Write(root, artifacts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Check(root, artifacts); err != nil {
		t.Fatalf("artifacts are still stale after Write: %v", err)
	}
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(relative), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}
