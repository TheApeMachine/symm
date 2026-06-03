package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func executionScenarios() []Scenario {
	return []Scenario{
		{
			ID:         "execution.limit_fill_inventory",
			Name:       "Limit entry fills through paper matcher and updates inventory",
			RunTimeout: 15 * time.Second,
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 100, 30, 100.5, 30)
			},
			PreDirectTickers: []market.TickerUpdate{{
				Symbol: testSymbolPrimary,
				Last:   100,
				Bid:    99.5,
				Ask:    100.5,
			}},
			PreDirectBooks: []market.Book{{
				Symbol: testSymbolPrimary,
				Bids:   []market.BookLevel{{Price: 100, Qty: 30}},
				Asks:   []market.BookLevel{{Price: 100.5, Qty: 30}},
			}},
			DirectMeasurements: playbookLiquidityVacuumMeasurements(testSymbolPrimary, 100),
			PostReplayTrades: []market.TradeUpdate{
				{Symbol: testSymbolPrimary, Side: "sell", Price: 100, Qty: 35},
			},
			PostOrderTickers: []market.TickerUpdate{{
				Symbol: testSymbolPrimary,
				Last:   99.75,
				Bid:    99.5,
				Ask:    99.5,
			}},
			PostOrderBooks: []market.Book{{
				Symbol: testSymbolPrimary,
				Bids:   []market.BookLevel{{Price: 100, Qty: 0.0001}},
				Asks:   []market.BookLevel{{Price: 99.5, Qty: 10}},
			}},
			PostOrderDelay:  2 * time.Second,
			PostReplayDelay: 200 * time.Millisecond,
			PostReplayPace:  100 * time.Millisecond,
			SettleDelay:     5 * time.Second,
			Checks: []ScenarioCheck{
				{
					ID:   "execution.engine",
					Name: "Trader did not fail before scenario cancel",
					Evaluate: func(_ TapeSnapshot, engineErr error) (bool, string, map[string]any) {
						if engineErr == nil {
							return true, "ok", nil
						}

						if errors.Is(engineErr, context.Canceled) {
							return true, "ok", nil
						}

						if strings.Contains(engineErr.Error(), "context canceled") {
							return true, "ok", nil
						}

						return false, engineErr.Error(), nil
					},
				},
				checkActionType("execution.action", "Limit entry action observed on raw",
					perspectives.ActionLimit, testSymbolPrimary),
				checkFillEvent("execution.fill", "Trader publishes fill frame", testSymbolPrimary),
				checkInventory("execution.inventory", "Paper wallet holds SYN base inventory", "SYN", 0.00001),
				{
					ID:   "execution.cash",
					Name: "Cash balance decreases after entry fill",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						pass := snapshot.lastWalletBalance() > 0 && snapshot.lastWalletBalance() < 200

						return pass, formatCash(snapshot.lastWalletBalance()), map[string]any{
							"balance": snapshot.lastWalletBalance(),
						}
					},
				},
			},
		},
		{
			ID:   "execution.settle_reduces_inventory",
			Name: "Settle action exits held inventory through paper desk",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 30, 100.5, 30)
				builder.AppendBuyTrades(testSymbolPrimary, 8, 100.5, 0.05)
			},
			HoldingSymbols:     []string{testSymbolPrimary},
			DirectMeasurements: playbookMedianDepthExitMeasurements(testSymbolPrimary, 100),
			SettleDelay:        3 * time.Second,
			Checks: []ScenarioCheck{
				checkActionType("execution.exit", "Settle action observed on raw",
					perspectives.ActionSettlePosition, testSymbolPrimary),
			},
		},
	}
}

func formatCash(balance float64) string {
	return "balance=" + formatFloat(balance)
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%.4f", value)
}
