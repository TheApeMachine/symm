package broker

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

type ProtectiveStopState string

const (
	StopArmed         ProtectiveStopState = "armed"
	StopTriggered     ProtectiveStopState = "triggered"
	StopExitSubmitted ProtectiveStopState = "exit_submitted"
	StopExitConfirmed ProtectiveStopState = "exit_confirmed"
	StopNeedsRepair   ProtectiveStopState = "needs_repair"
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
	Offset          float64
	State           ProtectiveStopState
	TriggeredAt     time.Time
	ExitSubmittedAt time.Time
	ExitConfirmedAt time.Time
}

func NewStopLoss(
	symbol string,
	quantity float64,
	entryPrice float64,
	spreadBps float64,
	exitConfig config.ExitConfig,
) (*StopLoss, error) {
	if symbol == "" || quantity <= 0 || entryPrice <= 0 {
		return nil, fmt.Errorf("broker: invalid stop loss params")
	}

	offset := assessTrailOffset(exitConfig, spreadBps)

	return &StopLoss{
		Symbol:     symbol,
		Quantity:   quantity,
		EntryPrice: entryPrice,
		PeakPrice:  entryPrice,
		StopPrice:  entryPrice * (1 - offset),
		Offset:     offset,
		State:      StopArmed,
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
func (stopLoss *StopLoss) WidenOffsetFromTicker(
	ticker *market.TickerUpdate,
	exitConfig config.ExitConfig,
) {
	if !stopLoss.CanMonitor() {
		return
	}

	offset := assessTrailOffset(exitConfig, spreadBpsFromTicker(ticker))

	if offset <= stopLoss.Offset {
		return
	}

	stopLoss.Offset = offset
	stopLoss.StopPrice = stopLoss.PeakPrice * (1 - offset)
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
	stopLoss.StopPrice = price * (1 - stopLoss.Offset)

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

	stopLoss.Quantity -= quantity

	if stopLoss.Quantity > 0 {
		return false
	}

	_ = stopLoss.Close()

	return true
}

func ProtectiveStopStateFromString(value string) ProtectiveStopState {
	return ProtectiveStopState(strings.ToLower(strings.TrimSpace(value)))
}

func assessTrailOffset(exitConfig config.ExitConfig, spreadBps float64) float64 {
	offset := exitConfig.Float("trail_default", 0.015)
	spreadScale := exitConfig.SpreadScale

	if spreadScale > 0 && spreadBps > 0 {
		offset += (spreadBps / 10000) * spreadScale
	}

	floor := exitConfig.Float("stop_floor", 0.012)

	if floor > 0 && offset < floor {
		return floor
	}

	return offset
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
