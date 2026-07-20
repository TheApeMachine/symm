package strategy

import (
	"testing"

	"github.com/theapemachine/symm/types"
)

/*
TestAdmitRanksByUtilityWithFreeSlots picks the highest-utility challengers when
capacity is free — not forecast order.
*/
func TestAdmitRanksByUtilityWithFreeSlots(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Slots(1, 0, 1000)
	thesis := types.NewThesis(nil, nil)
	low := decideForecast("LOW/USD", 0.04, 0.01)
	high := decideForecast("HIGH/USD", 0.10, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, low, high)
	thesis.Cognition.Store("LOW/USD", buyCognition("LOW/USD"))
	thesis.Cognition.Store("HIGH/USD", buyCognition("HIGH/USD"))

	planner.Decide(thesis)

	entered := map[string]bool{}

	for _, decision := range thesis.Decisions {
		if decision.Action == "enter" {
			entered[decision.Symbol] = true
		}
	}

	if !entered["HIGH/USD"] {
		t.Fatalf("highest utility must take the free slot: decisions=%+v", thesis.Decisions)
	}

	if entered["LOW/USD"] {
		t.Fatal("lower utility must not consume the only free slot")
	}
}

/*
TestDecideRotatesWhenChallengerClearsWeakest proves a full book displaces the
weakest incumbent when rotate surplus is positive.
*/
func TestDecideRotatesWhenChallengerClearsWeakest(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)
	weakLot := decideFixture.Holding("WEAK/USD", 100, 1)
	keepLot := decideFixture.Holding("KEEP/USD", 100, 1)
	thesis.Holdings.Store("WEAK/USD", weakLot)
	thesis.Holdings.Store("KEEP/USD", keepLot)

	weak := decideForecast("WEAK/USD", 0.02, 0.01) // hold margin 0.01
	keep := decideForecast("KEEP/USD", 0.08, 0.01) // hold margin 0.07
	challenger := decideForecast("NEXT/USD", 0.12, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, weak, keep, challenger)
	thesis.Cognition.Store("NEXT/USD", buyCognition("NEXT/USD"))
	thesis.Manifold.Store("NEXT/USD", readyBasin("NEXT/USD", 0.2))

	planner = decideFixture.Slots(2, 0, 1000)
	decideFixture.Seed(planner, weakLot)
	decideFixture.Seed(planner, keepLot)
	planner.Decide(thesis)

	exited := false
	entered := false

	for _, decision := range thesis.Decisions {
		if decision.Action == "exit" && decision.Symbol == "WEAK/USD" &&
			decision.Cause == "rotation" {
			exited = true
		}

		if decision.Action == "enter" && decision.Symbol == "NEXT/USD" &&
			decision.Cause == "rotation" {
			entered = true

			if decision.Alternatives["rotate_surplus"] <= 0 {
				t.Fatalf("rotate surplus must be positive: %+v", decision)
			}
		}

		if decision.Action == "exit" && decision.Symbol == "KEEP/USD" {
			t.Fatal("stronger incumbent must not be displaced")
		}
	}

	if !exited || !entered {
		t.Fatalf("want rotate WEAK→NEXT, exited=%v entered=%v", exited, entered)
	}
}

/*
TestDecideWaitsWhenRotateSurplusNonPositive keeps incumbents when the
challenger does not clear hold utility plus exit cost.
*/
func TestDecideWaitsWhenRotateSurplusNonPositive(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)
	holdLot := decideFixture.Holding("HOLD/USD", 100, 1)
	thesis.Holdings.Store("HOLD/USD", holdLot)

	// hold margin 0.09; challenger utility after friction ~0.04 - costs < hold+exit
	hold := decideForecast("HOLD/USD", 0.10, 0.01)
	challenger := decideForecast("MEH/USD", 0.04, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, hold, challenger)
	thesis.Cognition.Store("MEH/USD", buyCognition("MEH/USD"))

	planner = decideFixture.Slots(1, 0, 1000)
	decideFixture.Seed(planner, holdLot)
	planner.Decide(thesis)

	sawWait := false

	for _, decision := range thesis.Decisions {
		if decision.Action == "exit" {
			t.Fatalf("must wait, not rotate: %+v", decision)
		}

		if decision.Action == "enter" && decision.Symbol == "MEH/USD" {
			t.Fatalf("weak challenger must not enter: %+v", decision)
		}

		if decision.Symbol == "MEH/USD" && decision.Cause == "rotate_wait" {
			sawWait = true
		}
	}

	if !sawWait {
		t.Fatal("expected rotate_wait nothing decision for MEH/USD")
	}
}

/*
TestRotateSurplusMatchesContract locks the Δ definition used by admit.
*/
func TestRotateSurplusMatchesContract(t *testing.T) {
	t.Parallel()

	if got := NewRotate().Surplus(0.08, 0.03, 0.01); got != 0.04 {
		t.Fatalf("want 0.04, got %v", got)
	}

	if got := NewRotate().Surplus(0.03, 0.03, 0.01); got >= 0 {
		t.Fatalf("want negative surplus, got %v", got)
	}
}

/*
BenchmarkDecideRotate measures a full-book rotate evaluation path.
*/
func BenchmarkDecideRotate(b *testing.B) {
	planner := decideFixture.Planner()

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		thesis := types.NewThesis(nil, nil)
		thesis.Holdings.Store("WEAK/USD", decideFixture.Holding("WEAK/USD", 100, 1))
		thesis.Holdings.Store("KEEP/USD", decideFixture.Holding("KEEP/USD", 100, 1))
		thesis.Forecasts = append(thesis.Forecasts,
			decideForecast("WEAK/USD", 0.02, 0.01),
			decideForecast("KEEP/USD", 0.08, 0.01),
			decideForecast("NEXT/USD", 0.12, 0.01),
		)
		thesis.Cognition.Store("NEXT/USD", buyCognition("NEXT/USD"))
		thesis.Manifold.Store(
			"NEXT/USD", readyBasin("NEXT/USD", 0.2),
		)
		_ = planner.Decide(thesis)
	}
}
