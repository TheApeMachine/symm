package sentiment

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

func seedTickers(signal *Signal, symbol string, base time.Time, count int, last float64, changePct float64) {
	updates := make(krakenmarket.TickerUpdates, count)

	for index := range count {
		price := last + float64(index)*0.01
		updates[index] = &krakenmarket.TickerUpdate{
			Symbol:    symbol,
			Last:      price,
			High:      price + 0.2,
			Low:       price - 0.2,
			Volume:    1000,
			VWAP:      price,
			ChangePct: changePct,
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
	Convey("Given a sentiment signal", testingTB, func() {
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

	Convey("Given a bullish cross-section on trades", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		universe := []struct {
			symbol string
			value  float64
		}{
			{"A/EUR", 2.0},
			{"B/EUR", 2.0},
			{"C/EUR", 2.0},
		}

		for _, entry := range universe {
			observeRow(signal, entry.symbol, 100, entry.value, 1000, 1, eventAt)
		}

		seedTickers(signal, "A/EUR", eventAt, 4, 104, 2.0)

		measurement, err := signal.Measure(measurementArtifact("A/EUR"))

		Convey("It should classify a risk-on surge", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
			So(measurement.Category, ShouldEqual, logic.CategoryRiskOnSurge)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldNotEqual, time.Time{})
		})
	})

	Convey("Given a weak cross-section with a local leader", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "LEAD/EUR", 100, 4.0, 1000, 1, eventAt)
		observeRow(signal, "LAG/EUR", 100, -2.0, 1000, 1, eventAt)
		observeRow(signal, "FLAT/EUR", 100, -1.0, 1000, 1, eventAt)

		seedTickers(signal, "LEAD/EUR", eventAt, 4, 104, 4.0)

		measurement, err := signal.Measure(measurementArtifact("LEAD/EUR"))

		Convey("It should classify a divergent move", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDivergentMove)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a sparse cross-section at startup", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedTickers(signal, "NEW/EUR", eventAt, 4, 104, 2.0)

		measurement, err := signal.Measure(measurementArtifact("NEW/EUR"))

		Convey("It should not error before the universe fills in", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
		})
	})

	Convey("Given future-dated rows in the cross-section", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		measureAt := eventAt.Add(time.Second)
		observeRow(signal, "A/EUR", 100, 2.0, 1000, 1, measureAt.Add(time.Hour))
		observeRow(signal, "B/EUR", 100, 2.0, 1000, 1, eventAt)

		seedTickers(signal, "A/EUR", eventAt, 4, 104, 2.0)

		measurement, err := signal.Measure(measurementArtifact("A/EUR"))

		Convey("It should not error on non-finite breadth inputs", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
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
		observeRow(signal, symbol, 100, float64(index%5)+0.5, 1000, 1, eventAt)
	}

	seedTickers(signal, "SYM0/EUR", eventAt, 8, 100, 1.0)

	artifact := measurementArtifact("SYM0/EUR")

	b.ResetTimer()

	for b.Loop() {
		_, err := signal.Measure(artifact)

		if err != nil {
			b.Fatal(err)
		}
	}
}
