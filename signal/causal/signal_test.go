package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

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

func TestCausalSymbolFallbackMeasure(t *testing.T) {
	Convey("Given macro drift without ladder history", t, func() {
		state := NewCausalSymbol()
		state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: "BTC/EUR", Last: 50000, ChangePct: 0.02,
		})

		system := &System{crossSection: &crossSection{}}
		system.crossSection.publishChangePct("BTC/EUR", 0.02)
		signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityTick), 8, system, 2.0, 0.5)

		reading, err := state.Measure(system.crossSection.macroMomentum("BTC/EUR"), 0, time.Now())

		Convey("It should publish systemic beta from association", func() {
			So(err, ShouldBeNil)
			So(reading.Category, ShouldEqual, logic.CategorySystemicBeta)
			So(reading.Strength, ShouldBeGreaterThan, 0)
		})

		Convey("It should add surprise through the signal publisher", func() {
			reading.Symbol = "BTC/EUR"
			measurement, err := signal.publish(reading)

			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCausal)
			So(measurement.Surprise, ShouldBeGreaterThanOrEqualTo, 0)
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
	state := NewCausalSymbol()
	state.FeedTicker(krakenmarket.TickerUpdate{
		Symbol: "BTC/EUR", Last: 50000, ChangePct: 0.02, Bid: 49990, Ask: 50010,
	})
	state.FeedBook(krakenmarket.Book{
		Bids: []krakenmarket.BookLevel{{Price: 49990, Qty: 10}},
		Asks: []krakenmarket.BookLevel{{Price: 50010, Qty: 10}},
	})

	system := &System{crossSection: &crossSection{}}
	system.crossSection.publishChangePct("BTC/EUR", 0.02)
	signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityBook), 64, system, 2.0, 0.5)

	b.ReportAllocs()

	for b.Loop() {
		reading, err := state.Measure(system.crossSection.macroMomentum("BTC/EUR"), 0, time.Now())

		if err == nil && reading.Category != logic.CategoryTypeNone {
			reading.Symbol = "BTC/EUR"
			_, _ = signal.publish(reading)
		}
	}
}
