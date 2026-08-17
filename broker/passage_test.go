package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func passagePositionFixture(trigger string) *Position {
	return &Position{
		pair: kraken.InstrumentPair{Symbol: "SIM/USD"},
		Decision: types.Decision{
			ID:     "decision-id",
			Symbol: "SIM/USD",
			Cause:  "vertical_ignition",
		},
		Holding: &types.Holding{
			Symbol:     "SIM/USD",
			EntryPrice: decimal.NewFromFloat64(100),
			Stoploss: &types.Stoploss{
				TriggerReason: trigger,
				Plan: &types.RiskPlan{
					Present:        true,
					RiskDistance:   decimal.NewFromFloat64(10),
					EntryNoiseBand: decimal.NewFromFloat64(1),
				},
			},
		},
	}
}

func TestPassageTrackerObserve(t *testing.T) {
	Convey("Given a tracked lot entered at one hundred with a ten-unit risk distance", t, func() {
		position := passagePositionFixture("")
		tracker := newPassageTracker(
			position,
			position.Holding.EntryPrice,
			2,
		)

		Convey("It should record excursions in risk distances", func() {
			tracker.observe(position, decimal.NewFromFloat64(95))
			tracker.observe(position, decimal.NewFromFloat64(104))
			tracker.observe(position, decimal.NewFromFloat64(97))

			So(tracker.episode.MaxAdverse, ShouldAlmostEqual, 0.5, 1e-12)
			So(tracker.episode.MaxFavorable, ShouldAlmostEqual, 0.4, 1e-12)
			So(tracker.episode.Observations, ShouldHaveLength, 4)
		})
	})
}

func TestPassageTrackerComplete(t *testing.T) {
	Convey("Given a tracked lot that hit its hard floor", t, func() {
		position := passagePositionFixture(types.TriggerHardFloor)
		tracker := newPassageTracker(position, decimal.NewFromFloat64(100), 1)

		Convey("It should classify the loss-first outcome", func() {
			episode, decided := tracker.complete(position)
			So(decided, ShouldBeTrue)
			So(episode.Outcome, ShouldEqual, types.OutcomeLossFirst)
			So(episode.ExitReason, ShouldEqual, types.TriggerHardFloor)
		})
	})

	Convey("Given a tracked lot that exited through its protected floor", t, func() {
		position := passagePositionFixture(types.TriggerProtectedFloor)
		tracker := newPassageTracker(position, decimal.NewFromFloat64(100), 1)

		Convey("It should classify the profit-first outcome", func() {
			episode, decided := tracker.complete(position)
			So(decided, ShouldBeTrue)
			So(episode.Outcome, ShouldEqual, types.OutcomeProfitFirst)
		})
	})

	Convey("Given a tracked lot closed for reasons neither boundary owns", t, func() {
		position := passagePositionFixture(types.TriggerRegimeInvalidated)
		tracker := newPassageTracker(position, decimal.NewFromFloat64(100), 1)

		Convey("It should censor the episode instead of inventing an outcome", func() {
			episode, decided := tracker.complete(position)
			So(decided, ShouldBeFalse)
			So(episode.Censored, ShouldBeTrue)
		})
	})
}

func TestDeskFoldPassage(t *testing.T) {
	Convey("Given a desk holding a first-passage model", t, func() {
		desk := &Desk{passage: types.NewPassageModel()}

		Convey("A decided lot folds its episode into the model", func() {
			position := passagePositionFixture(types.TriggerProtectedFloor)
			position.passage = newPassageTracker(
				position, decimal.NewFromFloat64(100), 1,
			)
			position.passage.episode.MaxAdverse = 0.6

			desk.foldPassage(position)

			_, ready := desk.PassageAdverseQuantile(0.95)
			So(ready, ShouldBeFalse)
			So(desk.passage.Total(), ShouldEqual, 1)
		})

		Convey("An unfilled lot contributes nothing", func() {
			position := passagePositionFixture(types.TriggerHardFloor)
			desk.foldPassage(position)
			So(desk.passage.Total(), ShouldEqual, 0)
		})
	})
}

func BenchmarkPassageTrackerObserve(b *testing.B) {
	position := passagePositionFixture("")
	tracker := newPassageTracker(position, decimal.NewFromFloat64(100), 1)
	mark := decimal.NewFromFloat64(99.5)
	b.ReportAllocs()

	for b.Loop() {
		tracker.observe(position, mark)
	}
}
