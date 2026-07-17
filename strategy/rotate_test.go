package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
TestAdmitRanksByUtilityWithFreeSlots picks the highest-utility challengers when
capacity is free — not forecast order.
*/
func TestAdmitRanksByUtilityWithFreeSlots(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	low := decideForecast("LOW/USD", 0.04, 0.01)
	high := decideForecast("HIGH/USD", 0.10, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, low, high)
	thesis.Cognition.Store("LOW/USD", buyCognition("LOW/USD"))
	thesis.Cognition.Store("HIGH/USD", buyCognition("HIGH/USD"))

	fees := map[string]float64{"LOW/USD": 0.001, "HIGH/USD": 0.001}
	planner.Decide(thesis, fees, 1000, 1, 0)

	entered := map[string]bool{}

	for _, decision := range thesis.Decisions {
		if decision.Action == "enter" {
			entered[decision.Symbol] = true
		}
	}

	if !entered["HIGH/USD"] {
		t.Fatal("highest utility must take the free slot")
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

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	thesis.Holdings.Store("WEAK/USD", types.Holding{
		Symbol: "WEAK/USD",
		Qty:    decimal.NewFromFloat64(100),
		Mark:   decimal.NewFromFloat64(1),
	})
	thesis.Holdings.Store("KEEP/USD", types.Holding{
		Symbol: "KEEP/USD",
		Qty:    decimal.NewFromFloat64(100),
		Mark:   decimal.NewFromFloat64(1),
	})

	weak := decideForecast("WEAK/USD", 0.02, 0.01) // hold margin 0.01
	keep := decideForecast("KEEP/USD", 0.08, 0.01) // hold margin 0.07
	challenger := decideForecast("NEXT/USD", 0.12, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, weak, keep, challenger)
	thesis.Cognition.Store("NEXT/USD", buyCognition("NEXT/USD"))
	thesis.Manifold.Store("NEXT/USD", readyBasin("NEXT/USD", 0.2))

	fees := map[string]float64{
		"WEAK/USD": 0.001,
		"KEEP/USD": 0.001,
		"NEXT/USD": 0.001,
	}
	planner.Decide(thesis, fees, 0, 2, 0)

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

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	thesis.Holdings.Store("HOLD/USD", types.Holding{
		Symbol: "KEEP/USD",
		Qty:    decimal.NewFromFloat64(100),
		Mark:   decimal.NewFromFloat64(1),
	})
	thesis.Holdings.Store("MEH/USD", types.Holding{
		Symbol: "MEH/USD",
		Qty:    decimal.NewFromFloat64(100),
		Mark:   decimal.NewFromFloat64(1),
	})

	// hold margin 0.09; challenger utility after friction ~0.04 - costs < hold+exit
	hold := decideForecast("HOLD/USD", 0.10, 0.01)
	challenger := decideForecast("MEH/USD", 0.04, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, hold, challenger)
	thesis.Cognition.Store("MEH/USD", buyCognition("MEH/USD"))

	fees := map[string]float64{"HOLD/USD": 0.001, "MEH/USD": 0.001}
	planner.Decide(thesis, fees, 0, 1, 0)

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

	if got := rotateSurplus(0.08, 0.03, 0.01); got != 0.04 {
		t.Fatalf("want 0.04, got %v", got)
	}

	if got := rotateSurplus(0.03, 0.03, 0.01); got >= 0 {
		t.Fatalf("want negative surplus, got %v", got)
	}
}

/*
BenchmarkDecideRotate measures a full-book rotate evaluation path.
*/
func BenchmarkDecideRotate(b *testing.B) {
	planner := NewPlanner(context.Background(), nil, nil, nil)
	fees := map[string]float64{
		"WEAK/USD": 0.001,
		"KEEP/USD": 0.001,
		"NEXT/USD": 0.001,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		thesis := types.NewThesis(nil, nil)
		thesis.Holdings.Store("WEAK/USD", types.Holding{
			Symbol: "WEAK/USD",
			Qty:    decimal.NewFromFloat64(100),
			Mark:   decimal.NewFromFloat64(1),
		})
		thesis.Holdings.Store("KEEP/USD", types.Holding{
			Symbol: "KEEP/USD",
			Qty:    decimal.NewFromFloat64(100),
			Mark:   decimal.NewFromFloat64(1),
		})
		thesis.Forecasts = append(thesis.Forecasts,
			decideForecast("WEAK/USD", 0.02, 0.01),
			decideForecast("KEEP/USD", 0.08, 0.01),
			decideForecast("NEXT/USD", 0.12, 0.01),
		)
		thesis.Cognition.Store("NEXT/USD", buyCognition("NEXT/USD"))
		thesis.Manifold.Store(
			"NEXT/USD", readyBasin("NEXT/USD", 0.2),
		)
		_ = planner.Decide(thesis, fees, 0, 2, 0)
	}
}
