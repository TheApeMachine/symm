package replay

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

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

func TestReplaySimulationMakerEntryFees(t *testing.T) {
	convey.Convey("Given a flat round trip with explicit maker/taker costs", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 100,
			},
		}
		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}
		makerEntry := ReplayCosts{
			MakerFeePct: 0.0016,
			TakerFeePct: 0.0026,
			SlippagePct: 0.0005,
		}
		allTaker := ReplayCosts{
			MakerFeePct: 0.0026,
			TakerFeePct: 0.0026,
			SlippagePct: 0.0005,
		}

		makerScore := NewReplaySimulationWithCosts(ctx, branches, rows, makerEntry).Score()
		takerScore := NewReplaySimulationWithCosts(ctx, branches, rows, allTaker).Score()

		convey.Convey("It should charge less drag on limit entry plus taker exit", func() {
			convey.So(makerScore, convey.ShouldBeGreaterThan, takerScore)
			convey.So(
				makerScore-takerScore,
				convey.ShouldAlmostEqual,
				0.0010,
				0.0001,
			)
		})
	})
}
