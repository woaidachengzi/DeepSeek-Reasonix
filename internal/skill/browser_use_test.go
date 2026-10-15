package skill_test

import (
	"strings"
	"testing"

	"reasonix/internal/skill"
)

func TestBrowserUseBuiltin(t *testing.T) {
	store := skill.New(skill.Options{HomeDir: t.TempDir()})
	sk, ok := store.Read("browser-use")
	if !ok || sk.Scope != skill.ScopeBuiltin || sk.RunAs != skill.RunInline {
		t.Fatalf("browser-use must be discoverable and execute inline: %+v, found=%v", sk, ok)
	}
	if sk.Description == "" || sk.Body == "" {
		t.Fatal("browser-use needs a discovery description and executable instructions")
	}
	index := skill.IndexBlock(store.List())
	if !strings.Contains(index, "- browser-use") || strings.Contains(index, sk.Body) {
		t.Fatal("only the skill index entry should enter the system prompt")
	}
	disabled := skill.New(skill.Options{HomeDir: t.TempDir(), DisabledNames: []string{"browser-use"}})
	if _, ok := disabled.Read("browser-use"); ok {
		t.Fatal("browser-use must respect the user's disabled skills")
	}
}
