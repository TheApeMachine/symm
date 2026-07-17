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

	thesis.Positions = append(thesis.Positions,
		types.Holding{
			Symbol: "AAA/USD",
			Qty:    decimal.NewFromFloat64(1),
			Mark:   decimal.NewFromFloat64(1),
		},
		types.Holding{
			Symbol: "BBB/USD",
			Qty:    decimal.NewFromFloat64(1),
			Mark:   decimal.NewFromFloat64(1),
		},
	)

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
	for _, holding := range thesis.Positions {
		if holding.Symbol == symbol {
			return holding, true
		}
	}

	return types.Holding{}, false
}
