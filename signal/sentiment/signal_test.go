package sentiment

import (
	"container/ring"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSignalMeasure(t *testing.T) {
	Convey("Given a bullish cross-section on trades", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(8)}
		measurements := ring.New(4)
		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
			measurements,
			crossSection,
			2.0,
			0.5,
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
				ring.New(4),
				crossSection,
				2.0,
				0.5,
			)

			for _, price := range entry.prices {
				entrySignal.measurements.Value = &krakenmarket.TradeUpdate{
					Symbol: entry.symbol,
					Price:  price,
					Qty:    1,
				}
				entrySignal.measurements = entrySignal.measurements.Next()
			}

			crossSection.publishChange(entry.symbol, 2.0)
		}

		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "A/EUR",
			Price:  102,
			Qty:    1,
		}
		signal.measurements = signal.measurements.Next()
		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "A/EUR",
			Price:  104,
			Qty:    1,
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should classify a risk-on surge", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
			So(measurement.Category, ShouldEqual, logic.CategoryRiskOnSurge)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a weak cross-section with a local leader", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(8)}
		signal := NewSignal(
			"LEAD/EUR",
			logic.NewEntity(logic.EntityTrade),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		crossSection.publishChange("LEAD/EUR", 4.0)
		crossSection.publishChange("LAG/EUR", -2.0)
		crossSection.publishChange("FLAT/EUR", -1.0)

		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "LEAD/EUR",
			Price:  100,
			Qty:    1,
		}
		signal.measurements = signal.measurements.Next()
		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "LEAD/EUR",
			Price:  104,
			Qty:    1,
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should classify a divergent move", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDivergentMove)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(4)}
		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)
		feedback := market.NewFeedback("A/EUR", 0.5, 1.0, 0.2, 3)

		_, err := signal.Measure(feedback)

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 2.0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(4)}
		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
			ring.New(2),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.TickerUpdate{Symbol: "A/EUR"}
		signal.measurements = signal.measurements.Next()

		_, err := signal.Measure(nil)

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	crossSection := &crossSection{breadthHistory: ring.New(32)}

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		crossSection.publishChange(symbol, float64(index%5)+0.5)
	}

	measurements := ring.New(8)
	signal := NewSignal(
		"SYM0/EUR",
		logic.NewEntity(logic.EntityTrade),
		measurements,
		crossSection,
		2.0,
		0.5,
	)

	for index := range 8 {
		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "SYM0/EUR",
			Price:  100 + float64(index),
			Qty:    float64(index%5) + 1,
		}
		signal.measurements = signal.measurements.Next()
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil)

		if err != nil {
			b.Fatal(err)
		}
	}
}
