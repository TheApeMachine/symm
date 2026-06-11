package correlation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func initCrossSection(cfg *market.CrossSectionConfig) {
	section, err := market.NewCrossSection(cfg)
	if err != nil {
		panic(err)
	}

	crossSection = section
}

func useCrossSection(t *testing.T) {
	t.Helper()

	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})
}

func observeRow(symbol string, price, value, volume, pressure float64, eventAt time.Time) {
	row, err := krakenmarket.NewSymbolRow(symbol, price, value, volume, pressure, eventAt)

	if err != nil {
		panic(err)
	}

	if err := crossSection.Observe(row); err != nil {
		panic(err)
	}
}

func observePrices(symbols []string, prices map[string]float64, shocks []float64, eventAt time.Time) {
	for _, shock := range shocks {
		for _, symbol := range symbols {
			prices[symbol] *= shock
			observeRow(symbol, prices[symbol], shock-1, prices[symbol]*1000, 1, eventAt)
		}
	}
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a correlated cross-section", t, func() {
		useCrossSection(t)

		symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"}
		prices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
			"SOL/EUR": 25,
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		observePrices(symbols, prices, []float64{1.005, 1.01, 1.015, 1.02, 1.025}, eventAt)

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "BTC/EUR",
			Price:  prices["BTC/EUR"],
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, eventAt)

		Convey("It should classify systemic herd", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCorrelation)
			So(measurement.Category, ShouldEqual, logic.CategorySystemicHerd)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a decoupled mover", t, func() {
		useCrossSection(t)

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
			observeRow("BTC/EUR", herdPrices["BTC/EUR"], shock-1, herdPrices["BTC/EUR"]*1000, 1, eventAt)
			observeRow("ETH/EUR", herdPrices["ETH/EUR"], shock-1, herdPrices["ETH/EUR"]*1000, 1, eventAt)
			observeRow("ALT/EUR", altPrices[index], 1, altPrices[index]*1000, 1, eventAt)
		}

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "ALT/EUR",
			Price:  14,
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, eventAt)

		Convey("It should classify decoupled alpha", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDecoupledAlpha)
		})
	})

	Convey("Given insufficient warmup", t, func() {
		useCrossSection(t)

		section, err := market.NewCrossSection(&market.CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   16,
			MinBars:     8,
			BreadthHist: 16,
		})

		if err != nil {
			t.Fatal(err)
		}

		crossSection = section

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		observeRow("BTC/EUR", 100, 1, 100000, 1, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "BTC/EUR",
			Price:  100,
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should withhold until the window is full", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryTypeNone)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     4,
		BreadthHist: 64,
	})

	for step := range 8 {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		observeRow("BTC/EUR", 100*math.Pow(1.01, float64(step)), 0.01, 100000, 1, eventAt)
		observeRow("ETH/EUR", 50*math.Pow(1.01, float64(step)), 0.01, 50000, 1, eventAt)
		observeRow("SOL/EUR", 25*math.Pow(1.01, float64(step)), 0.01, 25000, 1, eventAt)
	}

	signal := NewSignal(
		"BTC/EUR",
		logic.NewEntity(logic.EntityTrade),
	)

	signal.Record(&krakenmarket.TradeUpdate{
		Symbol: "BTC/EUR",
		Price:  100 * math.Pow(1.01, 8),
		Qty:    1,
	})

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	}
}
