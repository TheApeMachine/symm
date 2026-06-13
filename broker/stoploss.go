package broker

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/ring"
)

type ProtectiveStopState string

const (
	StopArmed         ProtectiveStopState = "armed"
	StopTriggered     ProtectiveStopState = "triggered"
	StopExitSubmitted ProtectiveStopState = "exit_submitted"
	StopExitConfirmed ProtectiveStopState = "exit_confirmed"
	StopNeedsRepair   ProtectiveStopState = "needs_repair"
	StopUnknown       ProtectiveStopState = "unknown"
)

/*
StopLoss is a ratcheting trailing stop managed by the desk for one long inventory line.
*/
type StopLoss struct {
	Symbol          string
	Quantity        float64
	EntryPrice      float64
	PeakPrice       float64
	StopPrice       float64
	HardStopPrice   float64
	Offset          float64
	State           ProtectiveStopState
	TriggeredAt     time.Time
	ExitSubmittedAt time.Time
	ExitConfirmedAt time.Time
	Prices          ring.FloatRing
}

func NewStopLoss(
	symbol string,
	quantity float64,
	entryPrice float64,
	spreadBps float64,
) (*StopLoss, error) {
	if symbol == "" || quantity <= 0 || entryPrice <= 0 {
		return nil, fmt.Errorf("broker: invalid stop loss params")
	}

	offset := DeriveTrailOffset(spreadBps, 0)
	maxInitialRisk := DeriveMaxInitialRisk(offset, 0)

	hardStopPrice := entryPrice * (1 - maxInitialRisk)

	pricesRing := ring.NewFloatRing(24)
	pricesRing.Push(entryPrice)

	return &StopLoss{
		Symbol:        symbol,
		Quantity:      quantity,
		EntryPrice:    entryPrice,
		PeakPrice:     entryPrice,
		HardStopPrice: hardStopPrice,
		StopPrice:     effectiveStopPrice(entryPrice, entryPrice*(1-offset), hardStopPrice),
		Offset:        offset,
		State:         StopArmed,
		Prices:        pricesRing,
	}, nil
}

/*
Evaluate reports whether the ticker price has crossed the current stop level.
*/
func (stopLoss *StopLoss) Evaluate(ticker *market.TickerUpdate) (bool, error) {
	if !stopLoss.CanMonitor() {
		return false, nil
	}

	price, err := longExitPriceFromTicker(ticker)

	if err != nil {
		return false, err
	}

	return price <= stopLoss.StopPrice, nil
}

/*
WidenOffsetFromTicker loosens the trail when the tape spread widens.
*/
func (stopLoss *StopLoss) WidenOffsetFromTicker(ticker *market.TickerUpdate) {
	if !stopLoss.CanMonitor() {
		return
	}

	price, err := longExitPriceFromTicker(ticker)
	if err == nil && price > 0 {
		stopLoss.Prices.Push(price)
	}

	mean, stddev := stopLoss.Prices.MeanStdDev()
	volatilityMultiplier := 0.0

	if mean > 0 {
		volatilityMultiplier = stddev / mean
	}

	baseOffset := DeriveTrailOffset(spreadBpsFromTicker(ticker), volatilityMultiplier)
	offset := baseOffset * (1.0 + volatilityMultiplier)
	offset = ClampDerivedTrailOffset(offset, spreadBpsFromTicker(ticker), volatilityMultiplier)

	if offset <= stopLoss.Offset {
		return
	}

	stopLoss.Offset = offset
	trailStop := stopLoss.PeakPrice * (1 - offset)
	stopLoss.StopPrice = effectiveStopPrice(stopLoss.EntryPrice, trailStop, stopLoss.HardStopPrice)
}


func effectiveStopPrice(entryPrice, trailStop, hardStop float64) float64 {
	if hardStop <= 0 {
		return trailStop
	}

	if trailStop < hardStop {
		return hardStop
	}

	return trailStop
}

/*
Ratchet raises the peak and stop when price moves favorably for a long position.
*/
func (stopLoss *StopLoss) Ratchet(ticker *market.TickerUpdate) (bool, error) {
	if !stopLoss.CanMonitor() {
		return false, nil
	}

	price, err := longExitPriceFromTicker(ticker)

	if err != nil {
		return false, err
	}

	if price <= stopLoss.PeakPrice {
		return false, nil
	}

	stopLoss.PeakPrice = price
	trailStop := price * (1 - stopLoss.Offset)
	stopLoss.StopPrice = effectiveStopPrice(stopLoss.EntryPrice, trailStop, stopLoss.HardStopPrice)

	return true, nil
}

/*
Close clears desk-side stop state. Exchange orders are not used for paper trailing stops.
*/
func (stopLoss *StopLoss) Close() error {
	stopLoss.Quantity = 0
	stopLoss.State = StopExitConfirmed
	stopLoss.ExitConfirmedAt = time.Now().UTC()

	return nil
}

func (stopLoss *StopLoss) CanMonitor() bool {
	if stopLoss == nil {
		return false
	}

	state := stopLoss.State

	if state == "" {
		state = StopArmed
	}

	return state == StopArmed
}

func (stopLoss *StopLoss) MarkExitSubmitted() {
	if stopLoss == nil {
		return
	}

	stopLoss.State = StopExitSubmitted
	stopLoss.ExitSubmittedAt = time.Now().UTC()
}

func (stopLoss *StopLoss) MarkNeedsRepair() {
	if stopLoss == nil {
		return
	}

	stopLoss.State = StopNeedsRepair
}

func (stopLoss *StopLoss) MarkTriggered(observedAt time.Time) {
	if stopLoss == nil {
		return
	}

	stopLoss.State = StopTriggered
	stopLoss.TriggeredAt = observedAt
}

func (stopLoss *StopLoss) Reduce(quantity float64) bool {
	if stopLoss == nil || quantity <= 0 {
		return false
	}

	remaining := stopLoss.Quantity - quantity

	if remaining <= 0 {
		stopLoss.Quantity = 0
		_ = stopLoss.Close()

		return true
	}

	stopLoss.Quantity = remaining

	return false
}

func ProtectiveStopStateFromString(value string) ProtectiveStopState {
	normalized := strings.ToLower(strings.TrimSpace(value))

	switch normalized {
	case string(StopArmed):
		return StopArmed
	case string(StopTriggered):
		return StopTriggered
	case string(StopExitSubmitted):
		return StopExitSubmitted
	case string(StopExitConfirmed):
		return StopExitConfirmed
	case string(StopNeedsRepair):
		return StopNeedsRepair
	default:
		return StopUnknown
	}
}


func spreadBpsFromTicker(ticker *market.TickerUpdate) float64 {
	if ticker.Bid <= 0 || ticker.Ask <= ticker.Bid {
		return 0
	}

	mid := (ticker.Ask + ticker.Bid) / 2

	if mid <= 0 {
		return 0
	}

	return (ticker.Ask - ticker.Bid) / mid * 10000
}
