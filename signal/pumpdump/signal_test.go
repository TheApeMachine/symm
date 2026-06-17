package pumpdump

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
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

func seedTrades(signal *Signal, symbol string, base time.Time, trades []*krakenmarket.TradeUpdate) {
	updates := make(krakenmarket.TradeUpdates, len(trades))

	for index, trade := range trades {
		update := *trade
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		updates[index] = &update
	}

	signal.trade.Update(updates)
}

func seedBooks(signal *Signal, symbol string, base time.Time, frames []*krakenmarket.BookUpdate) {
	updates := make(krakenmarket.BookUpdates, len(frames))

	for index, frame := range frames {
		update := *frame
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		updates[index] = &update
	}

	signal.book.Update(updates)
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given trade samples with a volume spike", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedTrades(signal, "ETH/EUR", eventAt, []*krakenmarket.TradeUpdate{
			{Price: 100, Qty: 1},
			{Price: 101, Qty: 1},
			{Price: 102, Qty: 1},
			{Price: 103, Qty: 1},
			{Price: 104, Qty: 20},
			{Price: 105, Qty: 20},
			{Price: 106, Qty: 20},
		})

		measurement, err := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should classify without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Source, ShouldEqual, logic.SourcePumpDump)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given book frames with valid touch spread", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedBooks(signal, "BTC/EUR", eventAt, []*krakenmarket.BookUpdate{
			{
				Bids: []krakenmarket.BookLevel{{Price: 99, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 101, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 100.2, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 6}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.05, Qty: 6}},
			},
		})

		measurement, err := signal.Measure(measurementArtifact("BTC/EUR"))

		Convey("It should measure spread compression without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "BTC/EUR")
			So(measurement.Spread, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given folded book snapshots with tightening spread", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedBooks(signal, "ETH/EUR", eventAt, []*krakenmarket.BookUpdate{
			{
				Bids: []krakenmarket.BookLevel{{Price: 99, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 101, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 100.2, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 6}},
			},
		})

		measurement, err := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should measure spread compression without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Spread, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a long silence before a thin print", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		signal.trade.Update(krakenmarket.TradeUpdates{
			{Symbol: "ALT/EUR", Price: 1, Qty: 1000, Timestamp: eventAt},
			{Symbol: "ALT/EUR", Price: 1, Qty: 1000, Timestamp: eventAt.Add(time.Second)},
			{Symbol: "ALT/EUR", Price: 1, Qty: 5, Timestamp: eventAt.Add(10 * time.Minute)},
		})

		Convey("It should decay stale volume context instead of phantom-spiking", func() {
			So(signal.crossSection.LastRvol("ALT/EUR"), ShouldBeLessThan, 0.1)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		qpool.NewQ[any](context.Background(), 2, 4, nil),
	)

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for index := range 32 {
		signal.trade.Update(krakenmarket.TradeUpdates{{
			Symbol:    "ETH/EUR",
			Price:     100 + float64(index),
			Qty:       float64(index%5) + 1,
			Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
		}})
	}

	artifact := measurementArtifact("ETH/EUR")

	b.ResetTimer()

	for b.Loop() {
		_, err := signal.Measure(artifact)

		if err != nil {
			b.Fatal(err)
		}
	}
}
