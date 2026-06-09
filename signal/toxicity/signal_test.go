package toxicity

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func newTestTracker(t testing.TB) *Tracker {
	t.Helper()

	return NewTracker()
}

func newTestSignal(t testing.TB, symbol string, entity logic.EntityType) *Signal {
	t.Helper()

	return NewSignal(
		symbol,
		logic.NewEntity(entity),
		8,
		newTestTracker(t),
		2.0,
		0.5,
	)
}

func warmSignalSurprise(t *testing.T, signal *Signal, symbol string) {
	t.Helper()

	signal.tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
	signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)

	originalMinFillToCancel := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
	viper.Set("signals.min_fill_to_cancel_ratio", 0.15)

	state := signal.tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.cancelBid = 0.3
	state.fillBid = 0.1

	for range 15 {
		_, err := signal.Measure(nil)
		So(err, ShouldBeNil)
	}

	viper.Set("signals.min_fill_to_cancel_ratio", originalMinFillToCancel)
}

func TestDefaultTracker(t *testing.T) {
	Convey("Given the process-wide tracker", t, func() {
		tracker := Default()

		Convey("It should match the package default instance", func() {
			So(tracker, ShouldEqual, defaultTracker)
		})
	})
}

func TestIsToxicHelper(t *testing.T) {
	Convey("Given an unknown symbol and price", t, func() {
		now := time.Now()

		Convey("It should delegate to the default tracker", func() {
			So(IsToxic("ZZZ/ISOLATED", 123.456789, now), ShouldBeFalse)
		})
	})
}

func TestNearTouchToxic(t *testing.T) {
	Convey("Given a near-touch toxic flag on the shared tracker", t, func() {
		ResetDefault()
		tracker := Default()
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should report near-touch toxicity for that symbol", func() {
			So(NearTouchToxic(symbol, now), ShouldBeTrue)
			So(NearTouchToxic("ETH/EUR", now), ShouldBeFalse)
		})
	})
}

func TestPriceKey(t *testing.T) {
	Convey("Given a pair with tick size", t, func() {
		pair := krakenmarket.Pair{TickSize: "0.1"}

		Convey("It should round prices to tick boundaries", func() {
			So(priceKey(100.01, pair), ShouldEqual, priceKey(100.04, pair))
			So(priceFromKey(priceKey(100.01, pair), pair), ShouldAlmostEqual, 100.0, 1e-9)
		})
	})

	Convey("Given a pair without tick size", t, func() {
		pair := krakenmarket.Pair{}

		Convey("It should discretize with fixed scale", func() {
			key := priceKey(100.000000001, pair)
			So(priceFromKey(key, pair), ShouldAlmostEqual, 100.0, 1e-4)
		})
	})
}

func TestIsToxicPriceKeyLookup(t *testing.T) {
	Convey("Given a toxic level stored at a rounded price", t, func() {
		tracker := newTestTracker(t)
		symbol := "ETH/EUR"
		now := time.Now()
		pair := krakenmarket.Pair{TickSize: "0.01"}

		state := tracker.stateLocked(symbol, pair)
		state.toxic[priceKey(100.0, pair)] = now.Add(toxicCooldown)

		Convey("It should match a slightly perturbed lookup price", func() {
			So(tracker.IsToxic(symbol, 100.0000004, now), ShouldBeTrue)
		})
	})
}

func TestTrackerApplyOrderToxicCancel(t *testing.T) {
	Convey("Given a large near-touch cancel", t, func() {
		tracker := newTestTracker(t)
		now := time.Now()
		symbol := "TEST/TOXIC"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFlashChurnFlagsNearTouchLevel(t *testing.T) {
	Convey("Given rapid near-touch add/delete churn without fills", t, func() {
		tracker := newTestTracker(t)
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFillToCancelThreshold(t *testing.T) {
	Convey("Given a tracker without cached ratio", t, func() {
		tracker := newTestTracker(t)

		Convey("It should lazily load threshold from viper", func() {
			So(tracker.fillToCancelThreshold(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestTrackerBookSideDepth(t *testing.T) {
	Convey("Given mid-price observations", t, func() {
		tracker := newTestTracker(t)
		now := time.Now()

		tracker.ObserveMid("BTC/EUR", krakenmarket.Pair{}, 100)
		tracker.ObserveLast("BTC/EUR", krakenmarket.Pair{}, 101)

		Convey("It should retain symbol state", func() {
			So(tracker.IsToxic("BTC/EUR", 100, now), ShouldBeFalse)
		})
	})
}

func TestSignalMeasureToxicBluffChurnStrength(t *testing.T) {
	Convey("Given a near-touch toxic cancel with churn ratio", t, func() {
		signal := newTestSignal(t, "ETH/EUR", logic.EntityBook)
		now := time.Now()
		symbol := "ETH/EUR"

		warmSignalSurprise(t, signal, symbol)

		signal.tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		state := signal.tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.toxic[priceKey(100, krakenmarket.Pair{})] = now.Add(time.Minute)
		state.toxicChurn[priceKey(100, krakenmarket.Pair{})] = 4.5

		measurement, err := signal.Measure(nil)

		Convey("It should retain churn ratio as strength with confidence and surprise", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryToxicBluff)
			So(measurement.Strength, ShouldEqual, 4.5)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Surprise, ShouldBeGreaterThan, 0)
			So(measurement.Price, ShouldEqual, 100)
		})
	})
}

func TestSignalMeasureToxicBluffSaturatedEvidence(t *testing.T) {
	Convey("Given finite near-touch churn that saturates evidence", t, func() {
		signal := newTestSignal(t, "DOGE/EUR", logic.EntityBook)
		now := time.Now()
		symbol := "DOGE/EUR"

		warmSignalSurprise(t, signal, symbol)

		signal.tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		state := signal.tracker.stateLocked(symbol, krakenmarket.Pair{})
		key := priceKey(100, krakenmarket.Pair{})
		state.toxic[key] = now.Add(time.Minute)
		state.toxicChurn[key] = math.MaxFloat64

		measurement, err := signal.Measure(nil)

		Convey("It should publish without rejecting unit-band confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryToxicBluff)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestSignalMeasureToxicBluff(t *testing.T) {
	Convey("Given a near-touch toxic cancel flag", t, func() {
		signal := newTestSignal(t, "ETH/EUR", logic.EntityBook)
		now := time.Now()
		symbol := "ETH/EUR"

		warmSignalSurprise(t, signal, symbol)

		signal.tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		signal.tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		signal.tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)

		measurement, err := signal.Measure(nil)

		Convey("It should publish toxic bluff with measurable strength", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryToxicBluff)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Surprise, ShouldBeGreaterThan, 0)
			So(measurement.Price, ShouldEqual, 100)
		})
	})
}

func TestSignalMeasureLiquidityVacuumFiniteStrength(t *testing.T) {
	Convey("Given cancel/fill asymmetry with observed fill flow", t, func() {
		signal := newTestSignal(t, "BTC/EUR", logic.EntityBook)
		symbol := "BTC/EUR"

		originalMinFillToCancel := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
		viper.Set("signals.min_fill_to_cancel_ratio", 0.15)
		defer viper.Set("signals.min_fill_to_cancel_ratio", originalMinFillToCancel)

		warmSignalSurprise(t, signal, symbol)

		state := signal.tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.cancelBid = 0.3
		state.fillBid = 0.1
		signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 50000)

		measurement, err := signal.Measure(nil)

		Convey("It should publish bounded strength with confidence and surprise", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryLiquidityVacuum)
			So(measurement.Strength, ShouldAlmostEqual, 20, 0.01)
			So(measurement.Strength, ShouldBeLessThan, 1e6)
			So(math.IsInf(measurement.Strength, 0), ShouldBeFalse)
			So(math.IsNaN(measurement.Strength), ShouldBeFalse)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Surprise, ShouldBeGreaterThan, 0)
			So(measurement.Price, ShouldEqual, 50000)
		})
	})
}

func TestSignalMeasureLiquidityVacuumRequiresFillFlow(t *testing.T) {
	Convey("Given cancel flow without matched fill", t, func() {
		signal := newTestSignal(t, "BTC/EUR", logic.EntityBook)
		symbol := "BTC/EUR"

		originalMinFillToCancel := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
		viper.Set("signals.min_fill_to_cancel_ratio", 0.15)
		defer viper.Set("signals.min_fill_to_cancel_ratio", originalMinFillToCancel)

		state := signal.tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.cancelBid = 1
		state.fillBid = 0

		measurement, err := signal.Measure(nil)

		Convey("It should not publish an incomplete asymmetry reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceNone)
		})
	})
}

func TestSignalMeasureHardSupport(t *testing.T) {
	Convey("Given balanced visible depth without toxic cancels", t, func() {
		signal := newTestSignal(t, "BTC/EUR", logic.EntityBook)
		now := time.Now()
		symbol := "BTC/EUR"

		signal.tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		signal.tracker.ApplyBookLevel(symbol, krakenmarket.Pair{}, SideBid, 99.5, 80, now)
		signal.tracker.ApplyBookLevel(symbol, krakenmarket.Pair{}, SideAsk, 100.5, 80, now)

		measurement, err := signal.Measure(nil)

		Convey("It should classify hard support", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceToxicity)
			So(measurement.Category, ShouldEqual, logic.CategoryHardSupport)
			So(measurement.Strength, ShouldEqual, 1)
		})
	})
}

func TestSystemFeedLevel3(t *testing.T) {
	Convey("Given a level3 update with add/delete events", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		ResetDefault()
		system := NewSystem(ctx, pool)
		now := time.Now()

		system.tracker.ObserveMid("BTC/EUR", krakenmarket.Pair{}, 100)
		state := system.tracker.stateLocked("BTC/EUR", krakenmarket.Pair{})
		state.bidTotal = 100

		system.feedLevel3(&krakenmarket.Level3Update{
			Symbol: "BTC/EUR",
			Bids: []krakenmarket.Bid{
				{
					Event:      "add",
					OrderID:    "l3-2",
					LimitPrice: 100,
					OrderQty:   15,
					Timestamp:  now,
				},
				{
					Event:      "delete",
					OrderID:    "l3-2",
					LimitPrice: 100,
					OrderQty:   15,
					Timestamp:  now,
				},
			},
		})

		Convey("It should classify per-order churn as toxic", func() {
			So(system.tracker.IsToxic("BTC/EUR", 100, now), ShouldBeTrue)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := newTestSignal(b, "BTC/EUR", logic.EntityBook)
	now := time.Now()
	symbol := "BTC/EUR"

	signal.tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
	signal.tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
	state := signal.tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.bidTotal = 100

	b.ReportAllocs()

	for b.Loop() {
		signal.tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		signal.tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)
		_, _ = signal.Measure(nil)
	}
}

func BenchmarkTrackerApplyOrder(b *testing.B) {
	tracker := newTestTracker(b)
	now := time.Now()
	symbol := "BTC/EUR"

	tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
	state := tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.bidTotal = 100

	for b.Loop() {
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)
	}
}
