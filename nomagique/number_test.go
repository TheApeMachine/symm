package nomagique

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
A nil input means "I have nothing to give you": the first stage answers with
its own seed, which becomes the fold's base. No special case is needed.
*/
func TestNumberFoldsFromTheFirstSeed(t *testing.T) {
	out := Number(
		arithmetic.NewAdd(core.From(1.0)),
		arithmetic.NewAdd(core.From(2.0)),
		arithmetic.NewAdd(core.From(3.0)),
		arithmetic.NewAdd(core.From(4.0)),
	)

	if actual := core.To[float64](out); actual != 10 {
		t.Fatalf("got %v want 10", actual)
	}
}
