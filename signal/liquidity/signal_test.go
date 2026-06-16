package liquidity

import (
	"context"
	"fmt"
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

func seedTickers(signal *Signal, symbol string, base time.Time, count int, last float64, volume float64) {
	updates := make(krakenmarket.TickerUpdates, count)

	for index := range count {
		price := last + float64(index)*0.01
		updates[index] = &krakenmarket.TickerUpdate{
			Symbol:    symbol,
			Last:      price,
			High:      price + 0.2,
			Low:       price - 0.2,
			Volume:    volume,
			VWAP:      price,
			Ask:       price + 0.1,
			Bid:       price - 0.1,
			AskQty:    1,
			BidQty:    1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	signal.ticker.Update(updates)
}

func TestNewSignal(testingTB *testing.T) {
	Convey("Given a liquidity signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		Convey("It should allocate feed handlers", func() {
			So(signal, ShouldNotBeNil)
			So(signal.ticker, ShouldNotBeNil)
			So(signal.features, ShouldNotBeNil)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a cross-section with deep and thin peers", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "COIN/EUR", 10, 1, 800, 1, eventAt)
		observeRow(signal, "PEER/EUR", 10, 1, 900, 1, eventAt)

		seedTickers(signal, "ALT/EUR", eventAt, 4, 10, 1200)

		measurement, err := signal.Measure(measurementArtifact("ALT/EUR"))

		Convey("It should publish robust liquidity", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
			So(measurement.Category, ShouldEqual, logic.CategoryRobustLiquidity)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a peak-scarcity symbol", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "DEEP/EUR", 10, 1, 1100, 1, eventAt)
		observeRow(signal, "MID/EUR", 10, 1, 950, 1, eventAt)

		seedTickers(signal, "THIN/EUR", eventAt, 4, 5, 50)

		measurement, err := signal.Measure(measurementArtifact("THIN/EUR"))

		Convey("It should classify extreme scarcity", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryExtremeScarcity)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given market-wide high absolute volume", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "DEEP/EUR", 10, 1, 600, 1, eventAt)
		observeRow(signal, "MID/EUR", 10, 1, 700, 1, eventAt)

		baselineErr := signal.Metrics.SeedBaseline(
			"THIN/EUR",
			eventAt.Add(-time.Hour),
			100,
		)

		So(baselineErr, ShouldBeNil)

		seedTickers(signal, "THIN/EUR", eventAt, 4, 5, 300)

		measurement, err := signal.Measure(measurementArtifact("THIN/EUR"))

		Convey("It should suppress relative scarcity above baseline", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryMedianDepth)
		})
	})

	Convey("Given fewer than two universe symbols", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedTickers(signal, "SOLO/EUR", eventAt, 4, 5, 100)

		measurement, err := signal.Measure(measurementArtifact("SOLO/EUR"))

		Convey("It should return best effort without a peer universe", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
			So(measurement.BestEffort, ShouldBeTrue)
		})
	})

	Convey("Given a book-triggered ticker without 24h summary", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "COIN/EUR", 10, 1, 800, 1, eventAt)
		observeRow(signal, "PEER/EUR", 10, 1, 900, 1, eventAt)

		signal.ticker.Update(krakenmarket.TickerUpdates{{
			Symbol:    "ALT/EUR",
			Bid:       9.9,
			Ask:       10.1,
			AskQty:    1,
			BidQty:    1,
			Last:      10,
			Volume:    1200,
			Timestamp: eventAt,
		}})

		measurement, err := signal.Measure(measurementArtifact("ALT/EUR"))

		Convey("It should publish without ResolveValue errors", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		qpool.NewQ[any](context.Background(), 2, 4, nil),
	)

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		observeRow(signal, symbol, 10, 1, float64(500+index*50), 1, eventAt)
	}

	seedTickers(signal, "SYM0/EUR", eventAt, 4, 10, 1200)

	artifact := measurementArtifact("SYM0/EUR")

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(artifact)

		if err != nil {
			b.Fatal(err)
		}
	}
}
