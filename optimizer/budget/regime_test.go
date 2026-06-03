package budget

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTagRowRegimes(t *testing.T) {
	convey.Convey("Given causal measurements on replay rows", t, func() {
		viper.Set("signals.causal.condition_switch", 100.0)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha, SNR: 3, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryLiquidityShock,
				Strength: 150, SNR: 5, Last: 90,
			},
		}
		tags := TagRowRegimes(rows)

		convey.Convey("It should tag each row by dominant causal regime", func() {
			convey.So(tags[0], convey.ShouldEqual, StructuralRegimeNormalFlow)
			convey.So(tags[1], convey.ShouldEqual, StructuralRegimeLiquidityPanic)
		})
	})
}

func TestCausalStructuralRegimeUsesConditionSwitch(t *testing.T) {
	convey.Convey("Given a causal liquidity shock above the condition switch", t, func() {
		viper.Set("signals.causal.condition_switch", 100.0)
		snapshots := []perspectives.Measurement{{
			Source:   perspectives.SourceCausal,
			Category: perspectives.CategoryLiquidityShock,
			Strength: 150,
			SNR:      2,
		}}

		convey.Convey("It should classify the tick as liquidity panic", func() {
			regime, ok := causalStructuralRegime(snapshots)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(regime, convey.ShouldEqual, StructuralRegimeLiquidityPanic)
		})
	})
}

func init() {
	if viper.GetViper().GetFloat64("signals.causal.condition_switch") <= 0 {
		viper.Set("signals.causal.condition_switch", 100.0)
	}
}
