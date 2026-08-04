package types

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Stoploss regulates one open lot from direct ticker updates forwarded by the
owning Position.
*/
type Stoploss struct {
	ctx    context.Context
	cancel context.CancelFunc
	Status Status           `json:"status"`
	Symbol string           `json:"symbol"`
	Entry  *decimal.Decimal `json:"entry"`
	Peak   *decimal.Decimal `json:"peak"`
	Mark   *decimal.Decimal `json:"mark"`
	Floor  *decimal.Decimal `json:"floor"`
}

/*
NewStoploss constructs an armed regulator from the position's initial entry
basis. A realized fill can rebind it before live tickers take over the mark.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	entry *decimal.Decimal,
	mark *decimal.Decimal,
) *Stoploss {
	errnie.Info("creating stoploss")

	ctx, cancel := context.WithCancel(ctx)

	stoploss := &Stoploss{
		ctx:    ctx,
		cancel: cancel,
		Symbol: symbol,
		Status: PENDING,
		Entry:  entry,
		Mark:   mark,
	}

	stoploss.evaluate()
	stoploss.Status = ARMED
	return stoploss
}

/*
Update advances the independent stop state from live ticker bids.
*/
func (stoploss *Stoploss) Update(ticker kraken.TickerData) error {
	if ticker.Bid == nil {
		return nil
	}

	stoploss.Mark = ticker.Bid
	stoploss.evaluate()

	if stoploss.Floor != nil && ticker.Bid.Cmp(stoploss.Floor) <= 0 {
		stoploss.Status = TRIGGERED
	}

	return nil
}

/*
evaluate seeds Peak and Floor on first call, then raises them only when the
live bid is above the break-even price. This ensures the trailing stop only
locks in profit once the round-trip fee cost has been recovered — any upward
movement while still below break-even leaves the floor exactly where it was.
*/
func (stoploss *Stoploss) evaluate() {
	if stoploss.Peak == nil {
		stoploss.Peak = stoploss.Mark
	}

	if stoploss.Floor == nil {
		stoploss.Floor = decimal.ExactMul(
			stoploss.Peak, decimal.NewFromFloat64(0.98),
		)

		return
	}

	// Only raise the floor when the position is actually profitable after fees.
	if stoploss.Entry != nil && stoploss.Mark.Cmp(stoploss.Entry) <= 0 {
		return
	}

	if stoploss.Mark.Cmp(stoploss.Peak) > 0 {
		stoploss.Peak = stoploss.Mark

		stoploss.Floor = decimal.ExactMul(
			stoploss.Peak, decimal.NewFromFloat64(0.98),
		)
	}
}

/*
Close cancels the regulator context and delegates exit to the same callback so
manual close and floor breach share one order path.
*/
func (stoploss *Stoploss) Close() (err error) {
	if stoploss.cancel != nil {
		stoploss.cancel()
	}

	return err
}
