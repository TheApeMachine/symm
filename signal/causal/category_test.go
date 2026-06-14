package causal

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestCausalCategory(t *testing.T) {
	Convey("Given causal ladder reasons", t, func() {
		Convey("It should map intervention paths to endogenous alpha", func() {
			So(causalCategory("intervention"), ShouldEqual, logic.CategoryEndogenousAlpha)
			So(causalCategory("counterfactual"), ShouldEqual, logic.CategoryEndogenousAlpha)
		})

		Convey("It should map regime inversion to liquidity shock", func() {
			So(
				causalCategory("intervention_regime_inversion"),
				ShouldEqual,
				logic.CategoryLiquidityShock,
			)
		})

		Convey("It should map macro association to systemic beta", func() {
			So(causalCategory("macro_association"), ShouldEqual, logic.CategorySystemicBeta)
		})
	})
}

func TestBookLiquidity(t *testing.T) {
	Convey("Given spread, floor, and batch volume", t, func() {
		Convey("It should scale volume by effective spread", func() {
			liquidity := bookLiquidity(20, 10, 1000)
			So(liquidity, ShouldEqual, 50)
		})

		Convey("It should reject incomplete inputs", func() {
			So(bookLiquidity(0, 10, 1000), ShouldEqual, 0)
		})
	})
}

func TestMacroSectionMacroMomentum(t *testing.T) {
	Convey("Given peer symbol changes", t, func() {
		crossSection.reset()
		crossSection.Observe("BTC/EUR", 0.02)
		crossSection.Observe("ETH/EUR", 0.04)
		crossSection.Observe("SOL/EUR", 0.06)

		Convey("It should return the peer median excluding the queried symbol", func() {
			So(crossSection.MacroMomentum("BTC/EUR"), ShouldEqual, 0.05)
		})
	})
}

func TestCausalSymbolEvaluate(t *testing.T) {
	Convey("Given labeled causal history", t, func() {
		state, err := NewCausalSymbol()

		So(err, ShouldBeNil)

		for index := range minCausalHistory + 4 {
			state.samples = append(state.samples, newCausalSample(
				0.01*float64(index%3),
				10+float64(index),
				5+float64(index%5),
				0.001*float64(index%4),
			))
		}

		outcome, err := state.evaluate(
			newCausalSample(0.02, 12, 6, 0),
			0.1,
		)

		So(err, ShouldBeNil)
		So(math.IsNaN(outcome.Raw), ShouldBeFalse)
	})
}

func TestAnchorChange(t *testing.T) {
	Convey("Given anchor and price", t, func() {
		Convey("It should return fractional change from anchor", func() {
			_, change := anchorChange(100, 105)
			So(change, ShouldAlmostEqual, 0.05, 1e-9)
		})
	})
}

func BenchmarkCausalSymbolEvaluate(b *testing.B) {
	state, err := NewCausalSymbol()

	if err != nil {
		b.Fatal(err)
	}

	for index := range minCausalHistory + 20 {
		state.samples = append(state.samples, newCausalSample(
			0.01*float64(index%3),
			10+float64(index),
			5+float64(index%5),
			0.001*float64(index%4),
		))
	}

	current := newCausalSample(0.02, 12, 6, 0)

	b.ReportAllocs()

	for b.Loop() {
		_, err := state.evaluate(current, 0.15)

		if err != nil {
			b.Fatal(err)
		}
	}
}
