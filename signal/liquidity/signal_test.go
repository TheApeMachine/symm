package liquidity

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

func setLiquidityTestConfig() {
	viper.Set("signals.liquidity.measurements_capacity", 4)
	viper.Set("signals.liquidity.baseline_window", time.Minute)
}

func seedTickers(signal *Signal, symbol string, base time.Time, count int, last float64, volume float64) {
	for index := range count {
		price := last + float64(index)*0.01
		signal.Record(&krakenmarket.TickerUpdate{
			Symbol:    symbol,
			Last:      price,
			High:      price + 0.2,
			Low:       price - 0.2,
			Volume:    volume,
			VWAP:      price,
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
		BreadthHist: 64,
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
	setLiquidityTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	Convey("Given a cross-section with deep and thin peers", t, func() {
		useCrossSection(t)

		observeRow("COIN/EUR", 10, 1, 800, 1, eventAt)
		observeRow("PEER/EUR", 10, 1, 900, 1, eventAt)

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		seedTickers(signal, "ALT/EUR", eventAt, 4, 10, 1200)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should publish robust liquidity", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
			So(measurement.Category, ShouldEqual, logic.CategoryRobustLiquidity)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldEqual, measureAt)
		})
	})

	Convey("Given a peak-scarcity symbol", t, func() {
		useCrossSection(t)

		observeRow("DEEP/EUR", 10, 1, 1100, 1, eventAt)
		observeRow("MID/EUR", 10, 1, 950, 1, eventAt)

		signal := NewSignal(
			"THIN/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		seedTickers(signal, "THIN/EUR", eventAt, 4, 5, 50)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify extreme scarcity", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryExtremeScarcity)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given market-wide high absolute volume", t, func() {
		useCrossSection(t)

		observeRow("DEEP/EUR", 10, 1, 600, 1, eventAt)
		observeRow("MID/EUR", 10, 1, 700, 1, eventAt)

		signal := NewSignal(
			"THIN/EUR",
			logic.NewEntity(logic.EntityTick),
		)
		_, baselineErr := signal.volumeBaseline.Update(
			eventAt.Add(-time.Hour),
			100,
		)

		So(baselineErr, ShouldBeNil)

		seedTickers(signal, "THIN/EUR", eventAt, 4, 5, 300)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should suppress relative scarcity above baseline", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryMedianDepth)
		})
	})

	Convey("Given fewer than two universe symbols", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"SOLO/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		seedTickers(signal, "SOLO/EUR", eventAt, 4, 5, 100)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should return an empty measurement without a peer universe", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
		})
	})

	Convey("Given a book-triggered ticker without 24h summary", t, func() {
		useCrossSection(t)

		observeRow("COIN/EUR", 10, 1, 800, 1, eventAt)
		observeRow("PEER/EUR", 10, 1, 900, 1, eventAt)

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		signal.Record(&krakenmarket.TickerUpdate{
			Symbol:    "ALT/EUR",
			Bid:       9.9,
			Ask:       10.1,
			AskQty:    1,
			BidQty:    1,
			Timestamp: eventAt,
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should publish without ResolveValue errors", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		useCrossSection(t)
		viper.Set("signals.liquidity.surprise_threshold", 2.0)

		observeRow("COIN/EUR", 10, 1, 800, 1, eventAt)
		observeRow("PEER/EUR", 10, 1, 900, 1, eventAt)

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
		)
		feedback := market.NewFeedback("ALT/EUR", 0.5, 1.0, 0.2, 3)

		seedTickers(signal, "ALT/EUR", eventAt, 4, 10, 1200)

		_, err := signal.Measure(feedback, measureAt)

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 2.0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "ALT/EUR"})

		_, err := signal.Measure(nil, measureAt)

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSignalClassify(t *testing.T) {
	Convey("Given peer quote volumes", t, func() {
		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
		)
		peers := []float64{800, 900, 1000, 1100}
		lower, upper := signal.quartiles(peers)

		Convey("It should map peer quartiles onto scarcity categories", func() {
			So(signal.classify(1200, lower, upper, false, false), ShouldEqual, logic.CategoryRobustLiquidity)
			So(signal.classify(950, lower, upper, false, false), ShouldEqual, logic.CategoryMedianDepth)
			So(signal.classify(500, lower, upper, true, false), ShouldEqual, logic.CategoryExtremeScarcity)
			So(signal.classify(500, lower, upper, true, true), ShouldEqual, logic.CategoryMedianDepth)
		})
	})
}

func TestAbsoluteScaledVolumes(t *testing.T) {
	setLiquidityTestConfig()

	Convey("Given baseline-relative volume above history", t, func() {
		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		scaledQuote, scaledPeers := signal.absoluteScaledVolumes(
			300,
			[]float64{600, 700},
			3,
			true,
		)

		Convey("It should lift cross-section volumes before quartiles", func() {
			So(scaledQuote, ShouldEqual, 900)
			So(scaledPeers, ShouldResemble, []float64{1800, 2100})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setLiquidityTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 64,
	})

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		observeRow(symbol, 10, 1, float64(500+index*50), 1, eventAt)
	}

	signal := NewSignal(
		"SYM0/EUR",
		logic.NewEntity(logic.EntityTick),
	)

	seedTickers(signal, "SYM0/EUR", eventAt, 4, 10, 1200)

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil, eventAt.Add(time.Second))

		if err != nil {
			b.Fatal(err)
		}
	}
}
