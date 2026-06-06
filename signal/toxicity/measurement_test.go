package toxicity

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func warmTrackerSNR(t *testing.T, tracker *Tracker, symbol string) {
	t.Helper()

	tracker.ObserveMid(symbol, market.Pair{}, 100)
	tracker.ObserveLast(symbol, market.Pair{}, 100)

	originalMinFillToCancel := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
	viper.Set("signals.min_fill_to_cancel_ratio", 0.15)

	state := tracker.stateLocked(symbol, market.Pair{})
	state.cancelBid = 0.3
	state.fillBid = 0.1

	for index := range 15 {
		evidence := 0.45

		if index%2 == 1 {
			evidence = 0.55
		}

		err := state.tracked.Observe(
			types.CategoryLiquidityVacuum,
			evidence,
		)
		convey.So(err, convey.ShouldBeNil)

		_, err = tracker.surpriseField.Score(symbol, types.CategoryLiquidityVacuum)
		convey.So(err, convey.ShouldBeNil)
	}

	viper.Set("signals.min_fill_to_cancel_ratio", originalMinFillToCancel)
}

func TestTrackerMeasureToxicBluffChurnStrength(t *testing.T) {
	convey.Convey("Given a near-touch toxic cancel with churn ratio", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "ETH/EUR"

		warmTrackerSNR(t, tracker, symbol)

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		tracker.ObserveLast(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[priceKey(100, market.Pair{})] = now.Add(time.Minute)
		state.toxicChurn[priceKey(100, market.Pair{})] = 4.5

		measurement, err := tracker.Measure(symbol, now)

		convey.Convey("It should retain churn ratio as strength with confidence and SNR", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(measurement.Category, convey.ShouldEqual, types.CategoryToxicBluff)
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

		warmTrackerSNR(t, tracker, symbol)

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		tracker.ObserveLast(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[priceKey(100, market.Pair{})] = now.Add(time.Minute)

		measurement, err := tracker.Measure(symbol, now)

		convey.Convey("It should publish toxic bluff with measurable strength", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(measurement.Category, convey.ShouldEqual, types.CategoryToxicBluff)
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

		originalMinFillToCancel := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
		viper.Set("signals.min_fill_to_cancel_ratio", 0.15)
		defer viper.Set("signals.min_fill_to_cancel_ratio", originalMinFillToCancel)

		warmTrackerSNR(t, tracker, symbol)

		state := tracker.stateLocked(symbol, market.Pair{})
		state.cancelBid = 0.3
		state.fillBid = 0.1
		tracker.ObserveLast(symbol, market.Pair{}, 50000)

		measurement, err := tracker.Measure(symbol, now)

		convey.Convey("It should publish bounded strength with confidence and SNR", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(measurement.Category, convey.ShouldEqual, types.CategoryLiquidityVacuum)
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

		originalMinFillToCancel := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
		viper.Set("signals.min_fill_to_cancel_ratio", 0.15)
		defer viper.Set("signals.min_fill_to_cancel_ratio", originalMinFillToCancel)

		state := tracker.stateLocked(symbol, market.Pair{})
		state.cancelBid = 1
		state.fillBid = 0

		measurement, err := tracker.Measure(symbol, now)

		convey.Convey("It should not publish an incomplete asymmetry reading", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(measurement.Source, convey.ShouldEqual, types.SourceNone)
		})
	})
}

func TestTrackerMeasureHardSupport(t *testing.T) {
	convey.Convey("Given balanced visible depth without toxic cancels", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "BTC/EUR"

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		tracker.ObserveLast(symbol, market.Pair{}, 100)
		tracker.ApplyBookLevel(symbol, market.Pair{}, SideBid, 99.5, 80, now)
		tracker.ApplyBookLevel(symbol, market.Pair{}, SideAsk, 100.5, 80, now)

		measurement, err := tracker.Measure(symbol, now)

		convey.Convey("It should classify hard support", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(measurement.Source, convey.ShouldEqual, types.SourceToxicity)
			convey.So(measurement.Category, convey.ShouldEqual, types.CategoryHardSupport)
			convey.So(measurement.Strength, convey.ShouldEqual, 1)
		})
	})
}
