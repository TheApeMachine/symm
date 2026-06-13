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

func setSentimentTestConfig() {
	viper.Set("signals.sentiment.measurements_capacity", 4)
}

func seedTickers(signal *Signal, symbol string, base time.Time, count int, last float64, changePct float64) {
	for index := range count {
		price := last + float64(index)*0.01
		signal.Record(&krakenmarket.TickerUpdate{
			Symbol:    symbol,
			Last:      price,
			High:      price + 0.2,
			Low:       price - 0.2,
			Volume:    1000,
			VWAP:      price,
			ChangePct: changePct,
			Ask:       price + 0.1,
			Bid:       price - 0.1,
			AskQty:    1,
			BidQty:    1,
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
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 8,
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

func TestSignalMeasure(t *testing.T) {
	setSentimentTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	Convey("Given a bullish cross-section on trades", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		universe := []struct {
			symbol string
			value  float64
		}{
			{"A/EUR", 2.0},
			{"B/EUR", 2.0},
			{"C/EUR", 2.0},
		}

		for _, entry := range universe {
			observeRow(entry.symbol, 100, entry.value, 1000, 1, eventAt)
		}

		seedTickers(signal, "A/EUR", eventAt, 4, 104, 2.0)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify a risk-on surge", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
			So(measurement.Category, ShouldEqual, logic.CategoryRiskOnSurge)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldEqual, measureAt)
		})
	})

	Convey("Given a weak cross-section with a local leader", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"LEAD/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		observeRow("LEAD/EUR", 100, 4.0, 1000, 1, eventAt)
		observeRow("LAG/EUR", 100, -2.0, 1000, 1, eventAt)
		observeRow("FLAT/EUR", 100, -1.0, 1000, 1, eventAt)

		seedTickers(signal, "LEAD/EUR", eventAt, 4, 104, 4.0)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify a divergent move", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryDivergentMove)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTick),
		)
		feedback := market.NewFeedback("A/EUR", 0.5, 1.0, 0.2, 3)

		observeRow("A/EUR", 100, 2.0, 1000, 1, eventAt)
		observeRow("B/EUR", 100, 2.0, 1000, 1, eventAt)

		seedTickers(signal, "A/EUR", eventAt, 4, 100, 2.0)

		_, err := signal.Measure(feedback, measureAt)

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			// FeedbackTuner.Apply() can drive threshold well below the seed when MSE is high.
			So(signal.weights.Threshold, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "A/EUR"})

		_, err := signal.Measure(nil, measureAt)

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a sparse cross-section at startup", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"NEW/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		seedTickers(signal, "NEW/EUR", eventAt, 4, 104, 2.0)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should not error before the universe fills in", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
		})
	})

	Convey("Given future-dated rows in the cross-section", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"A/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		observeRow("A/EUR", 100, 2.0, 1000, 1, measureAt.Add(time.Hour))
		observeRow("B/EUR", 100, 2.0, 1000, 1, eventAt)

		seedTickers(signal, "A/EUR", eventAt, 4, 104, 2.0)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should not error on non-finite breadth inputs", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceSentiment)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setSentimentTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 32,
	})

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		observeRow(symbol, 100, float64(index%5)+0.5, 1000, 1, eventAt)
	}

	signal := NewSignal(
		"SYM0/EUR",
		logic.NewEntity(logic.EntityTick),
	)

	seedTickers(signal, "SYM0/EUR", eventAt, 8, 100, 1.0)

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil, measureAt)

		if err != nil {
			b.Fatal(err)
		}
	}
}
