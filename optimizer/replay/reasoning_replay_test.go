package replay

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

// frictionless: zero fees + slippage, immediate execution, a €100 EUR wallet.
func frictionlessCosts() ReplayCosts {
	return ReplayCosts{
		StartingCapital:        100,
		PositionFraction:       1,
		WalletCurrency:         "EUR",
		ExecutionStressEnabled: false,
	}
}

func ignite(last float64, at time.Time) perspectives.Measurement {
	return perspectives.Measurement{
		Symbol: "BTC/EUR", Category: perspectives.CategoryVerticalIgnition, SNR: 1.5, Last: last, At: at,
	}
}

func notHolding(category perspectives.CategoryType) perspectives.Predicate {
	return perspectives.Predicate{All: []perspectives.Predicate{
		{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: perspectives.ObservationNotHolding},
		{Subject: perspectives.SubjectSignal, Category: category, Unit: perspectives.UnitSNR, Op: perspectives.ComparisonAtLeast, Value: 1.0},
	}}
}

func TestThoughtSimulationScoresAReasoningTree(t *testing.T) {
	Convey("Given a frictionless €100 wallet and a tape", t, func() {
		base := time.Unix(1_700_000_000, 0)

		Convey("A reasoning tree (enter on ignition, settle on reversal) trades and profits", func() {
			thoughts := []perspectives.Thought{
				{When: notHolding(perspectives.CategoryVerticalIgnition), Do: perspectives.Act{Type: perspectives.ActionMarket}},
				{
					When: perspectives.Predicate{All: []perspectives.Predicate{
						{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: perspectives.ObservationHolding},
						{Subject: perspectives.SubjectSignal, Category: perspectives.CategoryActiveReversal, Unit: perspectives.UnitSNR, Op: perspectives.ComparisonAtLeast, Value: 1.0},
					}},
					Do: perspectives.Act{Type: perspectives.ActionSettlePosition},
				},
			}

			rows := []perspectives.Measurement{
				ignite(100, base),
				ignite(102, base.Add(time.Second)),
				ignite(104, base.Add(2*time.Second)),
				{Symbol: "BTC/EUR", Category: perspectives.CategoryActiveReversal, SNR: 1.5, Last: 104, At: base.Add(3 * time.Second)},
			}

			sim := NewThoughtSimulation(context.Background(), thoughts, PrecompileTape(rows), frictionlessCosts())
			result := sim.Result()

			So(result.ClosedTrades, ShouldEqual, 1)
			So(result.Score, ShouldAlmostEqual, 0.04, 1e-9) // entered 100, settled 104, on €100
		})

		Convey("A per-node trailing offset overrides the global default", func() {
			thoughts := []perspectives.Thought{
				{When: notHolding(perspectives.CategoryVerticalIgnition), Do: perspectives.Act{Type: perspectives.ActionMarket}},
				{
					When: perspectives.Predicate{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: perspectives.ObservationHolding},
					Do:   perspectives.Act{Type: perspectives.ActionTrailingStop, Offset: 0.02}, // tight 2% trail
				},
			}

			rows := []perspectives.Measurement{
				ignite(100, base),
				ignite(110, base.Add(time.Second)),  // peak 110
				ignite(107, base.Add(2*time.Second)), // 2.7% off the peak — breaches a 2% trail
			}

			costs := frictionlessCosts()
			costs.TrailingPct = 0.10 // a loose global trail that would NOT have fired at 107

			sim := NewThoughtSimulation(context.Background(), thoughts, PrecompileTape(rows), costs)
			result := sim.Result()

			So(result.ClosedTrades, ShouldEqual, 1)
			// trail fired at 110*(1-0.02)=107.8, filled at 107 -> +7% on €100.
			So(result.Score, ShouldAlmostEqual, 0.07, 1e-9)
		})
	})
}
