package parallel

import (
	"testing"

	nmstore "github.com/theapemachine/symm/nomagique/store"
)

func TestRegimeLabel(t *testing.T) {
	regions := []*nmstore.Region{
		{Key: 6, Brightness: 0.2},
		{Key: 7, Brightness: 0.9},
	}

	if got := regimeLabel(regions); got != "explosive" {
		t.Fatalf("regimeLabel() = %q, want explosive", got)
	}

	if regimeLabel(nil) != "unknown" {
		t.Fatal("expected unknown for empty regions")
	}
}
