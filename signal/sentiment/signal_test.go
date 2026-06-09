package sentiment

import (
	"container/ring"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSignalMeasure(t *testing.T) {
	Convey("Given a bullish cross-section on trades", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(8), matchWindow: time.Minute}
		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
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
				4,
				crossSection,
				2.0,
				0.5,
			)

			for _, price := range entry.prices {
				entrySignal.Record(&krakenmarket.TradeUpdate{
					Symbol: entry.symbol,
					Price:  price,
					Qty:    1,
				})
			}

			crossSection.publishChange(entry.symbol, 2.0, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
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

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify a risk-on surge", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
			So(measurement.Category, ShouldEqual, logic.CategoryRiskOnSurge)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a weak cross-section with a local leader", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(8), matchWindow: time.Minute}
		signal := NewSignal(
			"LEAD/EUR",
			logic.NewEntity(logic.EntityTrade),
			4,
			crossSection,
			2.0,
			0.5,
		)

		crossSection.publishChange("LEAD/EUR", 4.0, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		crossSection.publishChange("LAG/EUR", -2.0, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		crossSection.publishChange("FLAT/EUR", -1.0, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

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

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify a divergent move", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDivergentMove)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(4), matchWindow: time.Minute}
		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
			4,
			crossSection,
			2.0,
			0.5,
		)
		feedback := market.NewFeedback("A/EUR", 0.5, 1.0, 0.2, 3)

		_, err := signal.Measure(feedback, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 2.0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		crossSection := &crossSection{breadthHistory: ring.New(4), matchWindow: time.Minute}
		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
			2,
			crossSection,
			2.0,
			0.5,
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "A/EUR"})

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	crossSection := &crossSection{breadthHistory: ring.New(32), matchWindow: time.Minute}

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		crossSection.publishChange(symbol, float64(index%5)+0.5, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	}

	signal := NewSignal(
		"SYM0/EUR",
		logic.NewEntity(logic.EntityTrade),
		8,
		crossSection,
		2.0,
		0.5,
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
		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		if err != nil {
			b.Fatal(err)
		}
	}
}
