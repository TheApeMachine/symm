package strategy

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
TestAdmitKeepsReservedForOpportunity ensures that when normal slots are full,
only reserved entries (positive margin and cognitive lead) may take overflow.
*/
func TestAdmitKeepsReservedForOpportunity(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)

	aaa := decideFixture.Holding("AAA/USD", 1, 1)
	bbb := decideFixture.Holding("BBB/USD", 1, 1)
	thesis.Holdings.Store("AAA/USD", aaa)
	thesis.Holdings.Store("BBB/USD", bbb)
	decideFixture.Seed(planner, aaa)
	decideFixture.Seed(planner, bbb)

	boring := decideForecast("CCC/USD", 0.01, 0.02) // negative margin
	pump := decideForecast("OXT/USD", 0.08, 0.02)   // positive margin

	thesis.Forecasts = append(thesis.Forecasts, boring, pump)
	thesis.Cognition.Store("CCC/USD", buyCognition("CCC/USD"))
	thesis.Cognition.Store("OXT/USD", buyCognition("OXT/USD"))
	thesis.Manifold.Store("CCC/USD", readyBasin("CCC/USD", 0.2))
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	planner.Decide(thesis)

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

	t.Run("cognitive_no_confidence", func(t *testing.T) {
		planner := decideFixture.Planner()
		thesis := types.NewThesis(nil, nil)
		forecast := decideForecast("ZZZ/USD", 0.05, 0.01)
		thesis.Forecasts = append(thesis.Forecasts, forecast)
		thesis.Cognition.Store("ZZZ/USD", types.Cognition{
			Source: "dmt", Symbol: "ZZZ/USD", At: time.Unix(1, 0).UTC(),
			Winner: "buy", Ready: true, Confidence: 0, Ambiguous: false,
		})

		planner.Decide(thesis)

		for _, decision := range thesis.Decisions {
			if decision.Symbol != "ZZZ/USD" {
				continue
			}

			if decision.Action == "enter" {
				t.Fatalf("zero cognitive confidence must not enter: %+v", decision)
			}

			if decision.Cause != "cognitive_no_confidence" {
				t.Fatalf(
					"want cognitive_no_confidence, got %s (%s)",
					decision.Cause, decision.Reason,
				)
			}

			return
		}

		t.Fatal("expected a nothing decision for ZZZ/USD")
	})

	t.Run("forecast_no_confidence", func(t *testing.T) {
		planner := decideFixture.Planner()
		thesis := types.NewThesis(nil, nil)
		forecast := decideForecast("ZZZ/USD", 0.05, 0.01)
		forecast.Confidence = 0
		thesis.Forecasts = append(thesis.Forecasts, forecast)
		thesis.Cognition.Store("ZZZ/USD", buyCognition("ZZZ/USD"))

		planner.Decide(thesis)

		for _, decision := range thesis.Decisions {
			if decision.Symbol != "ZZZ/USD" {
				continue
			}

			if decision.Action == "enter" {
				t.Fatalf("zero forecast confidence must not enter: %+v", decision)
			}

			if decision.Cause != "forecast_no_confidence" {
				t.Fatalf(
					"want forecast_no_confidence, got %s (%s)",
					decision.Cause, decision.Reason,
				)
			}

			return
		}

		t.Fatal("expected a nothing decision for ZZZ/USD")
	})
}

/*
TestDecideRejectsWhenUncertaintyConsumesUtility ensures a raw positive return
that is smaller than uncertainty fails the single utility gate after friction.
*/
func TestDecideRejectsWhenUncertaintyConsumesUtility(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)
	forecast := decideForecast("WEAK/USD", 0.01, 0.05)
	thesis.Forecasts = append(thesis.Forecasts, forecast)
	thesis.Cognition.Store("WEAK/USD", buyCognition("WEAK/USD"))

	planner.Decide(thesis)

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

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)
	forecast := decideForecast("COLD/USD", 0.08, 0.02)
	thesis.Forecasts = append(thesis.Forecasts, forecast)
	thesis.Cognition.Store("COLD/USD", types.Cognition{
		Source: "dmt", Symbol: "COLD/USD", At: time.Unix(1, 0).UTC(),
		Winner: "buy", Ready: true, Confidence: 0.01, Ambiguous: false,
	})

	planner.Decide(thesis)

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
TestDecideCapsProposedNotionalByAvailableCash keeps Decide from publishing a
lot size; Allocator owns ProposedNotional, so rejected challengers stay at zero.
*/
func TestDecideCapsProposedNotionalByAvailableCash(t *testing.T) {
	t.Parallel()

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)
	fat := decideFixture.Holding("FAT/USD", 3400, 1)
	thesis.Holdings.Store("FAT/USD", fat)

	hold := decideForecast("FAT/USD", 0.10, 0.01)
	challenger := decideForecast("XRP/USD", 0.04, 0.01)
	challenger.BuyCapacity = 10_000
	thesis.Forecasts = append(thesis.Forecasts, hold, challenger)
	thesis.Cognition.Store("XRP/USD", buyCognition("XRP/USD"))

	available := 5.73
	planner = decideFixture.Slots(1, 0, available)
	decideFixture.Seed(planner, fat)
	planner.Decide(thesis)

	for _, decision := range thesis.Decisions {
		if decision.Symbol != "XRP/USD" {
			continue
		}

		if decision.AvailableCapital == nil ||
			decision.AvailableCapital.Float64() != available {
			t.Fatalf("available capital: got %v want %v", decision.AvailableCapital, available)
		}

		// Decide must not invent a lot; Allocator owns ProposedNotional.
		if decision.ProposedNotional != nil && decision.ProposedNotional.Sign() > 0 {
			t.Fatalf("Decide must leave notional unsized, got %v", decision.ProposedNotional)
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

	planner := decideFixture.Planner()
	thesis := types.NewThesis(nil, nil)
	weakLot := decideFixture.Holding("WEAK/USD", 100, 1)
	keepLot := decideFixture.Holding("KEEP/USD", 100, 1)
	thesis.Holdings.Store("WEAK/USD", weakLot)
	thesis.Holdings.Store("KEEP/USD", keepLot)

	weak := decideForecast("WEAK/USD", 0.02, 0.01)
	keep := decideForecast("KEEP/USD", 0.08, 0.01)
	challenger := decideForecast("NEXT/USD", 0.12, 0.01)
	thesis.Forecasts = append(thesis.Forecasts, weak, keep, challenger)
	thesis.Cognition.Store("NEXT/USD", buyCognition("NEXT/USD"))
	thesis.Manifold.Store("NEXT/USD", readyBasin("NEXT/USD", 0.2))

	planner = decideFixture.Slots(2, 0, 1000)
	decideFixture.Seed(planner, weakLot)
	decideFixture.Seed(planner, keepLot)
	planner.Decide(thesis)

	for _, decision := range thesis.Decisions {
		if decision.Action != "enter" || decision.Symbol != "NEXT/USD" {
			continue
		}

		if decision.Cause != "rotation" {
			t.Fatalf("want rotation enter, got %+v", decision)
		}

		if decision.ProposedNotional == nil ||
			decision.ProposedNotional.Sign() <= 0 {
			t.Fatalf(
				"rotate enter must be sized by Allocator, got %v",
				decision.ProposedNotional,
			)
		}

		return
	}

	t.Fatal("expected rotated enter for NEXT/USD")
}

func decideForecast(symbol string, expected, uncertainty float64) types.Forecasts {
	return types.Forecasts{
		Source:                     "resonance+causal",
		Symbol:                     symbol,
		At:                         time.Unix(1, 0).UTC(),
		ObservedInterval:           time.Second,
		SourceEpoch:                1,
		HorizonEvents:              1,
		ExpiresEpoch:               2,
		Target:                     "next_l3_epoch_mid_log_return",
		ModelVersion:               "resonance_return_head_v2_rls",
		Ready:                      true,
		Calibrated:                 true,
		FrictionReady:              true,
		CalibrationSamples:         8,
		IncrementalSkillLowerBound: 0.0001,
		ExpectedReturn:             expected,
		ReferencePrice:             1,
		BuyCapacity:                10_000,
		SellCapacity:               10_000,
		ExpectedFees:               0.001,
		ExpectedSpread:             0.001,
		ExpectedImpact:             0,
		ExpectedAdverseSelection:   0,
		Uncertainty:                uncertainty,
		Confidence:                 0.5,
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

func findHolding(thesis *types.Thesis, symbol string) (*types.Holding, bool) {
	var found *types.Holding
	ok := false

	thesis.Holdings.Range(func(_, value any) bool {
		holding, typed := value.(*types.Holding)

		if !typed || holding == nil || holding.Symbol != symbol {
			return true
		}

		found = holding
		ok = true

		return false
	})

	return found, ok
}
