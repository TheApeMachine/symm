package correlation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func setCorrelationTestConfig() {
	viper.Set("signals.correlation.measurements_capacity", 4)
}

func seedTrades(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	for index := range count {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		})
	}
}

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
	setCorrelationTestConfig()

	Convey("Given a correlated cross-section", t, func() {
		useCrossSection(t)

		symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"}
		prices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
			"SOL/EUR": 25,
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		measureAt := eventAt.Add(time.Second)
		observePrices(symbols, prices, []float64{1.005, 1.01, 1.015, 1.02, 1.025}, eventAt)

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		seedTrades(signal, "BTC/EUR", eventAt, 4, prices["BTC/EUR"])

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify systemic herd", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCorrelation)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a decoupled mover", t, func() {
		useCrossSection(t)

		herdPrices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		measureAt := eventAt.Add(time.Second)
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

		seedTrades(signal, "ALT/EUR", eventAt, 4, 14)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify decoupled alpha", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDecoupledAlpha)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
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

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		measureAt := eventAt.Add(time.Second)
		observeRow("BTC/EUR", 100, 1, 100000, 1, eventAt)
		seedTrades(signal, "BTC/EUR", eventAt, 1, 100)

		_, err = signal.Measure(nil, measureAt)

		Convey("It should withhold until the window is full", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setCorrelationTestConfig()

	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     4,
		BreadthHist: 64,
	})

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for step := range 8 {
		observeRow("BTC/EUR", 100*math.Pow(1.01, float64(step)), 0.01, 100000, 1, eventAt)
		observeRow("ETH/EUR", 50*math.Pow(1.01, float64(step)), 0.01, 50000, 1, eventAt)
		observeRow("SOL/EUR", 25*math.Pow(1.01, float64(step)), 0.01, 25000, 1, eventAt)
	}

	signal := NewSignal(
		"BTC/EUR",
		logic.NewEntity(logic.EntityTrade),
	)

	seedTrades(signal, "BTC/EUR", eventAt, 4, 100*math.Pow(1.01, 8))

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(nil, eventAt.Add(time.Second))
	}
}
