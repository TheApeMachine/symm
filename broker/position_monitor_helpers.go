package broker

import (
	"errors"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
)

func (desk *Desk) publishPositions() {
	if desk == nil || desk.positions == nil {
		return
	}

	frame := desk.positions.Snapshot()
	desk.recordExposure(frame)

	errnie.Error(desk.bus.Send(
		internal.ChannelUI,
		"positions",
		frame,
	))
}

func balanceInventory(
	balances user.Balances,
	currency string,
) map[string]float64 {
	if len(balances.Inventory) > 0 {
		return balances.Inventory
	}

	inventory := make(map[string]float64)

	for _, asset := range balances.Asset {
		assetName := strings.ToUpper(strings.TrimSpace(asset.Asset))

		if asset.Balance <= 0 || assetName == "" {
			continue
		}

		// Kraken prefixes fiat currency asset names with "Z" (e.g. ZUSD for USD).
		// Skip both plain currency and Kraken-prefixed fiat assetName values.
		if assetName == currency || assetName == "Z"+currency {
			continue
		}

		inventory[assetName] = asset.Balance
	}

	return inventory
}

func balanceMark(
	symbol string,
	balances user.Balances,
	fallback float64,
) float64 {
	if balances.Marks == nil {
		return fallback
	}

	mark := balances.Marks[symbol]

	if mark <= 0 {
		return fallback
	}

	return mark
}

func normalizedCurrency(value string, fallback string) string {
	currency := strings.ToUpper(strings.TrimSpace(value))

	if currency != "" {
		return currency
	}

	if fallback != "" {
		return fallback
	}

	return "USD"
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func longExitPriceFromTicker(ticker *market.TickerUpdate) (float64, error) {
	if ticker == nil {
		return 0, errnie.Error(errors.New("broker: ticker is required"))
	}

	if ticker.Bid > 0 {
		return ticker.Bid, nil
	}

	return ticker.ResolvePrice()
}

func positionFramesEqual(
	left PositionMonitorFrame,
	right PositionMonitorFrame,
) bool {
	if left.Cash != right.Cash ||
		left.Currency != right.Currency ||
		left.OpenPositions != right.OpenPositions ||
		left.PricedPositions != right.PricedPositions ||
		left.ExitValue != right.ExitValue ||
		left.ExitBalance != right.ExitBalance ||
		left.LiquidationBalance != right.LiquidationBalance ||
		len(left.Positions) != len(right.Positions) {
		return false
	}

	for index := range left.Positions {
		if !positionStatesEqual(left.Positions[index], right.Positions[index]) {
			return false
		}
	}

	return true
}

func positionStatesEqual(left PositionState, right PositionState) bool {
	return left.Symbol == right.Symbol &&
		left.Quantity == right.Quantity &&
		left.AverageEntry == right.AverageEntry &&
		left.Mark == right.Mark &&
		left.ExitValue == right.ExitValue &&
		left.Unrealized == right.Unrealized &&
		left.UnrealizedPct == right.UnrealizedPct &&
		left.Priced == right.Priced &&
		left.ExitFeeRate == right.ExitFeeRate &&
		left.PeakPrice == right.PeakPrice &&
		left.StopPrice == right.StopPrice &&
		left.Offset == right.Offset &&
		left.MarkSource == right.MarkSource &&
		left.ObservedAt.Equal(right.ObservedAt)
}
