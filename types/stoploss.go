package types

import (
	"context"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Stoploss regulates one open lot.

It owns two price exits and two regime circuit breakers. The hard floor is the
maximum loss and fires on sight. The protected floor is the profit the position
has already earned and fires only on a breach that holds. A validated structural
break or an execution-noise band that has outgrown the entry regime exits before
either price line; ordinary forecast decay and drawdown remain diagnostic.

Both Peak and Floor are monotonic. A peak once reached was reachable, and
protection once armed is not handed back because volatility later widened.
*/
type Stoploss struct {
	ctx    context.Context
	cancel context.CancelFunc
	Status Status           `json:"status"`
	Symbol string           `json:"symbol"`
	Floor  *decimal.Decimal `json:"floor"`
	Mark   *decimal.Decimal `json:"mark"`
	Peak   *decimal.Decimal `json:"peak"`
}

/*
NewStoploss constructs a regulator for one lot from the geometry the entry was
sized under and whatever provisional basis is known before the fill.

The lines built here are estimates: the order has not crossed yet, so the entry
price is the ask the decision was priced at. RebindFill replaces them the
moment the venue says what was actually paid.
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
		Status: ARMED,
		Mark:   mark,
		Peak:   mark,
		Floor:  mark.Mul(decimal.NewFromFloat64(0.98)),
	}

	return stoploss
}

/*
Update the stoploss with the current mark price. If the mark is above the peak,
the peak is raised. If the mark is below the floor, the floor is lowered.
*/
func (stoploss *Stoploss) Update(mark *decimal.Decimal) {
	stoploss.Mark = mark

	if stoploss.Mark.Cmp(stoploss.Floor) < 0 {
		stoploss.Status = TRIGGERED
	}

	if mark.Cmp(stoploss.Peak) > 0 {
		stoploss.Peak = mark
		stoploss.Floor = mark.Mul(decimal.NewFromFloat64(0.98))
	}
}

/*
Close cancels the regulator context.
*/
func (stoploss *Stoploss) Close() (err error) {
	if stoploss.cancel != nil {
		stoploss.cancel()
	}

	return err
}
