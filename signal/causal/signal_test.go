package causal

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

func feedTrades(signal *Signal, updates krakenmarket.TradeUpdates) {
	signal.trade.Update(&updates)
}

func TestNewSignal(testingTB *testing.T) {
	Convey("Given a trade entity", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
			&logic.Entity{Type: logic.EntityTypeTrade},
		)

		Convey("It should allocate feed handlers", func() {
			So(signal, ShouldNotBeNil)
			So(signal.trade, ShouldNotBeNil)
			So(signal.ticker, ShouldNotBeNil)
			So(signal.book, ShouldNotBeNil)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a causal signal fed with trades", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
			&logic.Entity{Type: logic.EntityTypeTrade},
		)

		baseTime := time.Now()
		price := 100.0

		for index := range 64 {
			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "BTC/USD",
					Side:      "buy",
					Price:     price + float64(index)*0.5,
					Qty:       0.1 + float64(index)*0.01,
					Timestamp: baseTime.Add(time.Duration(index) * time.Second),
				},
			})
		}

		signal.book.Update(krakenmarket.BookUpdates{
			&krakenmarket.BookUpdate{
				Symbol: "BTC/USD",
				Bids: []krakenmarket.BookLevel{
					{Price: price, Qty: 10},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: price + 0.1, Qty: 10},
				},
				Timestamp: baseTime,
			},
		})

		signal.trade.ObserveLiquidity("BTC/USD", signal.book.LatestLiquidity("BTC/USD"))

		measurement, measureErr := signal.Measure(measurementArtifact("BTC/USD"))

		Convey("It should derive category through inline nomagique.Number", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCausal)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.Price, ShouldBeGreaterThan, 0)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldBeIn,
				logic.CategoryEndogenousAlpha,
				logic.CategorySystemicBeta,
				logic.CategoryLiquidityShock,
				logic.CategoryCausalNoise,
			)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalTradeUpdate(testingTB *testing.T) {
	Convey("Given a causal signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
			&logic.Entity{Type: logic.EntityTypeTrade},
		)

		feedTrades(signal, krakenmarket.TradeUpdates{
			&krakenmarket.TradeUpdate{
				Symbol: "BTC/USD",
				Price:  1.0,
				Qty:    0.1,
				Side:   "buy",
			},
		})

		Convey("It should store the trade per symbol on the feed handler", func() {
			updates := signal.trade.Read("BTC/USD")

			So(updates, ShouldNotBeNil)
			So(len(updates), ShouldEqual, 1)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		newTestPool(b),
		&logic.Entity{Type: logic.EntityTypeTrade},
	)

	baseTime := time.Now()
	price := 100.0

	for index := range 16 {
		feedTrades(signal, krakenmarket.TradeUpdates{
			&krakenmarket.TradeUpdate{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     price + float64(index)*0.5,
				Qty:       0.1,
				Timestamp: baseTime.Add(time.Duration(index) * time.Second),
			},
		})
	}

	artifact := measurementArtifact("BTC/USD")

	b.ReportAllocs()

	for b.Loop() {
		_, _ = signal.Measure(artifact)
	}
}
