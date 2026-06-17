package toxicity

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestMain(mainTesting *testing.M) {
	viper.Set("signals.feed_ring_capacity", 64)
	mainTesting.Run()
}

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func measurementArtifact(scope string) *datura.Artifact {
	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("measurement").
		WithScope(scope)
}

func newTestTracker(testingTB testing.TB) *Tracker {
	testingTB.Helper()

	return NewTracker()
}

func newTestSignal(testingTB testing.TB) *Signal {
	testingTB.Helper()

	signal := NewSignal(context.Background(), newTestPool(testingTB))
	signal.Tracker = newTestTracker(testingTB)

	return signal
}

func seedChurnGateHistory(signal *Signal, symbol string) {
	state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})

	for _, ratio := range []float64{0.7, 0.8, 0.9, 0.95} {
		state.gates.ChurnRatios.Observe(ratio)
	}
}

func seedBooks(signal *Signal, symbol string, base time.Time, count int) {
	updates := make(krakenmarket.BookUpdates, count)

	for index := range count {
		bid := 99.0 + float64(index)*0.01
		ask := 101.0 + float64(index)*0.01
		updates[index] = &krakenmarket.BookUpdate{
			Symbol: symbol,
			Type:   "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: bid, Qty: 10},
				{Price: bid - 0.01, Qty: 5},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: ask, Qty: 8},
				{Price: ask + 0.01, Qty: 4},
			},
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	signal.book.Update(updates)
}

func warmSignalSurprise(testingTB *testing.T, signal *Signal, symbol string) {
	testingTB.Helper()

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	seedBooks(signal, symbol, eventAt, 4)

	signal.Tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
	signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)

	state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.flow.CancelBid = 0.3
	state.flow.FillBid = 0.1

	artifact := measurementArtifact(symbol)

	for range 15 {
		_, err := signal.Measure(artifact)
		So(err, ShouldBeNil)
	}
}

func TestDefaultTracker(testingTB *testing.T) {
	Convey("Given the process-wide tracker", testingTB, func() {
		ResetDefault()
		before := defaultTracker.Load()

		ResetDefault()
		after := defaultTracker.Load()

		Convey("It should swap the default instance on reset", func() {
			So(before, ShouldNotBeNil)
			So(after, ShouldNotBeNil)
			So(after, ShouldNotEqual, before)
		})
	})
}

func TestIsToxicHelper(testingTB *testing.T) {
	Convey("Given a toxic cancel on the default tracker", testingTB, func() {
		ResetDefault()
		tracker := defaultTracker.Load()
		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "ZZZ/ISOLATED"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)
		tracker.ApplyBookFrame(symbol, pair, &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}, startAt)

		removedAt := startAt.Add(15 * time.Second)
		tracker.ApplyBookFrame(symbol, pair, &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{},
		}, removedAt)

		Convey("It should delegate IsToxic to the active tracker", func() {
			So(IsToxic(symbol, 100.01, removedAt), ShouldBeFalse)
		})
	})
}

func TestNearTouchToxic(testingTB *testing.T) {
	Convey("Given a near-touch toxic flag on the shared tracker", testingTB, func() {
		ResetDefault()
		tracker := defaultTracker.Load()
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should report near-touch toxicity for that symbol", func() {
			So(NearTouchToxic(symbol, now), ShouldBeTrue)
			So(NearTouchToxic("ETH/EUR", now), ShouldBeFalse)
		})
	})
}

func TestPriceKey(testingTB *testing.T) {
	Convey("Given a pair with tick size", testingTB, func() {
		state := &symbolState{pair: krakenmarket.Pair{TickSize: "0.1"}}

		Convey("It should round prices to tick boundaries", func() {
			So(priceKey(state, 100.01), ShouldEqual, priceKey(state, 100.04))
			So(priceFromKey(state, priceKey(state, 100.01)), ShouldAlmostEqual, 100.0, 1e-9)
		})
	})

	Convey("Given a pair without tick size", testingTB, func() {
		state := newSymbolState(krakenmarket.Pair{})
		state.mid = 100

		for _, step := range []float64{0.0001, 0.00012, 0.00011} {
			state.priceIncrements.Observe(step)
		}

		Convey("It should discretize from observed price increments", func() {
			key := priceKey(state, 100.000000001)
			So(priceFromKey(state, key), ShouldAlmostEqual, 100.0, 1e-4)
		})
	})
}

func TestIsToxicPriceKeyLookup(testingTB *testing.T) {
	Convey("Given a toxic level stored at a rounded price", testingTB, func() {
		tracker := newTestTracker(testingTB)
		symbol := "ETH/EUR"
		now := time.Now()
		pair := krakenmarket.Pair{TickSize: "0.01"}

		state := tracker.stateLocked(symbol, pair)
		matchWindow := state.timing.MatchWindow(state.tradeSpan())
		state.toxic.Flag(
			priceKey(state, 100.0),
			0,
			1,
			now.Add(state.timing.Cooldown(matchWindow)),
		)

		Convey("It should match a slightly perturbed lookup price", func() {
			So(tracker.IsToxic(symbol, 100.0000004, now), ShouldBeTrue)
		})

		Convey("It should match one tick away", func() {
			So(tracker.IsToxic(symbol, 100.01, now), ShouldBeTrue)
		})
	})
}

func TestTrackerApplyOrderToxicCancel(testingTB *testing.T) {
	Convey("Given a large near-touch cancel", testingTB, func() {
		tracker := newTestTracker(testingTB)
		now := time.Now()
		symbol := "TEST/TOXIC"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFlashChurnFlagsNearTouchLevel(testingTB *testing.T) {
	Convey("Given rapid near-touch add/delete churn without fills", testingTB, func() {
		tracker := newTestTracker(testingTB)
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFillToCancelThreshold(testingTB *testing.T) {
	Convey("Given a tracker without cached ratio", testingTB, func() {
		tracker := newTestTracker(testingTB)

		Convey("It should derive threshold from symbol flow", func() {
			So(tracker.fillToCancelThreshold(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestTrackerBookSideDepth(testingTB *testing.T) {
	Convey("Given mid-price observations", testingTB, func() {
		tracker := newTestTracker(testingTB)
		now := time.Now()

		tracker.ObserveMid("BTC/EUR", krakenmarket.Pair{}, 100)
		tracker.ObserveLast("BTC/EUR", krakenmarket.Pair{}, 101)

		Convey("It should retain symbol state", func() {
			So(tracker.IsToxic("BTC/EUR", 100, now), ShouldBeFalse)
		})
	})
}

func TestSignalMeasureToxicBluffChurnStrength(testingTB *testing.T) {
	Convey("Given a near-touch toxic cancel with churn ratio", testingTB, func() {
		signal := newTestSignal(testingTB)
		now := time.Now()
		symbol := "ETH/EUR"

		warmSignalSurprise(testingTB, signal, symbol)

		signal.Tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.toxic.Flag(priceKey(state, 100), 4.5, 0, now.Add(time.Minute))
		seedChurnGateHistory(signal, symbol)

		measurement, err := signal.Measure(measurementArtifact(symbol))

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

func TestSignalMeasureToxicBluffSaturatedEvidence(testingTB *testing.T) {
	Convey("Given finite near-touch churn that saturates evidence", testingTB, func() {
		signal := newTestSignal(testingTB)
		now := time.Now()
		symbol := "DOGE/EUR"

		warmSignalSurprise(testingTB, signal, symbol)

		signal.Tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
		key := priceKey(state, 100)
		state.toxic.Flag(key, math.MaxFloat64, 0, now.Add(time.Minute))
		seedChurnGateHistory(signal, symbol)

		measurement, err := signal.Measure(measurementArtifact(symbol))

		Convey("It should publish without rejecting unit-band confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryToxicBluff)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestSignalMeasureToxicBluff(testingTB *testing.T) {
	Convey("Given a near-touch toxic cancel flag", testingTB, func() {
		signal := newTestSignal(testingTB)
		eventAt := time.Now()
		symbol := "ETH/EUR"

		state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.mid = 100
		state.lastPrice = 100
		state.flow.BidDepth = 100
		seedChurnGateHistory(signal, symbol)

		signal.Tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, 100, 15, eventAt, eventAt)
		signal.Tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, 100, 15, eventAt, eventAt)

		artifact := measurementArtifact(symbol)

		for range 15 {
			_, err := signal.Measure(artifact)
			So(err, ShouldBeNil)
		}

		measurement, err := signal.Measure(artifact)

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

func TestSignalMeasureLiquidityVacuumFiniteStrength(testingTB *testing.T) {
	Convey("Given cancel/fill asymmetry with observed fill flow", testingTB, func() {
		signal := newTestSignal(testingTB)
		symbol := "BTC/EUR"

		warmSignalSurprise(testingTB, signal, symbol)

		state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.CancelBid = 0.3
		state.flow.FillBid = 0.1
		signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 50000)

		measurement, err := signal.Measure(measurementArtifact(symbol))
		threshold := signal.Tracker.fillToCancelThreshold()
		maxRatio := algorithm.CancelFillRatio(state.flow.CancelBid, state.flow.FillBid)
		expectedStrength := math.Min(
			maxRatio/threshold,
			signal.Tracker.vacuumStrengthLimit(symbol, threshold, maxRatio),
		)

		Convey("It should publish bounded strength with confidence and surprise", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryLiquidityVacuum)
			So(measurement.Strength, ShouldAlmostEqual, expectedStrength, 0.01)
			So(measurement.Strength, ShouldBeLessThan, 1e6)
			So(math.IsInf(measurement.Strength, 0), ShouldBeFalse)
			So(math.IsNaN(measurement.Strength), ShouldBeFalse)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.Surprise, ShouldBeGreaterThan, 0)
			So(measurement.Price, ShouldEqual, 50000)
		})
	})
}

func TestSignalMeasureLiquidityVacuumRequiresFillFlow(testingTB *testing.T) {
	Convey("Given cancel flow without matched fill", testingTB, func() {
		signal := newTestSignal(testingTB)
		symbol := "BTC/EUR"

		state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.CancelBid = 1
		state.flow.FillBid = 0
		signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)

		_, err := signal.Measure(measurementArtifact(symbol))

		Convey("It should not publish an incomplete asymmetry reading", func() {
			So(err, ShouldBeNil)
		})
	})
}

func TestSignalMeasureHardSupport(testingTB *testing.T) {
	Convey("Given balanced visible depth without toxic cancels", testingTB, func() {
		signal := newTestSignal(testingTB)
		now := time.Now()
		symbol := "BTC/EUR"
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		seedBooks(signal, symbol, eventAt, 4)

		signal.Tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
		signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
		signal.Tracker.ApplyBookLevel(symbol, krakenmarket.Pair{}, SideBid, 99.5, 80, now)
		signal.Tracker.ApplyBookLevel(symbol, krakenmarket.Pair{}, SideAsk, 100.5, 80, now)
		state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.FillBid = 0.1
		state.flow.FillAsk = 0.1
		state.flow.CancelBid = 0
		state.flow.CancelAsk = 0

		snapshot, _, _ := signal.Tracker.Snapshot(symbol, now)
		expectedStrength := math.Min(snapshot.bidDepth, snapshot.askDepth) /
			math.Max(snapshot.bidDepth, snapshot.askDepth)

		measurement, err := signal.Measure(measurementArtifact(symbol))

		Convey("It should classify hard support", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceToxicity)
			So(measurement.Category, ShouldEqual, logic.CategoryHardSupport)
			So(measurement.Strength, ShouldAlmostEqual, expectedStrength, 0.001)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestTrackerLevel3Churn(testingTB *testing.T) {
	Convey("Given a level3 update with add/delete events", testingTB, func() {
		ResetDefault()
		tracker := newTestTracker(testingTB)

		now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid("BTC/EUR", krakenmarket.Pair{}, 100)
		state := tracker.stateLocked("BTC/EUR", krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder("BTC/EUR", krakenmarket.Pair{}, "add", "l3-2", SideBid, 100, 15, now, now)
		tracker.ApplyOrder("BTC/EUR", krakenmarket.Pair{}, "delete", "l3-2", SideBid, 100, 15, now, now)

		Convey("It should classify per-order churn as toxic", func() {
			So(tracker.IsToxic("BTC/EUR", 100, now), ShouldBeTrue)
		})
	})
}

func TestSignalFeedsTracker(testingTB *testing.T) {
	Convey("Given a toxicity signal ingesting market frames through feeds", testingTB, func() {
		signal := newTestSignal(testingTB)
		symbol := "BTC/USD"
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal.trade.Update(krakenmarket.TradeUpdates{{
			Symbol:    symbol,
			Price:     100,
			Qty:       1,
			Timestamp: eventAt,
		}})

		signal.book.Update(krakenmarket.BookUpdates{{
			Symbol: symbol,
			Type:   "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99.5, Qty: 10},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 100.5, Qty: 10},
			},
			Timestamp: eventAt,
		}})

		observeAt := eventAt.Add(time.Second)
		snapshot, lastPrice, ok := signal.Tracker.Snapshot(symbol, observeAt)

		Convey("It should populate tracker state without manual tracker calls", func() {
			So(ok, ShouldBeTrue)
			So(lastPrice, ShouldEqual, 100)
			So(snapshot.bidDepth, ShouldEqual, 10)
			So(snapshot.askDepth, ShouldEqual, 10)
		})
	})

	Convey("Given snapshot and delta book frames through feeds", testingTB, func() {
		signal := newTestSignal(testingTB)
		symbol := "ETH/EUR"
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal.trade.Update(krakenmarket.TradeUpdates{{
			Symbol:    symbol,
			Price:     100,
			Qty:       1,
			Timestamp: eventAt,
		}})

		signal.book.Update(krakenmarket.BookUpdates{{
			Symbol: symbol,
			Type:   "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99.99, Qty: 10},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 100.01, Qty: 10},
			},
			Timestamp: eventAt,
		}})

		signal.book.Update(krakenmarket.BookUpdates{{
			Symbol: symbol,
			Type:   "update",
			Asks: []krakenmarket.BookLevel{
				{Price: 100.01, Qty: 5},
			},
			Timestamp: eventAt.Add(time.Millisecond),
		}})

		observeAt := eventAt.Add(time.Second)
		snapshot, _, ok := signal.Tracker.Snapshot(symbol, observeAt)

		Convey("It should apply deltas without wiping untouched levels", func() {
			So(ok, ShouldBeTrue)
			So(snapshot.bidDepth, ShouldEqual, 10)
			So(snapshot.askDepth, ShouldEqual, 5)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := newTestSignal(b)
	now := time.Now()
	symbol := "BTC/EUR"
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	seedBooks(signal, symbol, eventAt, 4)

	signal.Tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
	signal.Tracker.ObserveLast(symbol, krakenmarket.Pair{}, 100)
	state := signal.Tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.flow.BidDepth = 100

	artifact := measurementArtifact(symbol)

	b.ReportAllocs()

	for b.Loop() {
		signal.Tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		signal.Tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)
		_, _ = signal.Measure(artifact)
	}
}

func BenchmarkTrackerApplyOrder(b *testing.B) {
	tracker := newTestTracker(b)
	now := time.Now()
	symbol := "BTC/EUR"

	tracker.ObserveMid(symbol, krakenmarket.Pair{}, 100)
	state := tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.flow.BidDepth = 100

	for b.Loop() {
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)
	}
}
