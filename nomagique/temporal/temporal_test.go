package temporal

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestClockAndDuration(t *testing.T) {
	clockInput := types.Frame{}
	clockInput.Put(SymbolAge, 1)
	clockInput.Put(SymbolSpan, 2)
	clock := Clock(clockInput)

	if clock.Err != nil {
		t.Fatal(clock.Err)
	}

	if got := clock.MustGet(SymbolProgress); got != 0.5 {
		t.Fatalf("progress=%v; want 0.5", got)
	}

	durationInput := types.Frame{}
	durationInput.Put(SymbolCurrentSec, 101)
	durationInput.Put(SymbolCurrentNsec, 100_000_000)
	durationInput.Put(SymbolPreviousSec, 100)
	durationInput.Put(SymbolPreviousNsec, 900_000_000)
	duration := Duration(durationInput)

	if duration.Err != nil {
		t.Fatal(duration.Err)
	}

	if got := duration.MustGet(SymbolDelta); math.Abs(got-0.2) > 1e-12 {
		t.Fatalf("duration=%v; want 0.2", got)
	}
}

func TestIntervalRetainsPreviousTimestamp(t *testing.T) {
	stream := types.NewStream(Interval, types.Frame{})
	input := types.Frame{}
	input.Put(SymbolTimestamp, 100)
	first := stream.Step(input)

	if first.Err != nil {
		t.Fatal(first.Err)
	}

	if first.MustGet(SymbolDelta) != 0 {
		t.Fatal("first interval should be zero")
	}

	input.Put(SymbolTimestamp, 100.5)
	second := stream.Step(input)

	if second.Err != nil {
		t.Fatal(second.Err)
	}

	if second.MustGet(SymbolDelta) != 0.5 {
		t.Fatalf("second interval=%v; want 0.5", second.MustGet(SymbolDelta))
	}
}
