package cvd

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

func feedTrades(signal *Signal, updates krakenmarket.TradeUpdates) {
	signal.trade.Update(updates)
}

func TestNewSignal(testingTB *testing.T) {
	Convey("Given a CVD signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		Convey("It should allocate the trade feed handler", func() {
			So(signal, ShouldNotBeNil)
			So(signal.trade, ShouldNotBeNil)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given aggressive buy flow with rising price", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		for index := range 5 {
			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "BTC/EUR",
					Side:      "buy",
					Price:     100 + float64(index)*0.01,
					Qty:       1,
					Timestamp: eventAt.Add(time.Duration(index) * time.Second),
				},
			})
		}

		measurement, measureErr := signal.Measure(measurementArtifact("BTC/EUR"))

		Convey("It should classify aggressive drive", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCVD)
			So(measurement.Category, ShouldEqual, logic.CategoryAggressiveDrive)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0.55)
		})
	})

	Convey("Given aggressive buy flow with flat price", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		for index := range 4 {
			price := 50.0

			if index%2 == 1 {
				price = 50.001
			}

			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "ETH/EUR",
					Side:      "buy",
					Price:     price,
					Qty:       2,
					Timestamp: eventAt.Add(time.Duration(index) * time.Second),
				},
			})
		}

		measurement, measureErr := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should classify hidden absorption", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryHiddenAbsorption)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given balanced two-sided flow", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		trades := []struct {
			side  string
			price float64
		}{
			{"buy", 25},
			{"sell", 25.1},
			{"buy", 25},
			{"sell", 25.1},
		}

		for index, trade := range trades {
			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "SOL/EUR",
					Side:      trade.side,
					Price:     trade.price,
					Qty:       1,
					Timestamp: eventAt.Add(time.Duration(index) * time.Second),
				},
			})
		}

		measurement, measureErr := signal.Measure(measurementArtifact("SOL/EUR"))

		Convey("It should classify stochastic balance", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryStochasticBalance)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given divergent high-net flow", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		prices := []float64{100, 99.5, 99, 98.5}

		for index, price := range prices {
			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "DOGE/EUR",
					Side:      "buy",
					Price:     price,
					Qty:       5,
					Timestamp: eventAt.Add(time.Duration(index) * time.Second),
				},
			})
		}

		measurement, measureErr := signal.Measure(measurementArtifact("DOGE/EUR"))

		Convey("It should classify hidden absorption", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryHiddenAbsorption)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given insufficient trades", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		measurement, measureErr := signal.Measure(measurementArtifact("XRP/EUR"))

		Convey("It should withhold until trades are available", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceType(""))
		})
	})

	Convey("Given many trades with cumulative drift below sqrt-scaled threshold", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		for index := range 50 {
			price := 100.0

			if index%2 == 1 {
				price = 100.001
			}

			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "ETH/EUR",
					Side:      "buy",
					Price:     price,
					Qty:       2,
					Timestamp: eventAt.Add(time.Duration(index) * time.Second),
				},
			})
		}

		measurement, measureErr := signal.Measure(measurementArtifact("ETH/EUR"))

		Convey("It should still classify hidden absorption", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryHiddenAbsorption)
		})
	})
}

func TestSignalTradeUpdate(testingTB *testing.T) {
	Convey("Given a CVD signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		feedTrades(signal, krakenmarket.TradeUpdates{
			&krakenmarket.TradeUpdate{
				Symbol:    "BTC/USD",
				Price:     1.0,
				Qty:       0.1,
				Side:      "buy",
				Timestamp: time.Now(),
			},
			&krakenmarket.TradeUpdate{
				Symbol:    "BTC/USD",
				Price:     1.01,
				Qty:       0.1,
				Side:      "buy",
				Timestamp: time.Now().Add(time.Second),
			},
		})

		Convey("It should store the trade per symbol on the feed handler", func() {
			signal.trade.scope = "BTC/USD"

			buf := make([]byte, 4096)
			n, readErr := signal.trade.Read(buf)

			So(readErr, ShouldBeIn, nil, io.EOF)
			So(n, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		newTestPool(b),
	)

	baseTime := time.Now()

	for index := range 64 {
		feedTrades(signal, krakenmarket.TradeUpdates{
			&krakenmarket.TradeUpdate{
				Symbol:    "BTC/EUR",
				Side:      "buy",
				Price:     100 + float64(index)*0.01,
				Qty:       1,
				Timestamp: baseTime.Add(time.Duration(index) * time.Second),
			},
		})
	}

	artifact := measurementArtifact("BTC/EUR")

	b.ReportAllocs()

	for b.Loop() {
		_, _ = signal.Measure(artifact)
	}
}
