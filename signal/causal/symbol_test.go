package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCausalCategory(t *testing.T) {
	Convey("Given causal reasons", t, func() {
		Convey("It should map ladder and fallback reasons onto perspectives", func() {
			So(causalCategory("intervention"), ShouldEqual, perspectives.CategoryEndogenousAlpha)
			So(causalCategory("counterfactual_like_regime_inversion"), ShouldEqual, perspectives.CategoryLiquidityShock)
			So(causalCategory("macro_association"), ShouldEqual, perspectives.CategorySystemicBeta)
			So(causalCategory("flow_pressure"), ShouldEqual, perspectives.CategoryCausalNoise)
		})
	})
}

func TestCausalSymbolEvaluate(t *testing.T) {
	Convey("Given enough ladder training history", t, func() {
		state := NewCausalSymbol()
		seedLadderHistory(state, minCausalHistory+8)

		outcome := state.evaluate(
			newCausalSample(0.5, 90, 2.5, 0),
			0,
		)

		Convey("It should produce a positive intervention read", func() {
			So(outcome.intervention, ShouldBeGreaterThan, 0)
			So(outcome.raw, ShouldBeGreaterThan, 0)
			So(outcome.reason, ShouldNotBeEmpty)
		})
	})

	Convey("Given insufficient history", t, func() {
		state := NewCausalSymbol()

		outcome := state.evaluate(newCausalSample(0.5, 90, 2.5, 0), 0)

		Convey("It should not emit a ladder outcome", func() {
			So(outcome.raw, ShouldEqual, 0)
		})
	})
}

func TestCausalSymbolMeasure(t *testing.T) {
	Convey("Given a symbol with ladder history and live microstructure", t, func() {
		state := NewCausalSymbol()
		base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

		state.FeedTicker(market.TickerUpdate{Last: 100, ChangePct: 1.2, Volume: 1000})
		state.FeedBook(bookSnapshot("BTC/EUR", 99.5, 8, 100.5, 6))

		for index := range 16 {
			state.FeedTrade(market.TradeUpdate{
				Symbol:    "BTC/EUR",
				Side:      "buy",
				Price:     100,
				Qty:       3,
				Timestamp: base.Add(time.Duration(index) * time.Millisecond),
			})
		}

		seedLadderHistory(state, minCausalHistory+8)

		measurement, _, err := state.Measure(0.5, 0, base.Add(time.Second))

		Convey("It should publish a ladder measurement with category confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, perspectives.SourceCausal)
			So(measurement.Category, ShouldEqual, perspectives.CategoryEndogenousAlpha)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a fallback macro read without ladder history", t, func() {
		state := NewCausalSymbol()

		state.FeedTicker(market.TickerUpdate{Last: 100, ChangePct: 1.5, Volume: 1000})

		measurement, _, err := state.Measure(0.8, 0, time.Now())

		Convey("It should classify systemic beta from macro association", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, perspectives.CategorySystemicBeta)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given flow pressure without price change", t, func() {
		state := NewCausalSymbol()
		base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

		state.FeedTicker(market.TickerUpdate{Last: 100, ChangePct: 0, Volume: 1000})

		for index := range 8 {
			state.FeedTrade(market.TradeUpdate{
				Symbol:    "BTC/EUR",
				Side:      "buy",
				Price:     100,
				Qty:       2,
				Timestamp: base.Add(time.Duration(index) * time.Millisecond),
			})
		}

		measurement, _, err := state.Measure(0, 0, base.Add(time.Second))

		Convey("It should classify causal noise from flow pressure", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, perspectives.CategoryCausalNoise)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestCausalSymbolEvaluateUpliftPath(t *testing.T) {
	Convey("Given nonlinear training history", t, func() {
		state := NewCausalSymbol()
		state.mu.Lock()
		state.samples = upliftTrainingSamples(minCausalHistory + 16)
		state.mu.Unlock()

		current := newCausalSample(0.2, 90, 3.5, 0)
		outcome := state.evaluate(current, 0)

		Convey("It should reach the uplift branch", func() {
			So(outcome.raw, ShouldBeGreaterThan, 0)
			So(outcome.uplift, ShouldNotEqual, 0)
			So(outcome.reason, ShouldNotBeEmpty)
		})
	})
}

func TestCausalSymbolEvaluateConditionBreak(t *testing.T) {
	Convey("Given collinear liquidity and flow history", t, func() {
		state := NewCausalSymbol()
		state.mu.Lock()
		state.samples = collinearTrainingSamples(minCausalHistory + 12)
		state.mu.Unlock()

		outcome := state.evaluate(newCausalSample(0.1, 80, 2, 0), 0)

		Convey("It should classify through the inverted regime", func() {
			So(outcome.inverted, ShouldBeTrue)
			So(causalCategory(outcome.reason), ShouldEqual, perspectives.CategoryLiquidityShock)
		})
	})
}

func TestCausalSymbolMeasureWithoutPrice(t *testing.T) {
	Convey("Given a symbol without a last price", t, func() {
		state := NewCausalSymbol()

		measurement, _, err := state.Measure(0.5, 0, time.Now())

		Convey("It should not publish", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, perspectives.SourceNone)
		})
	})
}

func BenchmarkCausalSymbolEvaluate(b *testing.B) {
	state := NewCausalSymbol()
	seedLadderHistory(state, minCausalHistory+8)
	current := newCausalSample(0.5, 90, 2.5, 0)

	b.ReportAllocs()

	for b.Loop() {
		_ = state.evaluate(current, 0.2)
	}
}

func BenchmarkCausalSymbolMeasure(b *testing.B) {
	state := NewCausalSymbol()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	state.FeedTicker(market.TickerUpdate{Last: 100, ChangePct: 1.2, Volume: 1000})
	state.FeedBook(bookSnapshot("BTC/EUR", 99.5, 8, 100.5, 6))

	for index := range 16 {
		state.FeedTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "buy",
			Price:     100,
			Qty:       3,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		})
	}

	seedLadderHistory(state, minCausalHistory+8)
	now := base.Add(time.Second)

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = state.Measure(0.5, 0.2, now)
	}
}
