package hawkes

import (
	"context"
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
)

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

func tradeBurst(symbol string, base time.Time, count int) krakenmarket.TradeUpdates {
	updates := make(krakenmarket.TradeUpdates, count)

	for index := range count {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		updates[index] = &krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Side:      side,
			Price:     100 + float64(index)*0.01,
			Qty:       1.5 + float64(index%5)*0.1,
			Timestamp: base.Add(time.Duration(index) * 100 * time.Millisecond),
		}
	}

	return updates
}

func feedTrades(signal *Signal, updates krakenmarket.TradeUpdates) {
	signal.trade.Update(updates)
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a Hawkes signal with a clustered trade burst", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var (
			measurement logic.Measurement
			measureErr  error
		)

		for range 4 {
			feedTrades(signal, tradeBurst("ALT/EUR", base, 128))
			measurement, measureErr = signal.Measure(measurementArtifact("ALT/EUR"))
		}

		Convey("It should produce a publishable thermal reading", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceHawkes)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalTradeUpdate(testingTB *testing.T) {
	Convey("Given a Hawkes signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		feedTrades(signal, tradeBurst("BTC/USD", time.Now(), 8))
		signal.trade.scope = "BTC/USD"

		buf := make([]byte, 4096)
		n, readErr := signal.trade.Read(buf)

		Convey("It should emit an excitation payload", func() {
			So(readErr, ShouldBeIn, nil, io.EOF)
			So(n, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasurePipelineFrameSize(testingTB *testing.T) {
	Convey("Given a saturated Hawkes trade ring", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		base := time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC)
		capacity := feed.FeedRingCapacity()

		feedTrades(signal, tradeBurst("BTC/EUR", base, capacity+32))
		signal.trade.scope = "BTC/EUR"

		frame := make([]byte, feed.FeatureFrameSize)
		readCount, readErr := signal.trade.Read(frame)

		So(readErr, ShouldNotEqual, io.ErrShortBuffer)
		So(readCount, ShouldBeGreaterThan, 0)

		_, writeErr := signal.algo.Write(frame[:readCount])

		So(writeErr, ShouldBeNil)

		Convey("When the composed pipeline is read", func() {
			outCount, pipelineErr := signal.algo.Read(frame)

			Convey("It should fit the feature frame without short buffer", func() {
				So(pipelineErr, ShouldNotEqual, io.ErrShortBuffer)
				So(outCount, ShouldBeGreaterThan, 0)
				So(outCount, ShouldBeLessThanOrEqualTo, len(frame))
			})
		})
	})
}

func TestSignalMeasurePipelineFrameSizeRepeated(testingTB *testing.T) {
	Convey("Given a saturated Hawkes trade ring measured repeatedly", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		base := time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC)
		capacity := feed.FeedRingCapacity()

		feedTrades(signal, tradeBurst("BTC/EUR", base, capacity+32))

		Convey("When Measure is called many times", func() {
			artifact := measurementArtifact("BTC/EUR")

			for range 200 {
				_, measureErr := signal.Measure(artifact)

				So(measureErr, ShouldBeNil)
			}
		})
	})
}

func TestTradeReadFitsFeatureFrame(testingTB *testing.T) {
	Convey("Given a full Hawkes trade ring", testingTB, func() {
		tradeFeed := NewTrade(context.Background())
		base := time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC)
		capacity := feed.FeedRingCapacity()

		tradeFeed.Update(tradeBurst("ETH/USD", base, capacity+32))
		tradeFeed.scope = "ETH/USD"

		frame := make([]byte, feed.FeatureFrameSize)

		Convey("When Read is called", func() {
			readCount, readErr := tradeFeed.Read(frame)

			Convey("It should fit the feature frame without short buffer", func() {
				So(readErr, ShouldNotEqual, io.ErrShortBuffer)
				So(readCount, ShouldBeGreaterThan, 0)
				So(readCount, ShouldBeLessThanOrEqualTo, len(frame))
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		newTestPool(b),
	)

	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	feedTrades(signal, tradeBurst("BTC/EUR", base, 128))
	artifact := measurementArtifact("BTC/EUR")

	b.ReportAllocs()

	for b.Loop() {
		_, _ = signal.Measure(artifact)
	}
}
