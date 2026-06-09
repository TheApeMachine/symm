package sentiment

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
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
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 8,
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a bullish cross-section on trades", t, func() {
		useCrossSection(t)
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		universe := []struct {
			symbol string
			prices []float64
		}{
			{"A/EUR", []float64{100, 102}},
			{"B/EUR", []float64{100, 103}},
			{"C/EUR", []float64{100, 101.5}},
		}

		for _, entry := range universe {
			entrySignal := NewSignal(
				entry.symbol,
				logic.NewEntity(logic.EntityTrade),
			)

			for _, price := range entry.prices {
				entrySignal.Record(&krakenmarket.TradeUpdate{
					Symbol: entry.symbol,
					Price:  price,
					Qty:    1,
				})
			}

			crossSection.Observe(&krakenmarket.Symbol{
				Name: entry.symbol, Value: 2.0, Updated: eventAt,
			})
		}

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "A/EUR",
			Price:  102,
			Qty:    1,
		})
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "A/EUR",
			Price:  104,
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, eventAt)

		Convey("It should classify a risk-on surge", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
			So(measurement.Category, ShouldEqual, logic.CategoryRiskOnSurge)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldEqual, eventAt)
		})
	})

	Convey("Given a weak cross-section with a local leader", t, func() {
		useCrossSection(t)
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal := NewSignal(
			"LEAD/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		crossSection.Observe(&krakenmarket.Symbol{
			Name: "LEAD/EUR", Value: 4.0, Updated: eventAt,
		})
		crossSection.Observe(&krakenmarket.Symbol{
			Name: "LAG/EUR", Value: -2.0, Updated: eventAt,
		})
		crossSection.Observe(&krakenmarket.Symbol{
			Name: "FLAT/EUR", Value: -1.0, Updated: eventAt,
		})

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "LEAD/EUR",
			Price:  100,
			Qty:    1,
		})
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "LEAD/EUR",
			Price:  104,
			Qty:    1,
		})

		measurement, err := signal.Measure(nil, eventAt)

		Convey("It should classify a divergent move", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDivergentMove)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		useCrossSection(t)
		viper.Set("signals.sentiment.surge_threshold", 2.0)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
		)
		feedback := market.NewFeedback("A/EUR", 0.5, 1.0, 0.2, 3)

		_, err := signal.Measure(feedback, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 2.0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "A/EUR"})

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 32,
	})

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		crossSection.Observe(&krakenmarket.Symbol{
			Name: symbol, Value: float64(index%5) + 0.5, Updated: eventAt,
		})
	}

	signal := NewSignal(
		"SYM0/EUR",
		logic.NewEntity(logic.EntityTrade),
	)

	for index := range 8 {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "SYM0/EUR",
			Price:  100 + float64(index),
			Qty:    float64(index%5) + 1,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil, eventAt)

		if err != nil {
			b.Fatal(err)
		}
	}
}
