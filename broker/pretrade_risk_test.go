package broker

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
)

func testTradingConfig() config.TradingConfig {
	return config.TradingConfig{
		Model:                  "paper",
		MaxConcurrentPositions: 1,
		MaxQuoteAge:            time.Second,
		OrderAckTimeout:        time.Second,
		EntryTransitTTL:        time.Second,
	}
}

func seedSpreadHistory(
	riskGate *TickerPreTradeRiskGate,
	symbol string,
	bid float64,
	ask float64,
	now time.Time,
) {
	SeedSpreadHistory(riskGate, symbol, bid, ask, now)
}

func TestPreTradeRiskGateRejectsUnsafeQuotes(test *testing.T) {
	testconfig.Load(test)

	tradingConfig := testTradingConfig()
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

	convey.Convey("Given a spread anomaly after a tight baseline", test, func() {
		seedSpreadHistory(riskGate, "BTC/USD", 99.99, 100.01, now)

		quote := QuoteSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99,
			Ask:        101,
			Last:       100,
			ObservedAt: now,
		}

		convey.So(riskGate.Validate(action, quote, now), convey.ShouldNotBeNil)
	})

	convey.Convey("Given excessive projected slippage after a tight baseline", test, func() {
		seedSpreadHistory(riskGate, "ETH/USD", 99.99, 100.01, now)

		slippageAction := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "ETH/USD",
			Price:    100,
			Quantity: 0.1,
		}
		quote := QuoteSnapshot{
			Symbol:     "ETH/USD",
			Bid:        100,
			Ask:        100.7,
			Last:       100,
			ObservedAt: now,
		}

		convey.So(riskGate.Validate(slippageAction, quote, now), convey.ShouldNotBeNil)
	})
}

func TestDeskLoadQuoteEvictsExpiredSnapshot(test *testing.T) {
	convey.Convey("Given an expired quote in the desk map", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, _ := newTestDesk(test, ctx, pool)

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
	testconfig.Load(test)

	convey.Convey("Given a desk with a spread blowout after a tight baseline", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(test, ctx, pool)

		defer func() { _ = desk.Close() }()

		now := time.Now().UTC()
		gate, gateOK := desk.riskGate.(*TickerPreTradeRiskGate)

		convey.So(gateOK, convey.ShouldBeTrue)
		seedSpreadHistory(gate, "BTC/USD", 99.99, 100.01, now)
		touchRegistry.SeedTouch(symmmarket.TouchSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99,
			Ask:        104,
			Last:       101,
			ObservedAt: now,
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
		desk, _ := newTestDesk(test, ctx, pool)

		defer func() { _ = desk.Close() }()

		stopLoss, stopErr := NewStopLoss(
			"BTC/USD",
			0.1,
			100,
			0,
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
