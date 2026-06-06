package replay

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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
		_ = simulation.Result()
	}
}
