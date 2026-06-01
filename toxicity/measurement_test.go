package toxicity

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTrackerMeasureToxicBluffChurnStrength(t *testing.T) {
	convey.Convey("Given a near-touch toxic cancel with churn ratio", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "ETH/EUR"

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		tracker.ObserveLast(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[100] = now.Add(time.Minute)
		state.toxicChurn[100] = 4.5

		measurement, ok := tracker.Measure(symbol, now)

		convey.Convey("It should retain churn ratio as strength with confidence and SNR", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(measurement.Category, convey.ShouldEqual, perspectives.CategoryToxicBluff)
			convey.So(measurement.Strength, convey.ShouldEqual, 4.5)
			convey.So(measurement.Confidence, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.SNR, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.Last, convey.ShouldEqual, 100)
		})
	})
}

func TestTrackerMeasureToxicBluff(t *testing.T) {
	convey.Convey("Given a near-touch toxic cancel flag", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "ETH/EUR"

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		tracker.ObserveLast(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[100] = now.Add(time.Minute)

		measurement, ok := tracker.Measure(symbol, now)

		convey.Convey("It should publish toxic bluff with measurable strength", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(measurement.Category, convey.ShouldEqual, perspectives.CategoryToxicBluff)
			convey.So(measurement.Strength, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.Confidence, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.SNR, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.Last, convey.ShouldEqual, 100)
		})
	})
}

func TestTrackerMeasureLiquidityVacuumFiniteStrength(t *testing.T) {
	convey.Convey("Given cancel/fill asymmetry with observed fill flow", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "BTC/EUR"

		viper.Set("signals.min_fill_to_cancel_ratio", 0.15)
		defer viper.Set("signals.min_fill_to_cancel_ratio", 0.0)

		state := tracker.stateLocked(symbol, market.Pair{})
		state.cancelBid = 0.3
		state.fillBid = 0.1
		tracker.ObserveLast(symbol, market.Pair{}, 50000)

		measurement, ok := tracker.Measure(symbol, now)

		convey.Convey("It should publish bounded strength with confidence and SNR", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(measurement.Category, convey.ShouldEqual, perspectives.CategoryLiquidityVacuum)
			convey.So(measurement.Strength, convey.ShouldAlmostEqual, 20, 0.01)
			convey.So(measurement.Strength, convey.ShouldBeLessThan, 1e6)
			convey.So(math.IsInf(measurement.Strength, 0), convey.ShouldBeFalse)
			convey.So(math.IsNaN(measurement.Strength), convey.ShouldBeFalse)
			convey.So(measurement.Confidence, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.SNR, convey.ShouldBeGreaterThan, 0)
			convey.So(measurement.Last, convey.ShouldEqual, 50000)
		})
	})
}

func TestTrackerMeasureLiquidityVacuumRequiresFillFlow(t *testing.T) {
	convey.Convey("Given cancel flow without matched fill", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "BTC/EUR"

		viper.Set("signals.min_fill_to_cancel_ratio", 0.15)
		defer viper.Set("signals.min_fill_to_cancel_ratio", 0.0)

		state := tracker.stateLocked(symbol, market.Pair{})
		state.cancelBid = 1
		state.fillBid = 0

		_, ok := tracker.Measure(symbol, now)

		convey.Convey("It should not publish an incomplete asymmetry reading", func() {
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}
