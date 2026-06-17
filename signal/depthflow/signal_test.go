package depthflow

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

func observeRow(
	signal *Signal,
	symbol string,
	price, value, volume, pressure float64,
	eventAt time.Time,
) {
	row, err := krakenmarket.NewSymbolRow(symbol, price, value, volume, pressure, eventAt)

	if err != nil {
		panic(err)
	}

	if err := signal.CrossSection.Observe(row); err != nil {
		panic(err)
	}
}

func seedBooks(
	signal *Signal,
	symbol string,
	base time.Time,
	frames []*krakenmarket.BookUpdate,
) {
	updates := make(krakenmarket.BookUpdates, len(frames))

	for index, frame := range frames {
		update := *frame
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		updates[index] = &update
	}

	signal.book.Update(updates)
}

func bidHeavyFrames() []*krakenmarket.BookUpdate {
	return []*krakenmarket.BookUpdate{
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99, Qty: 10},
				{Price: 98, Qty: 20},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 1},
				{Price: 102, Qty: 1},
			},
		},
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99, Qty: 10},
				{Price: 98, Qty: 20},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 1},
				{Price: 102, Qty: 1},
			},
		},
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99, Qty: 12},
				{Price: 98, Qty: 22},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 1},
				{Price: 102, Qty: 1},
			},
		},
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99, Qty: 14},
				{Price: 98, Qty: 24},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 1},
				{Price: 102, Qty: 1},
			},
		},
	}
}

func spoofFrames() []*krakenmarket.BookUpdate {
	return []*krakenmarket.BookUpdate{
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 1},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 2},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 2},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
		{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 2},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		},
	}
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a bid-heavy book", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "BTC/EUR", 100, 1, 10000, 0.8, eventAt)
		seedBooks(signal, "BTC/EUR", eventAt, bidHeavyFrames())

		measurement, err := signal.Measure(measurementArtifact("BTC/EUR"))

		Convey("It should classify loaded imbalance", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid wall with bearish touch", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "ETH/EUR", 50, 1, 10000, -0.5, eventAt)
		seedBooks(signal, "ETH/EUR", eventAt, spoofFrames())

		measurement, err := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should classify spoof trap", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategorySpoofTrap)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given trade pressure update", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		updates := make(krakenmarket.TradeUpdates, 2)

		for index, price := range []float64{25, 25.1} {
			updates[index] = &krakenmarket.TradeUpdate{
				Symbol:    "SOL/EUR",
				Side:      "buy",
				Price:     price,
				Qty:       float64(index + 2),
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			}
		}

		signal.trade.Update(updates)

		_, err := signal.Measure(measurementArtifact("SOL/EUR"))

		Convey("It should observe trade pressure while awaiting book", func() {
			So(err, ShouldBeNil)

			pressure := signal.CrossSection.Pressure("SOL/EUR")
			So(pressure, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureBeforeUniverseEntry(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a book update before the symbol enters the cross-section", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedBooks(signal, "ZBCN/USD", eventAt, bidHeavyFrames())

		measurement, err := signal.Measure(measurementArtifact("ZBCN/USD"))

		Convey("It should measure without halting on missing trade pressure", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
			So(measurement.Symbol, ShouldEqual, "ZBCN/USD")
		})
	})
}

func TestSignalMeasureWithholdsUntilBookHistoryReady(testingTB *testing.T) {
	Convey("Given one book update in the ring", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		signal.book.Update(krakenmarket.BookUpdates{{
			Symbol:    "BTC/EUR",
			Timestamp: base,
			Type:      "snapshot",
			Bids:      []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
			Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
		}})

		measurement, err := signal.Measure(measurementArtifact("BTC/EUR"))

		Convey("It should withhold until enough ring history exists", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceType(""))
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		qpool.NewQ[any](context.Background(), 2, 4, nil),
	)

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	observeRow(signal, "BTC/EUR", 100, 1, 10000, 0.8, eventAt)
	seedBooks(signal, "BTC/EUR", eventAt, bidHeavyFrames())

	artifact := measurementArtifact("BTC/EUR")

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(artifact)

		if err != nil {
			b.Fatal(err)
		}
	}
}
