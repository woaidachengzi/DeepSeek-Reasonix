package sessionidentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveIdentityPathExistingMissingAndSymlinkBoundaries(t *testing.T) {
	root := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(realDir, "session.jsonl")
	if err := os.WriteFile(realFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type resolutionCase struct {
		name, path, want string
		wantError        bool
	}
	checkCases := func(parent *testing.T, cases []resolutionCase) {
		for _, test := range cases {
			parent.Run(test.name, func(t *testing.T) {
				got, err := resolveIdentityPath(test.path)
				if test.wantError {
					if err == nil {
						t.Fatalf("resolveIdentityPath(%q) = %q, want error", test.path, got)
					}
					return
				}
				if err != nil || got != test.want {
					t.Fatalf("resolveIdentityPath(%q) = %q, %v; want %q", test.path, got, err, test.want)
				}
			})
		}
	}
	checkCases(t, []resolutionCase{
		{name: "existing file", path: realFile, want: filepath.Join(resolvedRoot, "real", "session.jsonl")},
		{name: "missing child", path: filepath.Join(realDir, "missing.jsonl"), want: filepath.Join(resolvedRoot, "real", "missing.jsonl")},
		{name: "missing descendants", path: filepath.Join(realDir, "new", "deep", "missing.jsonl"), want: filepath.Join(resolvedRoot, "real", "new", "deep", "missing.jsonl")},
		{name: "file as parent", path: filepath.Join(realFile, "missing.jsonl"), wantError: true},
	})
	t.Run("symlinks", func(t *testing.T) {
		aliasDir := filepath.Join(root, "alias")
		if err := os.Symlink(realDir, aliasDir); err != nil {
			t.Skipf("directory symlinks unavailable: %v", err)
		}
		dangling := filepath.Join(root, "dangling")
		if err := os.Symlink(filepath.Join(root, "absent-target"), dangling); err != nil {
			t.Skipf("file symlinks unavailable: %v", err)
		}
		checkCases(t, []resolutionCase{
			{name: "directory alias", path: filepath.Join(aliasDir, "session.jsonl"), want: filepath.Join(resolvedRoot, "real", "session.jsonl")},
			{name: "missing below directory alias", path: filepath.Join(aliasDir, "new", "missing.jsonl"), want: filepath.Join(resolvedRoot, "real", "new", "missing.jsonl")},
			{name: "dangling symlink", path: dangling, wantError: true},
			{name: "below dangling symlink", path: filepath.Join(dangling, "missing.jsonl"), wantError: true},
		})
	})
}
