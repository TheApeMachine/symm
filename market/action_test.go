package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestPrepareActionEntrySizing(t *testing.T) {
	Convey("Given an empty book and a buy action", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		holdings := logic.NewHoldings()
		measurements := []logic.Measurement{
			logic.NewMeasurement(
				logic.SourceHawkes,
				"BTC/USD",
				50_000,
				1,
				1,
				1,
				1,
				logic.CategoryFrenzy,
				logic.RegimeTypeNone,
				logic.PositionTypeNone,
				0.9,
				1,
			),
		}

		action := &logic.Action{
			Type: logic.ActionMarket,
			Side: trading.Buy,
		}

		tradingConfig := config.TradingConfig{
			Model:                  "paper",
			PrimarySlotCount:       2,
			PrimarySlotFraction:    0.2,
			SecondarySlotFraction:  0.1,
			MaxConcurrentPositions: 5,
		}

		Convey("It should size the first slot at the primary fraction", func() {
			prepared, err := prepareAction(holdings, action, measurements, tradingConfig, 200)

			So(err, ShouldBeNil)
			So(prepared, ShouldNotBeNil)
			So(prepared.Quantity, ShouldAlmostEqual, 0.0008, 1e-12)
			So(prepared.Fraction, ShouldEqual, 0.2)
			So(prepared.EntryConfidence, ShouldEqual, 0.9)
		})
	})
}

func TestPrepareActionExitQuantity(t *testing.T) {
	Convey("Given an open position and a settle action", t, func() {
		holdings := logic.NewHoldings()
		holdings.SetPosition("BTC/USD", 0.5, 0.8)

		action := &logic.Action{
			Type:     logic.ActionSettlePosition,
			Side:     trading.Sell,
			Symbol:   "BTC/USD",
			Fraction: 1.0,
		}

		prepared, err := prepareAction(holdings, action, nil, config.TradingConfig{}, 0)

		Convey("It should fill exit quantity from holdings", func() {
			So(err, ShouldBeNil)
			So(prepared, ShouldNotBeNil)
			So(prepared.Quantity, ShouldEqual, 0.5)
		})
	})
}
