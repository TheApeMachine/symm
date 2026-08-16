package transport

import (
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func TestWindowRetainsBoundedRing(t *testing.T) {
	stream := nomagique.NewStream(Window, nomagique.Frame{})
	input := nomagique.Frame{}
	input.Put(SymbolCapacity, 3)

	for _, sample := range []float64{1, 2, 3, 4} {
		input.Put(nomagique.SampleValue, sample)

		if _, err := stream.Step(input); err != nil {
			t.Fatal(err)
		}
	}

	state := stream.Project()

	if state.MustGet(nomagique.SampleCount) != 3 || state.MustGet(nomagique.SampleHead) != 1 {
		t.Fatalf(
			"count=%v head=%v; want 3 and 1",
			state.MustGet(nomagique.SampleCount),
			state.MustGet(nomagique.SampleHead),
		)
	}

	want := []float64{4, 2, 3}

	for index, expected := range want {
		if got := state.MustGet(nomagique.MustSampleSymbol(index)); got != expected {
			t.Fatalf("sample/%d=%v; want %v", index, got, expected)
		}
	}
}
