package temporal

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestClockAndDuration(t *testing.T) {
	clock := types.Frame{}
	clock.Put(SymbolAge, 1)
	clock.Put(SymbolSpan, 2)
	Clock(&clock)

	if clock.Err != nil {
		t.Fatal(clock.Err)
	}

	if got := clock.MustGet(SymbolProgress); got != 0.5 {
		t.Fatalf("progress=%v; want 0.5", got)
	}

	duration := types.Frame{}
	duration.Put(SymbolCurrentSec, 101)
	duration.Put(SymbolCurrentNsec, 100_000_000)
	duration.Put(SymbolPreviousSec, 100)
	duration.Put(SymbolPreviousNsec, 900_000_000)
	Duration(&duration)

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
