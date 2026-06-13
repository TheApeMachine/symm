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

func executableEntryMeasurements() []logic.Measurement {
	return []logic.Measurement{
		{
			Source:          logic.SourcePrediction,
			Symbol:          "BTC/USD",
			Price:           50_000,
			Strength:        0.8,
			Volume:          1,
			Spread:          1,
			Elapsed:         1,
			Category:        logic.CategoryForecastEdge,
			Confidence:      0.8,
			Surprise:        1,
			ExpectedMoveBps: 80,
			EdgeConfidence:  0.7,
			Position:        logic.PositionTypeLong,
			DecisionGrade:   logic.DecisionGradeEdgeProvider,
		},
		{
			Source:          logic.SourceHawkes,
			Symbol:          "BTC/USD",
			Price:           50_000,
			Strength:        1,
			Volume:          1,
			Spread:          1,
			Elapsed:         1,
			Category:        logic.CategoryFrenzy,
			Confidence:      0.9,
			Surprise:        1,
			ExpectedMoveBps: 80,
			EdgeConfidence:  0.9,
			Position:        logic.PositionTypeLong,
			DecisionGrade:   logic.DecisionGradeExecutable,
		},
		{
			Source:          logic.SourceCVD,
			Symbol:          "BTC/USD",
			Price:           50_000,
			Strength:        0.8,
			Volume:          1,
			Spread:          1,
			Elapsed:         1,
			Category:        logic.CategoryAggressiveDrive,
			Confidence:      0.85,
			Surprise:        0.9,
			ExpectedMoveBps: 80,
			EdgeConfidence:  0.85,
			Position:        logic.PositionTypeLong,
			DecisionGrade:   logic.DecisionGradeExecutable,
		},
	}
}

func TestPrepareActionEntrySizing(t *testing.T) {
	Convey("Given an empty book and a buy action", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		holdings := logic.NewHoldings()
		measurements := executableEntryMeasurements()

		action := &logic.Action{
			Type: logic.ActionMarket,
			Side: trading.Buy,
		}

		tradingConfig := config.TradingConfig{
			Model:                  "paper",
			MaxConcurrentPositions: 4,
			OpportunitySlotCount:   2,
		}

		thresholdCtx := logic.NewThresholdContext(0.55, 0, 0)

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
				thresholdCtx,
				capitalProvider,
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

		thresholdCtx := logic.NewThresholdContext(0.55, 0, 0)

		capitalProvider, providerErr := trader.NewStaticCapitalProvider(200)

		So(providerErr, ShouldBeNil)

		pumpMeasurements := []logic.Measurement{
			{
				Source:          logic.SourcePrediction,
				Symbol:          "CELO/USD",
				Price:           0.25,
				Strength:        0.8,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        logic.CategoryForecastEdge,
				Confidence:      0.8,
				Surprise:        1,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.7,
				Position:        logic.PositionTypeLong,
				DecisionGrade:   logic.DecisionGradeEdgeProvider,
			},
			{
				Source:          logic.SourcePumpDump,
				Symbol:          "CELO/USD",
				Price:           0.25,
				Strength:        1.2,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        logic.CategoryVerticalIgnition,
				Confidence:      0.9,
				Surprise:        1.4,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.9,
				Position:        logic.PositionTypeLong,
				DecisionGrade:   logic.DecisionGradeExecutable,
			},
		}

		plainMeasurements := []logic.Measurement{
			{
				Source:          logic.SourcePrediction,
				Symbol:          "XLM/USD",
				Price:           0.12,
				Strength:        0.6,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        logic.CategoryForecastEdge,
				Confidence:      0.6,
				Surprise:        0.8,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.6,
				Position:        logic.PositionTypeLong,
				DecisionGrade:   logic.DecisionGradeEdgeProvider,
			},
			{
				Source:          logic.SourceHawkes,
				Symbol:          "XLM/USD",
				Price:           0.12,
				Strength:        0.4,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        logic.CategoryLaminar,
				Confidence:      0.6,
				Surprise:        0.8,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.6,
				Position:        logic.PositionTypeLong,
				DecisionGrade:   logic.DecisionGradeExecutable,
			},
		}

		Convey("It should reject a regular entry", func() {
			prepared, err := prepareAction(
				context.Background(),
				holdings,
				logic.EntrySlotOccupancyFromHoldings(holdings),
				&logic.Action{Type: logic.ActionMarket, Side: trading.Buy},
				plainMeasurements,
				tradingConfig,
				thresholdCtx,
				capitalProvider,
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
				thresholdCtx,
				capitalProvider,
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
			logic.ThresholdContext{},
			nil,
		)

		Convey("It should fill exit quantity from holdings", func() {
			So(err, ShouldBeNil)
			So(prepared, ShouldNotBeNil)
			So(prepared.Quantity, ShouldEqual, 0.5)
		})
	})
}

func TestPrepareExitActionUsesBranchAttribution(t *testing.T) {
	Convey("Given a thesis invalidation attribution", t, func() {
		holdings := logic.NewHoldings()
		holdings.SetPosition("BTC/USD", 1, 0.8, false)

		action := &logic.Action{
			Type:           logic.ActionSettlePosition,
			Side:           trading.Sell,
			Symbol:         "BTC/USD",
			Fraction:       0.2,
			ReasonSource:   logic.SourceExhaustion,
			ReasonCategory: logic.CategoryMechanicalCollapse,
			ExitTier:       logic.ExitTierThesisInvalidation,
		}

		prepared, err := prepareExitAction(action, holdings, nil)

		Convey("It should fully liquidate regardless of benign measurements", func() {
			So(err, ShouldBeNil)
			So(prepared.Quantity, ShouldEqual, 1)
		})
	})
}
