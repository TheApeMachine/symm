package replay

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// frictionless: zero fees + slippage, immediate execution, a €100 EUR wallet.
func frictionlessCosts() ReplayCosts {
	return ReplayCosts{
		StartingCapital:        100,
		PositionFraction:       1,
		WalletCurrency:         "EUR",
		WalletBalances:         map[string]float64{"EUR": 100},
		ExecutionStressEnabled: false,
	}
}

func ignite(last float64, at time.Time) types.Measurement {
	return types.Measurement{
		Symbol: "BTC/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: last, At: at,
	}
}

func notHolding(category types.CategoryType) reasoning.Predicate {
	return reasoning.Predicate{All: []reasoning.Predicate{
		{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationNotHolding},
		{Subject: reasoning.SubjectSignal, Category: category, Unit: reasoning.UnitSNR, Op: reasoning.ComparisonAtLeast, Value: 1.0},
	}}
}

func priceRow(last float64, at time.Time) types.Measurement {
	return types.Measurement{Symbol: "BTC/EUR", Last: last, At: at}
}

func priceCrossedUp(level float64) reasoning.Predicate {
	return reasoning.Predicate{
		Subject: reasoning.SubjectPrice, Unit: reasoning.UnitNone,
		Ago: 1, Op: reasoning.ComparisonCrossedUp, Value: level,
	}
}

// TestThoughtSimulationArmsAcrossTicks proves the stateful walk threads through the
// real ledger + window: a transient parent edge (price crosses 101) fires on one
// tick and LATCHES, so a later child edge (price crosses 103) — on a tick where the
// parent edge no longer holds — still reaches the entry. A single-tick walk could
// never enter here, because the parent gate is shut by the time the child triggers.
func TestThoughtSimulationArmsAcrossTicks(t *testing.T) {
	Convey("Given a frictionless €100 wallet and a price path that crosses two levels on different ticks", t, func() {
		testconfig.Load(t)
		base := time.Unix(1_700_000_000, 0)

		thoughts := []reasoning.Thought{
			{
				When: reasoning.Predicate{All: []reasoning.Predicate{
					{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationNotHolding},
					priceCrossedUp(101), // transient: true only on the tick price crosses 101
				}},
				Then: []reasoning.Thought{{
					When: priceCrossedUp(103), // fires a tick later, once the parent has latched
					Do:   reasoning.Act{Type: reasoning.ActionMarket},
				}},
			},
			{
				When: reasoning.Predicate{All: []reasoning.Predicate{
					{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationHolding},
					{Subject: reasoning.SubjectSignal, Category: types.CategoryActiveReversal, Unit: reasoning.UnitSNR, Op: reasoning.ComparisonAtLeast, Value: 1.0},
				}},
				Do: reasoning.Act{Type: reasoning.ActionSettlePosition},
			},
		}

		rows := []types.Measurement{
			priceRow(100, base),                    // no cross yet
			priceRow(102, base.Add(time.Second)),   // crosses 101 -> parent latches; 102<103 so child holds off
			priceRow(104, base.Add(2*time.Second)), // parent edge gone; crosses 103 -> child enters at 104
			{Symbol: "BTC/EUR", Category: types.CategoryActiveReversal, SNR: 1.5, Last: 108, At: base.Add(3 * time.Second)},
		}

		sim := NewThoughtSimulation(context.Background(), thoughts, mustPrecompileTape(t, rows), frictionlessCosts())
		result := sim.Result()

		So(result.ClosedTrades, ShouldEqual, 1)  // it did enter, only because the parent latched
		So(result.Score, ShouldBeGreaterThan, 0) // entered 104, settled 108
	})
}

func TestThoughtSimulationCountsFundBlockedEntries(t *testing.T) {
	Convey("Given a €100 wallet that one position fully deploys", t, func() {
		testconfig.Load(t)
		base := time.Unix(1_700_000_000, 0)

		// Enter on an ignition; no exit, so the first fill camps and ties up the cash.
		thoughts := []reasoning.Thought{
			{When: notHolding(types.CategoryVerticalIgnition), Do: reasoning.Act{Type: reasoning.ActionMarket}},
		}

		rows := []types.Measurement{
			ignite(100, base), // BTC/EUR enters, spends the whole €100
			{Symbol: "ETH/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 50, At: base.Add(time.Second)},
			{Symbol: "ETH/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 51, At: base.Add(2 * time.Second)},
		}

		result := NewThoughtSimulation(context.Background(), thoughts, mustPrecompileTape(t, rows), frictionlessCosts()).Result()

		Convey("ETH entries are wanted but blocked for want of free capital, and counted", func() {
			So(result.FundBlocked, ShouldBeGreaterThan, 0)
		})
	})
}

func TestThoughtSimulationFeesReduceReturn(t *testing.T) {
	Convey("Given the same reasoning tree scored with and without fees", t, func() {
		testconfig.Load(t)
		base := time.Unix(1_700_000_000, 0)

		thoughts := []reasoning.Thought{
			{When: notHolding(types.CategoryVerticalIgnition), Do: reasoning.Act{Type: reasoning.ActionMarket}},
			{
				When: reasoning.Predicate{All: []reasoning.Predicate{
					{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationHolding},
					{Subject: reasoning.SubjectSignal, Category: types.CategoryActiveReversal, Unit: reasoning.UnitSNR, Op: reasoning.ComparisonAtLeast, Value: 1.0},
				}},
				Do: reasoning.Act{Type: reasoning.ActionSettlePosition},
			},
		}

		rows := []types.Measurement{
			ignite(100, base),
			ignite(110, base.Add(time.Second)),
			{Symbol: "BTC/EUR", Category: types.CategoryActiveReversal, SNR: 1.5, Last: 110, At: base.Add(2 * time.Second)},
		}

		tape := mustPrecompileTape(t, rows)

		free := NewThoughtSimulation(context.Background(), thoughts, tape, frictionlessCosts()).Result()

		costs := frictionlessCosts()
		costs.TakerFeePct = 0.005 // 0.5% per side
		taxed := NewThoughtSimulation(context.Background(), thoughts, tape, costs).Result()

		Convey("Both trade once, but fees eat into the realized return", func() {
			So(free.ClosedTrades, ShouldEqual, 1)
			So(taxed.ClosedTrades, ShouldEqual, 1)
			So(taxed.Score, ShouldBeLessThan, free.Score)
		})
	})
}

func TestThoughtSimulationScoresAReasoningTree(t *testing.T) {
	Convey("Given a frictionless €100 wallet and a tape", t, func() {
		testconfig.Load(t)
		base := time.Unix(1_700_000_000, 0)

		Convey("A reasoning tree (enter on ignition, settle on reversal) trades and profits", func() {
			thoughts := []reasoning.Thought{
				{When: notHolding(types.CategoryVerticalIgnition), Do: reasoning.Act{Type: reasoning.ActionMarket}},
				{
					When: reasoning.Predicate{All: []reasoning.Predicate{
						{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationHolding},
						{Subject: reasoning.SubjectSignal, Category: types.CategoryActiveReversal, Unit: reasoning.UnitSNR, Op: reasoning.ComparisonAtLeast, Value: 1.0},
					}},
					Do: reasoning.Act{Type: reasoning.ActionSettlePosition},
				},
			}

			rows := []types.Measurement{
				ignite(100, base),
				ignite(102, base.Add(time.Second)),
				ignite(104, base.Add(2*time.Second)),
				{Symbol: "BTC/EUR", Category: types.CategoryActiveReversal, SNR: 1.5, Last: 104, At: base.Add(3 * time.Second)},
			}

			sim := NewThoughtSimulation(context.Background(), thoughts, mustPrecompileTape(t, rows), frictionlessCosts())
			result := sim.Result()

			So(result.ClosedTrades, ShouldEqual, 1)
			So(result.Score, ShouldAlmostEqual, 0.04, 1e-9) // entered 100, settled 104, on €100
		})

		Convey("A per-node trailing offset overrides the dynamic default", func() {
			thoughts := []reasoning.Thought{
				{When: notHolding(types.CategoryVerticalIgnition), Do: reasoning.Act{Type: reasoning.ActionMarket}},
				{
					When: reasoning.Predicate{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationHolding},
					Do:   reasoning.Act{Type: reasoning.ActionTrailingStop, Offset: 0.02}, // tight 2% trail
				},
			}

			rows := []types.Measurement{
				ignite(100, base),
				ignite(110, base.Add(time.Second)),   // peak 110
				ignite(107, base.Add(2*time.Second)), // 2.7% off the peak — breaches a 2% trail
			}

			sim := NewThoughtSimulation(context.Background(), thoughts, mustPrecompileTape(t, rows), frictionlessCosts())
			result := sim.Result()

			So(result.ClosedTrades, ShouldEqual, 1)
			// trail fired at 110*(1-0.02)=107.8, filled at 107 -> +7% on €100.
			So(result.Score, ShouldAlmostEqual, 0.07, 1e-9)
		})
	})
}
