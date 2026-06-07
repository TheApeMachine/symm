package reasoning

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCanonicalPlaybookMatchesContract(t *testing.T) {
	playbook := CanonicalPlaybook()
	acts := collectActs(playbook)

	if !hasEntryAction(acts) || !hasProtectiveAction(acts) {
		t.Fatalf("canonical playbook missing entry or protective actions")
	}
}

func TestWriteCanonicalPlaybookYAML(t *testing.T) {
	if os.Getenv("WRITE_CANONICAL_PLAYBOOK") == "" {
		t.Skip("set WRITE_CANONICAL_PLAYBOOK=1 to regenerate perspectives.yaml")
	}

	_, file, _, ok := runtime.Caller(0)

	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	path := filepath.Join(filepath.Dir(file), "..", "cfg", "perspectives.yaml")
	raw, err := MarshalThoughts(CanonicalPlaybook(), 2)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
