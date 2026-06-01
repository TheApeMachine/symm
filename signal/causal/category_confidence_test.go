package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCategoryConfidence(t *testing.T) {
	Convey("Given causal category confidence", t, func() {
		Convey("It should read high for clear systemic beta fallback", func() {
			confidence := fallbackCategoryConfidence(
				perspectives.CategorySystemicBeta,
				2.0, 1.5, 0.5,
			)

			So(confidence, ShouldAlmostEqual, 1.0, 0.01)
		})

		Convey("It should read zero on the flow-pressure boundary for beta", func() {
			So(fallbackBetaConfidence(2.0, 0, 0.8), ShouldEqual, 0)
		})

		Convey("It should read high for clear flow-pressure noise", func() {
			confidence := fallbackNoiseConfidence(0.2, 0, 0.9)

			So(confidence, ShouldBeGreaterThan, 0.8)
		})

		Convey("It should read low for endogenous alpha near contagion break", func() {
			outcome := causalOutcome{
				raw:          1.0,
				intervention: 1.0,
				association:  0.2,
				contagion:    config.System.CausalContagionBreak - 0.01,
				inverted:     false,
			}

			confidence := ladderCategoryConfidence(
				perspectives.CategoryEndogenousAlpha,
				outcome,
			)

			So(confidence, ShouldBeLessThan, 0.2)
		})

		Convey("It should read high for liquidity shock above contagion break", func() {
			outcome := causalOutcome{
				raw:          1.0,
				intervention: 1.0,
				association:  0.2,
				contagion:    config.System.CausalContagionBreak + 0.05,
				inverted:     true,
			}

			confidence := ladderCategoryConfidence(
				perspectives.CategoryLiquidityShock,
				outcome,
			)

			So(confidence, ShouldBeGreaterThan, 0.2)
		})
	})
}

func TestCausalCategoryMapping(t *testing.T) {
	Convey("Given category confidence dispatch", t, func() {
		Convey("It should route ladder and fallback categories separately", func() {
			ladder := categoryConfidence(
				perspectives.CategoryEndogenousAlpha,
				causalOutcome{intervention: 1, association: 0.2, contagion: 0.1},
				0, 0, 0, true,
			)
			fallback := categoryConfidence(
				perspectives.CategorySystemicBeta,
				causalOutcome{},
				2, 1.5, 0.5, false,
			)

			So(ladder, ShouldBeGreaterThan, 0)
			So(fallback, ShouldAlmostEqual, 1.0, 0.01)
		})
	})
}

func TestInversionMarginAboveConditionBreak(t *testing.T) {
	Convey("Given a liquidity shock above the condition switch", t, func() {
		outcome := causalOutcome{
			intervention: 1,
			condition:    config.System.CausalConditionSwitch + 500,
		}

		margin := inversionMarginAbove(outcome)

		Convey("It should read high confidence", func() {
			So(margin, ShouldBeGreaterThan, 0.2)
		})
	})
}

func BenchmarkCategoryConfidence(b *testing.B) {
	outcome := causalOutcome{
		raw:          1,
		intervention: 1,
		association:  0.2,
		contagion:    0.1,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = categoryConfidence(
			perspectives.CategoryEndogenousAlpha,
			outcome,
			0.5, 1.2, 0.4, true,
		)
	}
}
