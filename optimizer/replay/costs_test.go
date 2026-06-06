package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func TestReplayCostsFeePct(t *testing.T) {
	convey.Convey("Given replay costs with maker and taker fees", t, func() {
		costs := ReplayCosts{
			MakerFeePct: 0.0016,
			TakerFeePct: 0.0026,
		}

		convey.Convey("It should select the maker rate for maker actions", func() {
			convey.So(
				costs.feePct(reasoning.ActionLimit),
				convey.ShouldAlmostEqual,
				0.0016,
				0.0000001,
			)
		})

		convey.Convey("It should select the taker rate for taker actions", func() {
			convey.So(
				costs.feePct(reasoning.ActionMarket),
				convey.ShouldAlmostEqual,
				0.0026,
				0.0000001,
			)
		})
	})
}

func TestReplayCostsTrailingVolatilityMultiple(t *testing.T) {
	convey.Convey("Given a configured trailing volatility multiple", t, func() {
		viper.Set("trading.exit.trailing_volatility_multiple", 4.5)

		defer viper.Set("trading.exit.trailing_volatility_multiple", 0.0)

		costs := ReplayCostsFromViper()

		convey.Convey("It should load the configured multiple", func() {
			convey.So(
				costs.TrailingVolatilityMultiple,
				convey.ShouldAlmostEqual,
				4.5,
				0.0000001,
			)
		})
	})
}

func BenchmarkReplayCostsFromViper(b *testing.B) {
	for b.Loop() {
		_ = ReplayCostsFromViper()
	}
}

func TestReplayCostsFromViper(t *testing.T) {
	convey.Convey("Given paper fee settings in config", t, func() {
		viper.Set("trading.paper.taker_fee_pct", 0.26)
		viper.Set("trading.paper.maker_fee_pct", 0.16)
		viper.Set("trading.paper.slippage_bps", 4.0)

		defer func() {
			viper.Set("trading.paper.taker_fee_pct", 0.0)
			viper.Set("trading.paper.maker_fee_pct", 0.0)
			viper.Set("trading.paper.slippage_bps", 0.0)
			viper.Set("trading.paper.fee_pct", 0.0)
		}()

		costs := ReplayCostsFromViper()

		convey.Convey("It should convert percent config into fractional replay costs", func() {
			convey.So(costs.TakerFeePct, convey.ShouldAlmostEqual, 0.0026, 0.0000001)
			convey.So(costs.MakerFeePct, convey.ShouldAlmostEqual, 0.0016, 0.0000001)
			convey.So(costs.SlippagePct, convey.ShouldAlmostEqual, 0.0004, 0.0000001)
		})
	})

	convey.Convey("Given only fee_pct alias", t, func() {
		viper.Set("trading.paper.fee_pct", 0.20)

		defer viper.Set("trading.paper.fee_pct", 0.0)

		costs := ReplayCostsFromViper()

		convey.Convey("It should treat fee_pct as the taker rate", func() {
			convey.So(costs.TakerFeePct, convey.ShouldAlmostEqual, 0.0020, 0.0000001)
		})
	})
}
