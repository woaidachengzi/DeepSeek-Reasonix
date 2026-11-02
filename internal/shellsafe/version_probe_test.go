package shellsafe

import "testing"

func TestRuntimeProbesRequireExactVersionInvocation(t *testing.T) {
	for _, command := range []string{"node -v", "node --version", "python -V", "python --version", "python3 -V", "python3 --version"} {
		if effect := ClassifyBash(command); !effect.IsPermissionReader() || effect.AnyMutation() {
			t.Errorf("version probe %q: %+v", command, effect)
		}
		if _, _, ok := CommandIsReadOnly(command); !ok {
			t.Errorf("legacy version probe %q refused", command)
		}
	}
	for _, command := range []string{
		"python -v", "python3 -v script.py", "python3 -vv script.py",
		"python3 -V script.py", "python3 --version script.py", "python3 --VERSION",
		"node -V", "node -v script.js", "node --version -e 'process.exit(0)'",
		"/usr/bin/python3 -v script.py", "python3.exe -v script.py",
		`echo "$(python3 -v script.py)"`, `cat "$(node -v script.js)"`,
	} {
		if effect := ClassifyBash(command); effect.IsPermissionReader() || !effect.WorkspaceMutation() {
			t.Errorf("code-executing shape %q classified as reader: %+v", command, effect)
		}
		if _, _, ok := CommandIsReadOnly(command); ok {
			t.Errorf("legacy reader accepted %q", command)
		}
	}
}
