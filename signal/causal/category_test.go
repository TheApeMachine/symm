package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestCausalEvidence(t *testing.T) {
	Convey("Given causal category evidence", t, func() {
		Convey("It should read finite evidence for clear systemic beta association", func() {
			evidence := associationEvidence(
				types.CategorySystemicBeta,
				2.0, 1.5, 0.5,
			)

			So(evidence, ShouldBeGreaterThan, uniformCausalConfidence)
			So(evidence, ShouldBeLessThan, 1)
		})

		Convey("It should read zero on the flow-pressure boundary for beta", func() {
			So(betaEvidence(2.0, 0, 0.8), ShouldEqual, 0)
		})

		Convey("It should read finite evidence for clear flow-pressure noise", func() {
			evidence := noiseEvidence(0.2, 0, 0.9)

			So(evidence, ShouldBeGreaterThan, uniformCausalConfidence)
			So(evidence, ShouldBeLessThan, 1)
		})

		Convey("It should read low for endogenous alpha near contagion break", func() {
			outcome := causalOutcome{
				raw:          1.0,
				intervention: 1.0,
				association:  0.2,
				contagion:    viper.GetViper().GetFloat64("signals.causal.contagion_break") - 0.01,
				inverted:     false,
			}

			evidence := ladderEvidence(
				types.CategoryEndogenousAlpha,
				outcome,
			)

			So(evidence, ShouldBeLessThan, 0.2)
		})

		Convey("It should read high for liquidity shock above contagion break", func() {
			outcome := causalOutcome{
				raw:          1.0,
				intervention: 1.0,
				association:  0.2,
				contagion:    viper.GetViper().GetFloat64("signals.causal.contagion_break") + 0.05,
				inverted:     true,
			}

			evidence := ladderEvidence(
				types.CategoryLiquidityShock,
				outcome,
			)

			So(evidence, ShouldBeGreaterThan, 0.2)
		})
	})
}

func TestCausalCategoryMapping(t *testing.T) {
	Convey("Given category evidence dispatch", t, func() {
		Convey("It should route ladder and association categories separately", func() {
			ladder := causalEvidence(
				types.CategoryEndogenousAlpha,
				causalOutcome{intervention: 1, association: 0.2, contagion: 0.1},
				0, 0, 0, true,
			)
			association := causalEvidence(
				types.CategorySystemicBeta,
				causalOutcome{},
				2, 1.5, 0.5, false,
			)

			So(ladder, ShouldBeGreaterThan, 0)
			So(association, ShouldBeGreaterThan, uniformCausalConfidence)
			So(association, ShouldBeLessThan, 1)
		})
	})
}

func TestInversionMarginAboveConditionBreak(t *testing.T) {
	Convey("Given a liquidity shock above the condition switch", t, func() {
		outcome := causalOutcome{
			intervention: 1,
			condition:    viper.GetViper().GetFloat64("signals.causal.condition_switch") + 500,
		}

		margin := inversionMarginAbove(outcome)

		Convey("It should read high confidence", func() {
			So(margin, ShouldBeGreaterThan, 0.2)
		})
	})
}

func BenchmarkCausalEvidence(b *testing.B) {
	outcome := causalOutcome{
		raw:          1,
		intervention: 1,
		association:  0.2,
		contagion:    0.1,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = causalEvidence(
			types.CategoryEndogenousAlpha,
			outcome,
			0.5, 1.2, 0.4, true,
		)
	}
}
