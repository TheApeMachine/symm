package broker

import (
	"fmt"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
StopLoss is a ratcheting trailing stop managed by the desk for one long inventory line.
*/
type StopLoss struct {
	Symbol     string
	Quantity   float64
	EntryPrice float64
	PeakPrice  float64
	StopPrice  float64
	Offset     float64
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
	}, nil
}

/*
Evaluate reports whether the ticker price has crossed the current stop level.
*/
func (stopLoss *StopLoss) Evaluate(ticker *market.TickerUpdate) (bool, error) {
	price, err := ticker.ResolvePrice()

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
	price, err := ticker.ResolvePrice()

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

	return nil
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
