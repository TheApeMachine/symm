package broker

import "testing"

func TestExitPathForStopFallsBackToEntry(t *testing.T) {
	position := &Position{
		EntryPath: "R01/R02",
		ExitPath:  "",
	}
	if got := position.exitPathForStop(); got != "R01/R02" {
		t.Fatalf("expected entry path, got %q", got)
	}
}

func TestExitPathForStopPrefersExitSpell(t *testing.T) {
	position := &Position{
		EntryPath: "R01/R02",
		HoldPath:  "R01/R02/R03",
		ExitPath:  "R04/R05",
	}
	if got := position.exitPathForStop(); got != "R04/R05" {
		t.Fatalf("expected exit spell, got %q", got)
	}
}
