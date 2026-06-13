package broker

import (
	"sort"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
)

/*
PositionMonitor owns the backend position economics published to the dashboard.
*/
type PositionMonitor struct {
	currency  string
	cash      float64
	positions map[string]*PositionState
}

type PositionState struct {
	Symbol        string    `json:"symbol"`
	Quantity      float64   `json:"qty"`
	AverageEntry  float64   `json:"avg_entry"`
	Mark          float64   `json:"mark"`
	ExitValue     float64   `json:"exit_value"`
	Unrealized    float64   `json:"unrealized"`
	UnrealizedPct float64   `json:"unrealized_pct"`
	Priced        bool      `json:"priced"`
	ExitFeeRate   float64   `json:"exit_fee_rate,omitempty"`
	PeakPrice     float64   `json:"peak_price,omitempty"`
	StopPrice     float64   `json:"stop_price,omitempty"`
	Offset        float64   `json:"offset,omitempty"`
	MarkSource    string    `json:"mark_source,omitempty"`
	ObservedAt    time.Time `json:"observed_at,omitempty"`
}

type PositionMonitorFrame struct {
	Type                string          `json:"type"`
	Currency            string          `json:"currency"`
	Cash                float64         `json:"cash"`
	OpenPositions       int             `json:"open_positions"`
	PricedPositions     int             `json:"priced_positions"`
	ExitValue           float64         `json:"exit_value"`
	ExitBalance         float64         `json:"exit_balance"`
	LiquidationBalance  float64         `json:"liquidation_balance"`
	LiquidationComplete bool            `json:"liquidation_complete"`
	InProfit            bool            `json:"in_profit"`
	Positions           []PositionState `json:"positions"`
}

func NewPositionMonitor() *PositionMonitor {
	return &PositionMonitor{
		currency:  "USD",
		positions: make(map[string]*PositionState),
	}
}

func (positionMonitor *PositionMonitor) ApplyBalance(
	balances user.Balances,
) bool {
	if positionMonitor == nil {
		return false
	}

	before := positionMonitor.Snapshot()
	currency := normalizedCurrency(balances.Currency, positionMonitor.currency)
	inventory := balanceInventory(balances, currency)
	seen := make(map[string]bool, len(inventory))

	positionMonitor.currency = currency
	positionMonitor.cash = balances.Balance

	for base, quantity := range inventory {
		if quantity <= 0 {
			continue
		}

		symbol := base + "/" + currency
		position := positionMonitor.position(symbol)
		position.Symbol = symbol
		position.Quantity = quantity
		position.AverageEntry = balances.AvgEntry[base]
		position.ExitFeeRate = balances.ExitFeeRate[base]
		position.Mark = balanceMark(symbol, balances, position.Mark)
		exitValue, exitOK := balances.Expected[base]
		unrealized, unrealizedOK := balances.Unrealized[base]
		position.applyBackendEconomics(
			exitValue,
			unrealized,
			exitOK && unrealizedOK,
			"balances",
		)
		seen[symbol] = true
	}

	for symbol := range positionMonitor.positions {
		if seen[symbol] {
			continue
		}

		delete(positionMonitor.positions, symbol)
	}

	return !positionFramesEqual(before, positionMonitor.Snapshot())
}

func (positionMonitor *PositionMonitor) ApplyTicker(
	ticker *market.TickerUpdate,
) bool {
	if positionMonitor == nil || ticker == nil || ticker.Symbol == "" {
		return false
	}

	position := positionMonitor.positions[ticker.Symbol]

	if position == nil {
		return false
	}

	price, err := longExitPriceFromTicker(ticker)

	if errnie.Error(err) != nil {
		return false
	}

	before := *position
	position.applyMark(price, "ticker_monitor", ticker.Timestamp)

	return !positionStatesEqual(before, *position)
}

func (positionMonitor *PositionMonitor) ApplyStopTicker(
	stopLoss *StopLoss,
	ticker *market.TickerUpdate,
) bool {
	if positionMonitor == nil || stopLoss == nil || ticker == nil {
		return false
	}

	price, err := longExitPriceFromTicker(ticker)

	if errnie.Error(err) != nil {
		return false
	}

	position := positionMonitor.position(stopLoss.Symbol)
	before := *position
	position.applyStop(stopLoss, price, ticker.Timestamp)

	return !positionStatesEqual(before, *position)
}

func (positionMonitor *PositionMonitor) ApplyStop(stopLoss *StopLoss) bool {
	if positionMonitor == nil || stopLoss == nil {
		return false
	}

	position := positionMonitor.position(stopLoss.Symbol)
	before := *position
	position.applyStop(stopLoss, stopLoss.EntryPrice, time.Time{})

	return !positionStatesEqual(before, *position)
}

func (positionMonitor *PositionMonitor) Reduce(
	symbol string,
	quantity float64,
) bool {
	if positionMonitor == nil || symbol == "" || quantity <= 0 {
		return false
	}

	position := positionMonitor.positions[symbol]

	if position == nil {
		return false
	}

	before := positionMonitor.Snapshot()
	position.Quantity -= quantity

	if position.Quantity <= 0 {
		delete(positionMonitor.positions, symbol)
		return true
	}

	position.applyMark(position.Mark, position.MarkSource, position.ObservedAt)

	return !positionFramesEqual(before, positionMonitor.Snapshot())
}

func (positionMonitor *PositionMonitor) Snapshot() PositionMonitorFrame {
	frame := PositionMonitorFrame{
		Type:      "positions",
		Currency:  positionMonitor.currency,
		Cash:      positionMonitor.cash,
		Positions: make([]PositionState, 0, len(positionMonitor.positions)),
	}

	for _, position := range positionMonitor.positions {
		if position.Quantity <= 0 {
			continue
		}

		frame.Positions = append(frame.Positions, *position)

		if !position.Priced {
			continue
		}

		frame.PricedPositions++
		frame.ExitValue += position.ExitValue
		frame.ExitBalance += position.Unrealized
	}

	sort.Slice(frame.Positions, func(leftIndex, rightIndex int) bool {
		return frame.Positions[leftIndex].Symbol <
			frame.Positions[rightIndex].Symbol
	})

	frame.OpenPositions = len(frame.Positions)
	frame.LiquidationBalance = frame.Cash + frame.ExitValue
	frame.LiquidationComplete = frame.PricedPositions == frame.OpenPositions
	frame.InProfit = frame.ExitBalance >= 0

	return frame
}

func (positionMonitor *PositionMonitor) position(
	symbol string,
) *PositionState {
	position := positionMonitor.positions[symbol]

	if position != nil {
		return position
	}

	position = &PositionState{Symbol: symbol}
	positionMonitor.positions[symbol] = position

	return position
}

func (position *PositionState) applyBackendEconomics(
	exitValue float64,
	unrealized float64,
	available bool,
	markSource string,
) {
	if !available || !finite(exitValue) || !finite(unrealized) {
		position.clearPricing()
		position.MarkSource = ""
		return
	}

	position.ExitValue = exitValue
	position.Unrealized = unrealized
	position.Priced = true
	position.MarkSource = markSource
	position.updateUnrealizedPct()
}

func (position *PositionState) applyStop(
	stopLoss *StopLoss,
	mark float64,
	observedAt time.Time,
) {
	position.Symbol = stopLoss.Symbol
	position.Quantity = stopLoss.Quantity
	position.AverageEntry = stopLoss.EntryPrice
	position.PeakPrice = stopLoss.PeakPrice
	position.StopPrice = stopLoss.StopPrice
	position.Offset = stopLoss.Offset
	position.applyMark(mark, "stop_monitor", observedAt)
}

func (position *PositionState) applyMark(
	mark float64,
	markSource string,
	observedAt time.Time,
) {
	if !finite(mark) || mark <= 0 || position.Quantity <= 0 ||
		position.AverageEntry <= 0 {
		position.clearPricing()
		return
	}

	exitValue := position.Quantity * mark
	exitFeeRate := position.ExitFeeRate

	if exitFeeRate < 0 {
		exitFeeRate = 0
	}

	if exitFeeRate >= 1 {
		position.clearPricing()
		return
	}

	if exitFeeRate > 0 {
		exitFeeMultiplier := 1 - exitFeeRate

		if exitFeeMultiplier <= 0 {
			position.clearPricing()
			return
		}

		exitValue *= exitFeeMultiplier
	}

	position.Mark = mark
	position.ExitValue = exitValue
	position.Unrealized = position.ExitValue -
		(position.Quantity * position.AverageEntry)
	position.Priced = true
	position.MarkSource = markSource
	position.ObservedAt = observedAt
	position.updateUnrealizedPct()
}

func (position *PositionState) clearPricing() {
	position.Priced = false
	position.ExitValue = 0
	position.Unrealized = 0
	position.UnrealizedPct = 0
}

func (position *PositionState) updateUnrealizedPct() {
	entryValue := position.Quantity * position.AverageEntry

	if entryValue <= 0 {
		position.UnrealizedPct = 0
		return
	}

	position.UnrealizedPct = (position.Unrealized / entryValue) * 100
}
