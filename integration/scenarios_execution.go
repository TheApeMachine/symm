package integration

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

func executionScenarios() []Scenario {
	return []Scenario{
		{
			ID:   "execution.limit_fill_inventory",
			Name: "Limit entry fills through paper matcher and updates inventory",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 30, 100.5, 30)
				builder.AppendSellTrade(testSymbolPrimary, 100.5, 0.05)
			},
			DirectMeasurements: playbookLiquidityVacuumMeasurements(testSymbolPrimary, 100),
			SettleDelay:        2500 * time.Millisecond,
			Checks: []ScenarioCheck{
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
			SettleDelay:        2500 * time.Millisecond,
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
