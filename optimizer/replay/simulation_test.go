package replay

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayResultReturnPerTrade(t *testing.T) {
	Convey("Given replay results", t, func() {
		Convey("It should average score by closed trades", func() {
			result := ReplayResult{Score: 0.10, ClosedTrades: 2}

			So(result.ReturnPerTrade(), ShouldAlmostEqual, 0.05, 1e-9)
		})

		Convey("It should return zero without closed trades", func() {
			So(ReplayResult{Score: 0.10}.ReturnPerTrade(), ShouldEqual, 0)
		})
	})
}

func TestThoughtSimulationScoreEmptyTape(t *testing.T) {
	Convey("Given an empty tape", t, func() {
		simulation := NewThoughtSimulation(context.Background(), nil, ReplayTape{}, frictionlessCosts())

		Convey("It should return zero score and result", func() {
			So(simulation.Score(), ShouldEqual, 0)

			result := simulation.Result()

			So(result.Score, ShouldEqual, 0)
			So(result.ClosedTrades, ShouldEqual, 0)
		})
	})
}

func TestPrecompiledRegimeMatchesClassifyRegime(t *testing.T) {
	Convey("Given a precompiled tape", t, func() {
		testconfig.Load(t)
		rows := make([]types.Measurement, 0, 48)

		for index := range 48 {
			rows = append(rows, types.Measurement{
				Symbol: "BTC/EUR",
				Last:   100 + float64(index)*0.002,
				At:     time.Unix(1_700_000_000, 0).Add(time.Duration(index) * time.Second),
			})
		}

		tape := mustPrecompileTape(t, rows)

		Convey("It should stamp the same regime live classification would use", func() {
			for tickIndex, tick := range tape.Ticks {
				snapshots := tape.AppendSnapshot(tickIndex, nil)
				live := perspectives.ClassifyRegime(snapshots).Regime

				So(tick.Regime, ShouldEqual, live)
			}
		})
	})
}

func TestThoughtSimulationSearchResultSkipsAttribution(t *testing.T) {
	Convey("Given a replay that closes a round trip", t, func() {
		testconfig.Load(t)
		base := time.Unix(1_700_000_000, 0)
		thoughts := []reasoning.Thought{
			{
				Name: "entry",
				When: reasoning.Predicate{
					Subject:   reasoning.SubjectPosition,
					Op:        reasoning.ComparisonEquals,
					Lifecycle: types.ObservationNotHolding,
				},
				Do: reasoning.Act{Type: reasoning.ActionMarket},
			},
			{
				When: reasoning.Predicate{
					Subject:   reasoning.SubjectPosition,
					Op:        reasoning.ComparisonEquals,
					Lifecycle: types.ObservationHolding,
				},
				Do: reasoning.Act{Type: reasoning.ActionSettle},
			},
		}
		rows := []types.Measurement{
			{Symbol: "BTC/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 100, Bid: 99.9, Ask: 100.1, At: base},
			{Symbol: "BTC/EUR", Last: 105, Bid: 104.9, Ask: 105.1, At: base.Add(time.Second)},
		}
		tape := mustPrecompileTape(t, rows)
		costs := frictionlessCosts()
		simulation := NewThoughtSimulation(context.Background(), thoughts, tape, costs)

		Convey("SearchResult should match core PnL without per-setup maps", func() {
			search := simulation.SearchResult()
			fullCosts := costs
			fullCosts.CollectAttribution = true
			fullCosts.CollectTrades = true
			full := NewThoughtSimulation(context.Background(), thoughts, tape, fullCosts).Result()

			So(search.Score, ShouldAlmostEqual, full.Score, 1e-9)
			So(search.ClosedTrades, ShouldEqual, full.ClosedTrades)
			So(search.PerStrategy, ShouldBeNil)
			So(search.Trades, ShouldBeNil)
			So(len(full.PerStrategy), ShouldBeGreaterThan, 0)
			So(len(full.Trades), ShouldEqual, full.ClosedTrades)
		})
	})
}

func BenchmarkThoughtSimulationResult(b *testing.B) {
	base := time.Unix(1_700_000_000, 0)
	thoughts := []reasoning.Thought{
		{
			When: reasoning.Predicate{
				Subject:   reasoning.SubjectPosition,
				Op:        reasoning.ComparisonEquals,
				Lifecycle: types.ObservationNotHolding,
			},
			Do: reasoning.Act{Type: reasoning.ActionMarket},
		},
	}
	rows := []types.Measurement{
		{Symbol: "BTC/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 100, At: base},
		{Symbol: "BTC/EUR", Last: 105, At: base.Add(time.Second)},
	}
	tape := mustPrecompileTape(b, rows)
	costs := frictionlessCosts()
	simulation := NewThoughtSimulation(context.Background(), thoughts, tape, costs)

	for b.Loop() {
		_ = simulation.SearchResult()
	}
}

func BenchmarkThoughtSimulationResultAttributed(b *testing.B) {
	base := time.Unix(1_700_000_000, 0)
	thoughts := []reasoning.Thought{
		{
			When: reasoning.Predicate{
				Subject:   reasoning.SubjectPosition,
				Op:        reasoning.ComparisonEquals,
				Lifecycle: types.ObservationNotHolding,
			},
			Do: reasoning.Act{Type: reasoning.ActionMarket},
		},
	}
	rows := []types.Measurement{
		{Symbol: "BTC/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 100, At: base},
		{Symbol: "BTC/EUR", Last: 105, At: base.Add(time.Second)},
	}
	tape := mustPrecompileTape(b, rows)
	costs := frictionlessCosts()
	costs.CollectAttribution = true
	costs.CollectTrades = true
	simulation := NewThoughtSimulation(context.Background(), thoughts, tape, costs)

	for b.Loop() {
		_ = simulation.Result()
	}
}
