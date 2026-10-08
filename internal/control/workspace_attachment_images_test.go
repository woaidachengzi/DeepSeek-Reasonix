package control

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControllerAttachmentImagesUseWorkspaceNeverForeignCWD(t *testing.T) {
	workspace, foreign := t.TempDir(), t.TempDir()
	t.Chdir(foreign)
	writeVisionTestConfig(t, workspace)
	ref, err := SaveImageBytesInRoot(workspace, "image/png", mustBase64(t, tinyPNG))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(foreign, ".reasonix", "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	// A same-spelling foreign file must neither override the owned file nor
	// become a fallback after the owned attachment is missing.
	decoy := append(mustBase64(t, tinyPNG), []byte("foreign valid PNG decoy")...)
	if err := os.WriteFile(filepath.Join(foreign, filepath.FromSlash(ref)), decoy, 0600); err != nil {
		t.Fatal(err)
	}
	c := &Controller{workspaceRoot: workspace, modelRef: "custom/vision-pro"}
	urls := c.inputImages("look at @" + ref)
	if len(urls) != 1 || urls[0] != "data:image/png;base64,"+tinyPNG {
		t.Fatal("owned attachment image was not delivered")
	}
	if err := os.Remove(filepath.Join(workspace, filepath.FromSlash(ref))); err != nil {
		t.Fatal(err)
	}
	if len(c.resolveInputImageCandidates("look at @"+ref)) != 0 {
		t.Fatal("missing owned image fell back to process cwd")
	}
}

func TestWorkspaceAttachmentImageReadIsConfinedPureAndRejectsSymlinks(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	ref := ".reasonix/attachments/owned.png"
	if _, err := visionImageDataURLInRoot(workspace, ref); err == nil {
		t.Fatal("missing attachment accepted")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".reasonix")); !os.IsNotExist(err) {
		t.Fatal("attachment read created directories")
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".reasonix", "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(workspace, filepath.FromSlash(ref))
	foreign := filepath.Join(outside, "outside.png")
	if err := os.WriteFile(foreign, mustBase64(t, tinyPNG), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, owned); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{ref, "../../outside.png", ".reasonix/attachments/../outside.png", foreign} {
		if _, err := visionImageDataURLInRoot(workspace, input); err == nil {
			t.Fatal("unsafe attachment accepted")
		}
	}
	if err := os.Remove(owned); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(owned, mustBase64(t, tinyPNG), 0600); err != nil {
		t.Fatal(err)
	}
	// Internal links are rejected too, preserving the existing attachment rule.
	alias := ".reasonix/attachments/alias.png"
	if err := os.Symlink("owned.png", filepath.Join(workspace, filepath.FromSlash(alias))); err != nil {
		t.Fatal(err)
	}
	if _, err := visionImageDataURLInRoot(workspace, alias); err == nil {
		t.Fatal("internal attachment symlink accepted")
	}
	if err := os.Symlink(outside, filepath.Join(workspace, ".reasonix", "attachments", "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := visionImageDataURLInRoot(workspace, ".reasonix/attachments/nested/outside.png"); err == nil {
		t.Fatal("symlinked attachment directory accepted")
	}
	value, err := visionImageDataURLInRoot(workspace, ref)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "data:image/png;base64,"))
	if err != nil || string(payload) != string(mustBase64(t, tinyPNG)) {
		t.Fatal("owned attachment bytes changed")
	}
}
