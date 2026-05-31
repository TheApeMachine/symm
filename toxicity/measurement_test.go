package toxicity

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTrackerMeasureToxicBluffChurnStrength(t *testing.T) {
	convey.Convey("Given a near-touch toxic cancel with churn ratio", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "ETH/EUR"

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[100] = now.Add(time.Minute)
		state.toxicChurn[100] = 4.5

		measurement, ok := tracker.Measure(symbol, now)

		convey.Convey("It should retain churn ratio as strength before SNR warmup", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(measurement.Category, convey.ShouldEqual, perspectives.CategoryToxicBluff)
			convey.So(measurement.Strength, convey.ShouldEqual, 4.5)
		})
	})
}

func TestTrackerMeasureToxicBluff(t *testing.T) {
	convey.Convey("Given a near-touch toxic cancel flag", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "ETH/EUR"

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[100] = now.Add(time.Minute)

		measurement, ok := tracker.Measure(symbol, now)

		convey.Convey("It should publish toxic bluff with measurable strength", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(measurement.Category, convey.ShouldEqual, perspectives.CategoryToxicBluff)
			convey.So(measurement.Strength, convey.ShouldBeGreaterThan, 0)
		})
	})
}
