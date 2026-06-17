package correlation

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	marketsection "github.com/theapemachine/symm/market"
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

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
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

func observePrices(
	signal *Signal,
	symbols []string,
	prices map[string]float64,
	shocks []float64,
	eventAt time.Time,
) {
	for _, shock := range shocks {
		for _, symbol := range symbols {
			prices[symbol] *= shock
			observeRow(signal, symbol, prices[symbol], shock-1, prices[symbol]*1000, 1, eventAt)
		}
	}
}

func seedTrades(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	updates := make(krakenmarket.TradeUpdates, count)

	for index := range count {
		updates[index] = &krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	signal.trade.Update(feed.TradeFeedArtifact(updates))
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a correlated cross-section", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"}
		prices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
			"SOL/EUR": 25,
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		observePrices(signal, symbols, prices, []float64{1.005, 1.01, 1.015, 1.02, 1.025}, eventAt)
		seedTrades(signal, "BTC/EUR", eventAt, 4, prices["BTC/EUR"])

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify systemic herd", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a decoupled mover", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		herdPrices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		shocks := []float64{1.005, 1.01, 1.015, 1.02, 1.025}
		altPrices := []float64{10.2, 9.8, 10.5, 9.5, 14.0}

		for index, shock := range shocks {
			herdPrices["BTC/EUR"] *= shock
			herdPrices["ETH/EUR"] *= shock
			observeRow(signal, "BTC/EUR", herdPrices["BTC/EUR"], shock-1, herdPrices["BTC/EUR"]*1000, 1, eventAt)
			observeRow(signal, "ETH/EUR", herdPrices["ETH/EUR"], shock-1, herdPrices["ETH/EUR"]*1000, 1, eventAt)
			observeRow(signal, "ALT/EUR", altPrices[index], 1, altPrices[index]*1000, 1, eventAt)
		}

		seedTrades(signal, "ALT/EUR", eventAt, 4, 14)

		result := signal.Measure(measurementQuery("ALT/EUR"))

		Convey("It should classify decoupled alpha", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given insufficient warmup", testingTB, func() {
		crossSection, err := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   16,
			MinBars:     8,
			BreadthHist: 16,
		})
		So(err, ShouldBeNil)

		signal := NewSignal(context.Background(), newTestPool(testingTB))
		signal.CrossSection = crossSection

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		observeRow(signal, "BTC/EUR", 100, 1, 100000, 1, eventAt)
		seedTrades(signal, "BTC/EUR", eventAt, 1, 100)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should return nil before warmup completes", func() {
			So(result, ShouldBeNil)
		})
	})

	Convey("Given a book-triggered ticker", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		observePrices(
			signal,
			[]string{"BTC/EUR", "ETH/EUR"},
			map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50},
			[]float64{1.005, 1.01, 1.015, 1.02, 1.025},
			eventAt,
		)

		signal.ticker.Update(feed.TickerFeedArtifact(krakenmarket.TickerUpdates{{
			Symbol:    "BTC/EUR",
			Bid:       99.9,
			Ask:       100.1,
			AskQty:    1,
			BidQty:    1,
			Last:      100,
			Volume:    1000,
			Timestamp: eventAt,
		}}))

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should publish without ticker value errors", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	query := measurementQuery("BTC/EUR")
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()

	for b.Loop() {
		signal := NewSignal(context.Background(), pool)

		for step := range 8 {
			observeRow(signal, "BTC/EUR", 100*math.Pow(1.01, float64(step)), 0.01, 100000, 1, eventAt)
			observeRow(signal, "ETH/EUR", 50*math.Pow(1.01, float64(step)), 0.01, 50000, 1, eventAt)
			observeRow(signal, "SOL/EUR", 25*math.Pow(1.01, float64(step)), 0.01, 25000, 1, eventAt)
		}

		seedTrades(signal, "BTC/EUR", eventAt, 4, 100*math.Pow(1.01, 8))

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}
	}
}
