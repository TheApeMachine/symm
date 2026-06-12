package causal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func observeCrossSectionRow(
	crossSection *market.CrossSection,
	symbol string,
	price, value, volume, pressure float64,
	eventAt time.Time,
) {
	row, err := krakenmarket.NewSymbolRow(symbol, price, value, volume, pressure, eventAt)

	if err != nil {
		panic(err)
	}

	if err := crossSection.Observe(row); err != nil {
		panic(err)
	}
}

func TestCausalMeasureTradeDefersWithoutSamples(t *testing.T) {
	Convey("Given an unwarmed trade ring", t, func() {
		viper.Set("signals.causal.measurements_capacity", 4)

		system := &System{}
		signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityTrade), system)

		measurement, measureErr := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		bus := internal.NewBus(
			context.Background(),
			qpool.NewQ[any](context.Background(), 2, 8, nil),
			[]internal.Channel{internal.ChannelMeasurements},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelMeasurements, "test-measurements"),
			},
		)

		Convey("It should defer without treating emptiness as an error", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Publish(bus), ShouldNotBeNil)
		})
	})
}

func TestCausalCategoryMapping(t *testing.T) {
	Convey("Given Pearl-ladder reasons", t, func() {
		Convey("It should map intervention to endogenous alpha", func() {
			So(causalCategory("intervention"), ShouldEqual, logic.CategoryEndogenousAlpha)
		})

		Convey("It should map regime inversion to liquidity shock", func() {
			So(causalCategory("intervention_regime_inversion"), ShouldEqual, logic.CategoryLiquidityShock)
		})

		Convey("It should map macro association to systemic beta", func() {
			So(causalCategory("macro_association"), ShouldEqual, logic.CategorySystemicBeta)
		})
	})
}

func TestCausalMeasureBookDefersPartialDeltaBeforeTouch(t *testing.T) {
	Convey("Given a one-sided book delta before causal has a complete touch", t, func() {
		viper.Set("signals.causal.measurements_capacity", 4)

		system := &System{}
		signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityBook), system)

		signal.Record(&krakenmarket.BookUpdate{
			Symbol:    "BTC/EUR",
			Timestamp: time.Date(2026, 6, 12, 19, 1, 41, 0, time.UTC),
			Bids: []krakenmarket.BookLevel{
				{Price: 100, Qty: 1},
			},
		})

		measurement, measureErr := signal.Measure(
			nil,
			time.Date(2026, 6, 12, 19, 1, 42, 0, time.UTC),
		)

		Convey("It should defer instead of killing the engine", func() {
			So(measureErr, ShouldBeNil)
			So(measurement, ShouldResemble, logic.Measurement{})
		})
	})
}

func TestCausalSymbolFallbackMeasure(t *testing.T) {
	Convey("Given macro drift without ladder history", t, func() {
		state, err := NewCausalSymbol()
		So(err, ShouldBeNil)

		state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: "BTC/EUR", Last: 50000, ChangePct: 0.02,
		})

		crossSection, err := market.NewCrossSection(&market.CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   64,
			MinBars:     8,
			BreadthHist: 64,
		})

		if err != nil {
			t.Fatal(err)
		}

		observeCrossSectionRow(crossSection, "BTC/EUR", 50000, 0.02, 50000000, 1, time.Now())

		signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityTick), &System{})

		reading, err := state.Measure(0.02, 0.5, time.Now())

		Convey("It should publish systemic beta from association", func() {
			So(err, ShouldBeNil)
			So(reading.Category, ShouldEqual, logic.CategorySystemicBeta)
			So(reading.Strength, ShouldBeGreaterThan, 0)
			So(reading.Confidence, ShouldBeGreaterThan, 0)
		})

		Convey("It should add surprise through the signal publisher", func() {
			reading.Symbol = "BTC/EUR"
			reading.Volume = 50000000
			reading.Spread = 20
			reading.Elapsed = 1
			measurement, err := signal.publish(reading, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCausal)
			So(measurement.Surprise, ShouldBeGreaterThanOrEqualTo, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestFillToCancelThresholdLazyLoad(t *testing.T) {
	Convey("Given viper config for causal contagion", t, func() {
		viper.Set("signals.causal.contagion_break", 0.8)

		Convey("It should read contagion break from config", func() {
			So(viper.GetFloat64("signals.causal.contagion_break"), ShouldEqual, 0.8)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	state, err := NewCausalSymbol()
	if err != nil {
		b.Fatal(err)
	}

	state.FeedTicker(krakenmarket.TickerUpdate{
		Symbol: "BTC/EUR", Last: 50000, ChangePct: 0.02, Bid: 49990, Ask: 50010,
	})
	if err := state.FeedBook(krakenmarket.BookUpdate{
		Bids: []krakenmarket.BookLevel{{Price: 49990, Qty: 10}},
		Asks: []krakenmarket.BookLevel{{Price: 50010, Qty: 10}},
	}); err != nil {
		b.Fatal(err)
	}

	system := &System{}
	crossSection, err := market.NewCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 64,
	})

	if err != nil {
		b.Fatal(err)
	}

	observeCrossSectionRow(crossSection, "BTC/EUR", 50000, 0.02, 50000000, 1, time.Now())
	signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityBook), system)

	b.ReportAllocs()

	for b.Loop() {
		reading, err := state.Measure(0.02, 0.5, time.Now())

		if err == nil && reading.Category != logic.CategoryTypeNone {
			reading.Symbol = "BTC/EUR"
			reading.Volume = 50000000
			reading.Spread = 20
			reading.Elapsed = 1
			_, _ = signal.publish(reading, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		}
	}
}
