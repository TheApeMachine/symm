package causal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
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

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
}

func feedTrades(signal *Signal, updates krakenmarket.TradeUpdates) {
	signal.trade.Update(feed.TradeFeedArtifact(updates))
}

func TestSignalMeasureWithholdsUntilLadderSettles(testingTB *testing.T) {
	Convey("Given a causal signal with insufficient ladder history", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		feedTrades(signal, krakenmarket.TradeUpdates{
			&krakenmarket.TradeUpdate{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     100,
				Qty:       0.1,
				Timestamp: time.Now(),
			},
		})
		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should withhold the measurement", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a causal signal fed with trades", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		baseTime := time.Now()
		price := 100.0

		for index := range 64 {
			wobble := float64((index*7)%13) * 0.5
			side := "buy"

			if index%3 == 0 {
				side = "sell"
			}

			feedTrades(signal, krakenmarket.TradeUpdates{
				&krakenmarket.TradeUpdate{
					Symbol:    "BTC/USD",
					Side:      side,
					Price:     price + wobble,
					Qty:       0.1 + wobble*0.04,
					Timestamp: baseTime.Add(time.Duration(index) * time.Second),
				},
			})
		}

		signal.book.Update(feed.BookFeedArtifact(krakenmarket.BookUpdates{
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
		}))

		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should derive category through inline nomagique.Number", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/USD")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalTradeUpdate(testingTB *testing.T) {
	Convey("Given a causal signal", testingTB, func() {
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
		})

		Convey("It should store the trade per symbol on the feed handler", func() {
			signal.trade.Scope = "BTC/USD"

			buf := make([]byte, 4096)
			n, _ := signal.trade.Read(buf)

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

	query := measurementQuery("BTC/USD")

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.Measure(query)
	}
}
