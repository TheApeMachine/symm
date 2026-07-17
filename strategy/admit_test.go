package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
TestAdmitKeepsReservedForOpportunity ensures that when normal slots are full,
only reserved entries (positive margin and cognitive lead) may take overflow.
*/
func TestAdmitKeepsReservedForOpportunity(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)

	thesis.Holdings.Store("AAA/USD", types.Holding{
		Symbol: "AAA/USD",
		Qty:    decimal.NewFromFloat64(1),
		Mark:   decimal.NewFromFloat64(1),
	})
	thesis.Holdings.Store("BBB/USD", types.Holding{
		Symbol: "BBB/USD",
		Qty:    decimal.NewFromFloat64(1),
		Mark:   decimal.NewFromFloat64(1),
	})

	boring := decideForecast("CCC/USD", 0.01, 0.02) // negative margin
	pump := decideForecast("OXT/USD", 0.08, 0.02)   // positive margin

	thesis.Forecasts = append(thesis.Forecasts, boring, pump)
	thesis.Cognition.Store("CCC/USD", buyCognition("CCC/USD"))
	thesis.Cognition.Store("OXT/USD", buyCognition("OXT/USD"))
	thesis.Manifold.Store("CCC/USD", readyBasin("CCC/USD", 0.2))
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	fees := map[string]float64{"CCC/USD": 0.001, "OXT/USD": 0.001}
	planner.Decide(thesis, fees, 1000, 2, 2)

	entered := map[string]bool{}
	rejected := map[string]string{}

	for _, decision := range thesis.Decisions {
		if decision.Action == "enter" {
			entered[decision.Symbol] = true
		}

		if decision.Action == "nothing" &&
			(decision.Symbol == "CCC/USD" || decision.Symbol == "OXT/USD") {
			rejected[decision.Symbol] = decision.Reason
		}
	}

	if entered["CCC/USD"] {
		t.Fatal("non-opportunity must not consume reserved overflow")
	}

	if !entered["OXT/USD"] {
		t.Fatalf("opportunity must take reserved overflow; rejected=%v", rejected)
	}

	holding, ok := findHolding(thesis, "OXT/USD")

	if !ok || !holding.IsOpportunity {
		t.Fatal("OXT holding must be flagged IsOpportunity")
	}

	for _, decision := range thesis.Decisions {
		if decision.Symbol != "OXT/USD" || decision.Action != "enter" {
			continue
		}

		if decision.OpportunityMargin <= 0 || decision.CognitiveLead <= 0 {
			t.Fatalf("enter must expose positive margin and lead: %+v", decision)
		}
	}
}

/*
TestDecideRejectsBuyWithoutConfidence ensures Winner=buy alone cannot enter
when cognition or forecast confidence is non-positive.
*/
func TestDecideRejectsBuyWithoutConfidence(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	forecast := decideForecast("ZZZ/USD", 0.05, 0.01)
	forecast.Confidence = 0
	thesis.Forecasts = append(thesis.Forecasts, forecast)
	thesis.Cognition.Store("ZZZ/USD", types.Cognition{
		Source: "dmt", Symbol: "ZZZ/USD", At: time.Unix(1, 0).UTC(),
		Winner: "buy", Ready: true, Confidence: 0, Ambiguous: false,
	})

	planner.Decide(thesis, map[string]float64{"ZZZ/USD": 0.001}, 1000, 2, 2)

	for _, decision := range thesis.Decisions {
		if decision.Symbol != "ZZZ/USD" {
			continue
		}

		if decision.Action == "enter" {
			t.Fatalf("zero-confidence buy must not enter: %+v", decision)
		}

		if decision.Cause != "cognitive_no_confidence" &&
			decision.Cause != "forecast_no_confidence" {
			t.Fatalf("want no-confidence cause, got %s (%s)", decision.Cause, decision.Reason)
		}

		return
	}

	t.Fatal("expected a nothing decision for ZZZ/USD")
}

/*
TestDecideRejectsWhenUncertaintyConsumesUtility ensures a raw positive return
that is smaller than uncertainty fails the single utility gate after friction.
*/
func TestDecideRejectsWhenUncertaintyConsumesUtility(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	forecast := decideForecast("WEAK/USD", 0.01, 0.05)
	thesis.Forecasts = append(thesis.Forecasts, forecast)
	thesis.Cognition.Store("WEAK/USD", buyCognition("WEAK/USD"))

	planner.Decide(thesis, map[string]float64{"WEAK/USD": 0.001}, 1000, 2, 2)

	for _, decision := range thesis.Decisions {
		if decision.Symbol != "WEAK/USD" {
			continue
		}

		if decision.Action == "enter" {
			t.Fatalf("uncertainty-consumed utility must not enter: %+v", decision)
		}

		if decision.Cause != "infeasible" {
			t.Fatalf("want infeasible, got %s (%s)", decision.Cause, decision.Reason)
		}

		if decision.Utility >= 0 {
			t.Fatalf("want negative utility, got %v", decision.Utility)
		}

		return
	}

	t.Fatal("expected a nothing decision for WEAK/USD")
}

/*
TestDecideRejectsWeakCognitiveConfidence blocks ε-confidence buys that still
clear Winner=buy and positive utility/margin.
*/
func TestDecideRejectsWeakCognitiveConfidence(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	forecast := decideForecast("COLD/USD", 0.08, 0.02)
	thesis.Forecasts = append(thesis.Forecasts, forecast)
	thesis.Cognition.Store("COLD/USD", types.Cognition{
		Source: "dmt", Symbol: "COLD/USD", At: time.Unix(1, 0).UTC(),
		Winner: "buy", Ready: true, Confidence: 0.01, Ambiguous: false,
	})

	planner.Decide(thesis, map[string]float64{"COLD/USD": 0.001}, 1000, 2, 2)

	for _, decision := range thesis.Decisions {
		if decision.Symbol != "COLD/USD" {
			continue
		}

		if decision.Action == "enter" {
			t.Fatalf("ε-confidence buy must not enter: %+v", decision)
		}

		if decision.Cause != "cognitive_weak" {
			t.Fatalf("want cognitive_weak, got %s (%s)", decision.Cause, decision.Reason)
		}

		return
	}

	t.Fatal("expected a nothing decision for COLD/USD")
}

/*
TestDecideCapsProposedNotionalByAvailableCash keeps rejected full-book
challengers from publishing book-depth or rotate-budget notionals against
cash AvailableCapital (the Fraction column's denominator).
*/
func TestDecideCapsProposedNotionalByAvailableCash(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(context.Background(), nil, nil, nil)
	thesis := types.NewThesis(nil, nil)
	thesis.Holdings.Store("FAT/USD", types.Holding{
		Symbol: "FAT/USD",
		Qty:    decimal.NewFromFloat64(3400),
		Mark:   decimal.NewFromFloat64(1),
	})

	hold := decideForecast("FAT/USD", 0.10, 0.01)
	challenger := decideForecast("XRP/USD", 0.04, 0.01)
	challenger.BuyCapacity = 10_000
	thesis.Forecasts = append(thesis.Forecasts, hold, challenger)
	thesis.Cognition.Store("XRP/USD", buyCognition("XRP/USD"))

	available := 5.73
	fees := map[string]float64{"FAT/USD": 0.001, "XRP/USD": 0.001}
	planner.Decide(thesis, fees, available, 1, 0)

	for _, decision := range thesis.Decisions {
		if decision.Symbol != "XRP/USD" {
			continue
		}

		if decision.AvailableCapital != available {
			t.Fatalf("available capital: got %v want %v", decision.AvailableCapital, available)
		}

		if decision.ProposedNotional > available {
			t.Fatalf(
				"proposed %v exceeds available %v (fraction would be %.0f%%)",
				decision.ProposedNotional,
				available,
				100*decision.ProposedNotional/available,
			)
		}

		return
	}

	t.Fatal("expected a decision for XRP/USD")
}

/*
TestDecideRotateScalesUpToIncumbentNotional ensures a cash-capped proposal
still adopts the freed incumbent notional when rotation wins.
*/
func TestDecideRotateScalesUpToIncumbentNotional(t *testing.T) {
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

	weak := decideForecast("WEAK/USD", 0.02, 0.01)
	keep := decideForecast("KEEP/USD", 0.08, 0.01)
	challenger := decideForecast("NEXT/USD", 0.12, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, weak, keep, challenger)
	thesis.Cognition.Store("NEXT/USD", buyCognition("NEXT/USD"))
	thesis.Manifold.Store("NEXT/USD", readyBasin("NEXT/USD", 0.2))

	fees := map[string]float64{
		"WEAK/USD": 0.001,
		"KEEP/USD": 0.001,
		"NEXT/USD": 0.001,
	}
	planner.Decide(thesis, fees, 5.73, 2, 0)

	for _, decision := range thesis.Decisions {
		if decision.Action != "enter" || decision.Symbol != "NEXT/USD" {
			continue
		}

		if decision.Cause != "rotation" {
			t.Fatalf("want rotation enter, got %+v", decision)
		}

		if decision.ProposedNotional != 100 {
			t.Fatalf(
				"rotate must size to freed incumbent notional, got %v",
				decision.ProposedNotional,
			)
		}

		return
	}

	t.Fatal("expected rotated enter for NEXT/USD")
}

func decideForecast(symbol string, expected, uncertainty float64) types.Forecasts {
	return types.Forecasts{
		Source:                   "resonance+causal",
		Symbol:                   symbol,
		At:                       time.Unix(1, 0).UTC(),
		ObservedInterval:         time.Second,
		SourceEpoch:              1,
		HorizonEvents:            1,
		ExpiresEpoch:             2,
		Target:                   "next_l3_epoch_mid_log_return",
		ModelVersion:             "resonance_return_head_v1",
		Ready:                    true,
		Calibrated:               true,
		FrictionReady:            true,
		CalibrationSamples:       8,
		ExpectedReturn:           expected,
		ReferencePrice:           1,
		BuyCapacity:              10_000,
		SellCapacity:             10_000,
		ExpectedFees:             0.001,
		ExpectedSpread:           0.001,
		ExpectedImpact:           0,
		ExpectedAdverseSelection: 0,
		Uncertainty:              uncertainty,
		Confidence:               0.5,
	}
}

func buyCognition(symbol string) types.Cognition {
	return types.Cognition{
		Source:      "dmt",
		Symbol:      symbol,
		At:          time.Unix(1, 0).UTC(),
		Winner:      "buy",
		Ready:       true,
		Confidence:  0.6,
		Ambiguous:   false,
		Contrast:    0.4,
		EntropyBits: 0.2,
	}
}

func findHolding(thesis *types.Thesis, symbol string) (types.Holding, bool) {
	var found types.Holding
	ok := false

	thesis.Holdings.Range(func(_, value any) bool {
		holding := value.(types.Holding)

		if holding.Symbol != symbol {
			return true
		}

		found = holding
		ok = true

		return false
	})

	return found, ok
}
