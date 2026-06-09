package correlation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func testCrossSection(minBars, capacity int) *crossSection {
	viper.Set("signals.trade_match_window", time.Minute)

	crossSection, err := newCrossSection(minBars, capacity)

	if err != nil {
		panic(err)
	}

	return crossSection
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a correlated cross-section", t, func() {
		crossSection := testCrossSection(4, 16)

		symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"}
		prices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
			"SOL/EUR": 25,
		}

		shocks := []float64{1.005, 1.01, 1.015, 1.02, 1.025}

		for _, shock := range shocks {
			for _, symbol := range symbols {
				prices[symbol] *= shock
				crossSection.publishPrice(symbol, prices[symbol], time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			}
		}

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			crossSection,
			2.0,
			0.5,
		)

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "BTC/EUR",
			Price:  prices["BTC/EUR"],
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify systemic herd", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCorrelation)
			So(measurement.Category, ShouldEqual, logic.CategorySystemicHerd)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a decoupled mover", t, func() {
		crossSection := testCrossSection(4, 16)

		herdPrices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
		}

		shocks := []float64{1.005, 1.01, 1.015, 1.02, 1.025}
		altPrices := []float64{10.2, 9.8, 10.5, 9.5, 14.0}

		for index, shock := range shocks {
			herdPrices["BTC/EUR"] *= shock
			herdPrices["ETH/EUR"] *= shock
			crossSection.publishPrice("BTC/EUR", herdPrices["BTC/EUR"], time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			crossSection.publishPrice("ETH/EUR", herdPrices["ETH/EUR"], time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			crossSection.publishPrice("ALT/EUR", altPrices[index], time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		}

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			crossSection,
			2.0,
			0.5,
		)

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "ALT/EUR",
			Price:  14,
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify decoupled alpha", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDecoupledAlpha)
		})
	})

	Convey("Given insufficient warmup", t, func() {
		crossSection := testCrossSection(8, 16)

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			crossSection,
			2.0,
			0.5,
		)

		crossSection.publishPrice("BTC/EUR", 100, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
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
	crossSection := testCrossSection(4, 64)

	for step := 0; step < 8; step++ {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		crossSection.publishPrice("BTC/EUR", 100*math.Pow(1.01, float64(step)), eventAt)
		crossSection.publishPrice("ETH/EUR", 50*math.Pow(1.01, float64(step)), eventAt)
		crossSection.publishPrice("SOL/EUR", 25*math.Pow(1.01, float64(step)), eventAt)
	}

	signal := NewSignal(
		"BTC/EUR",
		logic.NewEntity(logic.EntityTrade),
		64,
		crossSection,
		2.0,
		0.5,
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
