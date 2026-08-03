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
	ctx         context.Context
	cancel      context.CancelFunc
	Status      Status           `json:"status"`
	Symbol      string           `json:"symbol"`
	Entry       *decimal.Decimal `json:"entry"`
	Peak        *decimal.Decimal `json:"peak"`
	Mark        *decimal.Decimal `json:"mark"`
	Floor       *decimal.Decimal `json:"floor"`
	breachCount int
	shockTicks  int
}

/*
NewStoploss constructs an active regulator with default weight. Entry is bound
later via Bind when the position opens.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	mark *decimal.Decimal,
) *Stoploss {
	errnie.Info("creating stoploss")

	ctx, cancel := context.WithCancel(ctx)

	stoploss := &Stoploss{
		ctx:    ctx,
		cancel: cancel,
		Symbol: symbol,
		Entry:  mark,
		Mark:   mark,
		Status: PENDING,
	}

	stoploss.evaluate()

	stoploss.Status = ARMED
	return stoploss
}

/*
update advances the independent stop state from live ticker bids.
*/
func (stoploss *Stoploss) Update(ticker kraken.TickerData) error {
	if ticker.Bid == nil {
		return nil
	}

	previous := stoploss.Mark
	stoploss.Mark = ticker.Bid

	if previous != nil && previous.Sign() > 0 {
		shockThreshold := decimal.ExactMul(previous, decimal.NewFromFloat64(0.8))

		if shockThreshold != nil && ticker.Bid.Cmp(shockThreshold) < 0 {
			stoploss.shockTicks = 6
		}
	}

	if stoploss.shockTicks > 0 {
		stoploss.shockTicks--
		stoploss.breachCount = 0
		stoploss.evaluate()

		return nil
	}

	if stoploss.Floor != nil && ticker.Bid.Cmp(stoploss.Floor) > 0 {
		stoploss.breachCount = 0
	}

	if stoploss.Floor != nil && ticker.Bid.Cmp(stoploss.Floor) <= 0 {
		stoploss.breachCount++

		if stoploss.breachCount < 6 {
			stoploss.evaluate()
			return nil
		}

		stoploss.Status = TRIGGERED
	}

	stoploss.evaluate()

	return nil
}

/*
evaluate checks the current the current Peek, and potentially re-sets
the Peak and the Floor if the current Mark is higher than the Peak.
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
