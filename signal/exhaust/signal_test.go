package exhaust

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
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

func thinningBook(symbol string, bidDepth float64, askPrice float64) *krakenmarket.BookUpdate {
	return &krakenmarket.BookUpdate{
		Symbol: symbol,
		Type:   "snapshot",
		Bids:   []krakenmarket.BookLevel{{Price: 100, Qty: bidDepth}},
		Asks:   []krakenmarket.BookLevel{{Price: askPrice, Qty: bidDepth * 0.5}},
	}
}

func seedBooks(signal *Signal, symbol string, base time.Time, books []*krakenmarket.BookUpdate) {
	updates := make(krakenmarket.BookUpdates, len(books))

	for index, book := range books {
		update := *book
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		updates[index] = &update
	}

	signal.book.Update(updates)
}

func TestNewSignal(testingTB *testing.T) {
	Convey("Given an exhaust signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		Convey("It should allocate feed handlers", func() {
			So(signal, ShouldNotBeNil)
			So(signal.book, ShouldNotBeNil)
			So(signal.trade, ShouldNotBeNil)
			So(signal.ticker, ShouldNotBeNil)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given deteriorating long-side book history", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		symbol := "ETH/EUR"

		for index := range 8 {
			depth := 20.0 - float64(index)*2
			askPrice := 101.0 + float64(index)*0.5
			book := thinningBook(symbol, depth, askPrice)
			book.Timestamp = eventAt.Add(time.Duration(index) * time.Millisecond)
			signal.book.Update(krakenmarket.BookUpdates{book})
		}

		seedBooks(signal, symbol, eventAt, []*krakenmarket.BookUpdate{
			thinningBook(symbol, 8, 104.5),
			thinningBook(symbol, 6, 105),
			thinningBook(symbol, 5, 105.5),
			thinningBook(symbol, 4, 105),
		})

		measurement, err := signal.Measure(measurementArtifact(symbol))

		Convey("It should publish an exhaustion reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceExhaustion)
			So(measurement.Symbol, ShouldEqual, symbol)
			So(measurement.Price, ShouldBeGreaterThan, 0)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given smoothed pressure fade on the long side", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		symbol := "BTC/EUR"
		state := signal.crossSection.ensure(symbol)

		for _, sign := range []float64{1, 1, 1, 1, 1, -1, -1, -1} {
			smoothed := emaObserve(state.pressureEMA, sign)
			pushRing(&state.pressures, smoothed, featureRingCapacity)
		}

		state.lastPrice = 100

		measurement, err := signal.Measure(measurementArtifact(symbol))

		Convey("It should classify thermal exhaustion from pressure fade", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryThermalExhaustion)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given insufficient decay features", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		_, err := signal.Measure(measurementArtifact("SOL/EUR"))

		Convey("It should withhold until history is populated", func() {
			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		qpool.NewQ[any](context.Background(), 2, 4, nil),
	)

	symbol := "ETH/EUR"
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for index := range 12 {
		depth := 20.0 - float64(index)
		book := thinningBook(symbol, depth, 101+float64(index)*0.25)
		book.Timestamp = eventAt.Add(time.Duration(index) * time.Millisecond)
		signal.book.Update(krakenmarket.BookUpdates{book})
	}

	seedBooks(signal, symbol, eventAt, []*krakenmarket.BookUpdate{
		thinningBook(symbol, 8, 104),
		thinningBook(symbol, 7, 104.5),
		thinningBook(symbol, 6, 104),
		thinningBook(symbol, 6, 104),
	})

	artifact := measurementArtifact(symbol)

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(artifact)
	}
}
