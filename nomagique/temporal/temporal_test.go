package temporal

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func TestClockAndDuration(t *testing.T) {
	clockInput := nomagique.Frame{}
	clockInput.Put(SymbolAge, 1)
	clockInput.Put(SymbolSpan, 2)
	_, clock, err := Clock(nomagique.Frame{}, clockInput)

	if err != nil {
		t.Fatal(err)
	}

	if got := clock.MustGet(SymbolProgress); got != 0.5 {
		t.Fatalf("progress=%v; want 0.5", got)
	}

	durationInput := nomagique.Frame{}
	durationInput.Put(SymbolCurrentSec, 101)
	durationInput.Put(SymbolCurrentNsec, 100_000_000)
	durationInput.Put(SymbolPreviousSec, 100)
	durationInput.Put(SymbolPreviousNsec, 900_000_000)
	_, duration, err := Duration(nomagique.Frame{}, durationInput)

	if err != nil {
		t.Fatal(err)
	}

	if got := duration.MustGet(SymbolDelta); math.Abs(got-0.2) > 1e-12 {
		t.Fatalf("duration=%v; want 0.2", got)
	}
}

func TestIntervalRetainsPreviousTimestamp(t *testing.T) {
	stream := nomagique.NewStream(Interval, nomagique.Frame{})
	input := nomagique.Frame{}
	input.Put(SymbolTimestamp, 100)
	first, err := stream.Step(input)

	if err != nil {
		t.Fatal(err)
	}

	if first.MustGet(SymbolDelta) != 0 {
		t.Fatal("first interval should be zero")
	}

	input.Put(SymbolTimestamp, 100.5)
	second, err := stream.Step(input)

	if err != nil {
		t.Fatal(err)
	}

	if second.MustGet(SymbolDelta) != 0.5 {
		t.Fatalf("second interval=%v; want 0.5", second.MustGet(SymbolDelta))
	}
}
