package statistic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestMinimum(t *testing.T) {
	minimum := sampleFrame(7, 1, 5, 3)
	Minimum(&minimum)

	if minimum.Err != nil {
		t.Fatal(minimum.Err)
	}

	if got := minimum.MustGet(SymbolResult); got != 1 {
		t.Fatalf("minimum=%v; want 1", got)
	}

	empty := types.Frame{}
	Minimum(&empty)

	if empty.MustGet(SymbolReady) != 0 || empty.MustGet(SymbolResult) != 0 {
		t.Fatal("empty minimum should be provisional")
	}
}

func TestMinOf(t *testing.T) {
	symA := types.MustIntern("metric/a")
	symB := types.MustIntern("metric/b")

	minPrimitive := MinOf(symA, symB)

	input := types.Frame{}
	input.Put(symA, 4.2)
	input.Put(symB, -0.8)

	minPrimitive(&input)

	if input.Err != nil {
		t.Fatal(input.Err)
	}

	if got := input.MustGet(SymbolResult); got != -0.8 {
		t.Fatalf("MinOf result=%v; want -0.8", got)
	}
}

func TestTouch(t *testing.T) {
	upper := types.MustIntern("book/high")
	lower := types.MustIntern("book/low")
	sec := types.MustIntern("book/sec")
	nsec := types.MustIntern("book/nsec")

	touch := Touch("book/touch", upper, lower, sec, nsec)

	frame := types.Frame{}

	step := func(high, low, seconds, nanoseconds float64) {
		frame.Put(upper, high)
		frame.Put(lower, low)
		frame.Put(sec, seconds)
		frame.Put(nsec, nanoseconds)
		touch(&frame)

		if frame.Err != nil {
			t.Fatal(frame.Err)
		}
	}

	// First observation seeds both sides: center is (high+low)/2.
	step(100, 98, 1, 0)

	if center, _ := TouchCenter(&frame, "book/touch"); center != 99 {
		t.Fatalf("center after first step=%v; want 99", center)
	}

	// A higher high widens the upper extreme upward; the center follows.
	step(102, 98, 2, 0)

	if center, _ := TouchCenter(&frame, "book/touch"); center != 100 {
		t.Fatalf("center after higher high=%v; want 100", center)
	}

	// A lower low widens the lower extreme downward.
	step(102, 96, 3, 0)

	if center, _ := TouchCenter(&frame, "book/touch"); center != 99 {
		t.Fatalf("center after lower low=%v; want 99", center)
	}

	if !TouchReady(&frame, "book/touch") {
		t.Fatal("touch should be ready")
	}
}

func TestTouchRequiresInputs(t *testing.T) {
	upper := types.MustIntern("book/high")
	lower := types.MustIntern("book/low")
	sec := types.MustIntern("book/sec")
	nsec := types.MustIntern("book/nsec")

	touch := Touch("book/touch", upper, lower, sec, nsec)

	frame := types.Frame{}
	frame.Put(upper, 1)
	touch(&frame)

	if frame.Err == nil {
		t.Fatal("touch should reject a frame missing its required slots")
	}
}
