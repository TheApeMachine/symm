package broker

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestPreTradeRiskGateRejectsUnsafeQuotes(test *testing.T) {
	tradingConfig := config.TradingConfig{
		Model:                  "paper",
		PositionFraction:       0.2,
		PrimarySlotCount:       1,
		PrimarySlotFraction:    0.2,
		SecondarySlotFraction:  0.1,
		MaxConcurrentPositions: 1,
		MaxQuoteAge:            time.Second,
		MaxSpreadBps:           100,
		MaxSlippageBps:         50,
		OrderAckTimeout:        time.Second,
		EntryTransitTTL:        time.Second,
	}
	riskGate, gateErr := NewPreTradeRiskGate(tradingConfig)

	if gateErr != nil {
		test.Fatalf("risk gate: %v", gateErr)
	}

	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     trading.Buy,
		Symbol:   "BTC/USD",
		Price:    100,
		Quantity: 0.1,
	}
	now := time.Now().UTC()

	convey.Convey("Given a stale quote", test, func() {
		quote := QuoteSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99.9,
			Ask:        100.1,
			ObservedAt: now.Add(-2 * time.Second),
		}

		convey.So(riskGate.Validate(action, quote, now), convey.ShouldNotBeNil)
	})

	convey.Convey("Given a wide quote", test, func() {
		quote := QuoteSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99,
			Ask:        101,
			ObservedAt: now,
		}

		convey.So(riskGate.Validate(action, quote, now), convey.ShouldNotBeNil)
	})

	convey.Convey("Given excessive projected slippage", test, func() {
		quote := QuoteSnapshot{
			Symbol:     "BTC/USD",
			Bid:        100,
			Ask:        100.7,
			ObservedAt: now,
		}

		convey.So(riskGate.Validate(action, quote, now), convey.ShouldNotBeNil)
	})
}

func TestDeskLoadQuoteEvictsExpiredSnapshot(test *testing.T) {
	convey.Convey("Given an expired quote in the desk map", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		now := time.Now().UTC()
		desk.persistQuote(QuoteSnapshot{
			Symbol:     "ETH/USD",
			Bid:        3000,
			Ask:        3001,
			ObservedAt: now.Add(-time.Minute),
		})

		_, loadErr := desk.loadQuote("ETH/USD", now)

		convey.Convey("It should remove the snapshot and report quote required", func() {
			convey.So(loadErr, convey.ShouldNotBeNil)
			convey.So(loadErr.Error(), convey.ShouldContainSubstring, "quote for \"ETH/USD\" is required")
			convey.So(loadErr.Error(), convey.ShouldNotContainSubstring, "stale")

			_, stillStored := desk.quotes.Load("ETH/USD")
			convey.So(stillStored, convey.ShouldBeFalse)
		})
	})
}

func TestDeskPreTradeRiskGateBlocksUnsafeEntry(test *testing.T) {
	convey.Convey("Given a desk with a wide current quote", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		desk.onTicker(&market.TickerUpdate{
			Symbol: "BTC/USD",
			Bid:    99,
			Ask:    104,
			Last:   101,
		})

		desk.onAction(&logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Price:    101,
			Quantity: 0.1,
		})

		actionOpen := false
		desk.actions.Range(func(any, any) bool {
			actionOpen = true
			return false
		})

		convey.So(actionOpen, convey.ShouldBeFalse)
	})
}

func TestDeskPreTradeRiskGateAllowsRiskReducingExit(test *testing.T) {
	convey.Convey("Given a desk with a sell action for an existing stop", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		stopLoss, stopErr := NewStopLoss(
			"BTC/USD",
			0.1,
			100,
			0,
			desk.exitConfig.Load(),
		)

		convey.So(stopErr, convey.ShouldBeNil)
		desk.stops.Store("BTC/USD", stopLoss)

		desk.onAction(&logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Sell,
			Symbol:   "BTC/USD",
			Quantity: 0.1,
		})

		actionOpen := false
		desk.actions.Range(func(any, any) bool {
			actionOpen = true
			return false
		})

		convey.So(actionOpen, convey.ShouldBeTrue)
	})
}
