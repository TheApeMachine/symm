package market

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/trader"
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
			logic.NewMeasurement(
				logic.SourceCVD,
				"BTC/USD",
				50_000,
				0.8,
				1,
				1,
				1,
				logic.CategoryAggressiveDrive,
				logic.RegimeTypeNone,
				logic.PositionTypeNone,
				0.85,
				0.9,
			),
		}

		action := &logic.Action{
			Type: logic.ActionMarket,
			Side: trading.Buy,
		}

		tradingConfig := config.TradingConfig{
			Model:                  "paper",
			MaxConcurrentPositions: 4,
			OpportunitySlotCount:   2,
		}

		thresholdConfig := config.ThresholdConfig{
			EntryConfidenceBaseline:   0.55,
			TurbulenceConfidenceScale: 0.30,
			EntrySurpriseBaseline:     1.0,
		}

		Convey("It should derive positive entry sizing from the measurement spectrum", func() {
			capitalProvider, providerErr := trader.NewStaticCapitalProvider(200)

			So(providerErr, ShouldBeNil)

			prepared, err := prepareAction(
				context.Background(),
				holdings,
				logic.EntrySlotOccupancyFromHoldings(holdings),
				action,
				measurements,
				tradingConfig,
				thresholdConfig,
				capitalProvider,
				0,
			)

			So(err, ShouldBeNil)
			So(prepared, ShouldNotBeNil)
			So(prepared.Fraction, ShouldBeGreaterThan, 0)
			So(prepared.Quantity, ShouldAlmostEqual, (200*prepared.Fraction)/50_000, 1e-12)
			So(prepared.EntryConfidence, ShouldEqual, 0.9)
		})
	})
}

func TestPrepareActionOpportunitySlots(t *testing.T) {
	Convey("Given four open base positions", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		holdings := logic.NewHoldings()

		for index := range 4 {
			holdings.SetPosition(
				fmt.Sprintf("SYM%d/USD", index),
				1,
				0.7,
				false,
			)
		}

		tradingConfig := config.TradingConfig{
			Model:                  "paper",
			MaxConcurrentPositions: 4,
			OpportunitySlotCount:   2,
		}

		thresholdConfig := config.ThresholdConfig{
			EntryConfidenceBaseline:   0.55,
			TurbulenceConfidenceScale: 0.30,
			EntrySurpriseBaseline:     1.0,
		}

		capitalProvider, providerErr := trader.NewStaticCapitalProvider(200)

		So(providerErr, ShouldBeNil)

		pumpMeasurements := []logic.Measurement{
			logic.NewMeasurement(
				logic.SourcePumpDump,
				"CELO/USD",
				0.25,
				1.2,
				100,
				0.01,
				1,
				logic.CategoryVerticalIgnition,
				logic.RegimeTypeNone,
				logic.PositionTypeNone,
				0.9,
				1.4,
			),
		}

		plainMeasurements := []logic.Measurement{
			logic.NewMeasurement(
				logic.SourceHawkes,
				"XLM/USD",
				0.12,
				0.4,
				100,
				0.01,
				1,
				logic.CategoryLaminar,
				logic.RegimeTypeNone,
				logic.PositionTypeNone,
				0.6,
				0.8,
			),
		}

		Convey("It should reject a regular entry", func() {
			prepared, err := prepareAction(
				context.Background(),
				holdings,
				logic.EntrySlotOccupancyFromHoldings(holdings),
				&logic.Action{Type: logic.ActionMarket, Side: trading.Buy},
				plainMeasurements,
				tradingConfig,
				thresholdConfig,
				capitalProvider,
				0,
			)

			So(err, ShouldBeNil)
			So(prepared, ShouldBeNil)
		})

		Convey("It should accept a qualified pump entry in an opportunity slot", func() {
			prepared, err := prepareAction(
				context.Background(),
				holdings,
				logic.EntrySlotOccupancyFromHoldings(holdings),
				&logic.Action{Type: logic.ActionMarket, Side: trading.Buy},
				pumpMeasurements,
				tradingConfig,
				thresholdConfig,
				capitalProvider,
				0,
			)

			So(err, ShouldBeNil)
			So(prepared, ShouldNotBeNil)
			So(prepared.OpportunitySlot, ShouldBeTrue)
			So(prepared.Fraction, ShouldBeGreaterThan, 0)
		})
	})
}

func TestPrepareActionExitQuantity(t *testing.T) {
	Convey("Given an open position and a settle action", t, func() {
		holdings := logic.NewHoldings()
		holdings.SetPosition("BTC/USD", 0.5, 0.8, false)

		action := &logic.Action{
			Type:     logic.ActionSettlePosition,
			Side:     trading.Sell,
			Symbol:   "BTC/USD",
			Fraction: 1.0,
		}

		prepared, err := prepareAction(
			context.Background(),
			holdings,
			logic.EntrySlotOccupancyFromHoldings(holdings),
			action,
			nil,
			config.TradingConfig{},
			config.ThresholdConfig{},
			nil,
			0,
		)

		Convey("It should fill exit quantity from holdings", func() {
			So(err, ShouldBeNil)
			So(prepared, ShouldNotBeNil)
			So(prepared.Quantity, ShouldEqual, 0.5)
		})
	})
}
