package types

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Stoploss is an always-on position regulator. It tracks entry, marks the current
price against a trailing stop floor that ratchets up with profits, and emits
exit decisions when the floor is breached or a peak with adverse forward path
is detected. There is no armed/unbound state - it's active from initialization.
*/
type Stoploss struct {
	*Actor `json:"-"`
	ctx    context.Context
	cancel context.CancelFunc
	Status Status           `json:"status"`
	Symbol string           `json:"symbol"`
	Entry  *decimal.Decimal `json:"entry"`
	Peak   *decimal.Decimal `json:"peak"`
	Mark   *decimal.Decimal `json:"mark"`
	Floor  *decimal.Decimal `json:"floor"`
	exit   func() error
}

/*
NewStoploss constructs an active regulator with default weight. Entry is bound
later via Bind when the position opens.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	mark *decimal.Decimal,
	exit func() error,
) *Stoploss {
	errnie.Info("creating stoploss")

	ctx, cancel := context.WithCancel(ctx)

	stoploss := &Stoploss{
		ctx:    ctx,
		cancel: cancel,
		Symbol: symbol,
		Entry:  mark.Copy(),
		Mark:   mark.Copy(),
		exit:   exit,
		Status: PENDING,
	}

	stoploss.evaluate()

	stoploss.Actor = NewActor(ctx, "stoploss", map[string]Handler{
		"ticker": {Topic: "stoploss", Fn: stoploss.onTicker},
	})

	stoploss.Actor.Initialize()
	stoploss.Status = ARMED
	return stoploss
}

/*
Initialize the Stoploss if we are "recovering" after a restart.
*/
func (stoploss *Stoploss) Initialize(
	ctx context.Context,
	mark *decimal.Decimal,
	exit func() error,
) {
	stoploss.Status = PENDING
	stoploss.ctx = ctx
	stoploss.Entry = mark.Copy()
	stoploss.Mark = mark.Copy()
	stoploss.evaluate()

	stoploss.exit = exit
	stoploss.Actor.Initialize()
	stoploss.Status = ARMED
}

/*
onTicker advances the independent stop state from live ticker bids and invokes
the bound exit callback immediately when the floor is breached.
*/
func (stoploss *Stoploss) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data

	for _, row := range rows {
		if row.Symbol != stoploss.Symbol {
			continue
		}

		stoploss.Mark = row.Bid.Copy()

		if row.Bid.Cmp(stoploss.Floor) <= 0 {
			if stoploss.exit == nil {
				stoploss.Status = ERROR

				return errnie.Error(errnie.Err(
					errnie.ExpectationFailed,
					"stoploss: exit called but not set",
					nil,
				))
			}

			if err := stoploss.exit(); err != nil {
				stoploss.Status = ERROR

				return errnie.Error(errnie.Err(
					errnie.ExpectationFailed,
					"stoploss: exit failed",
					err,
				))
			}
		}

		stoploss.evaluate()
	}

	return stoploss
}

/*
Update the Stoploss with a new mark price. This is used to update the mark
from the Position on fills, and to update the mark from the Holding on recovery.
*/
func (stoploss *Stoploss) Update(mark *decimal.Decimal) {
	stoploss.Mark = mark.Copy()
	stoploss.evaluate()
}

/*
MarshalJSON emits the flat stop frame consumed by the terminal. Stoploss owns
the trailing floor, peak, and armed state, so Position can publish this directly
without Holding inventing stop fields.
*/
func (stoploss *Stoploss) MarshalJSON() ([]byte, error) {
	frame := map[string]any{
		"symbol":     stoploss.Symbol,
		"stop_price": stoploss.Floor,
		"peak_price": stoploss.Peak,
		"armed":      stoploss.Status == ARMED,
	}

	if stoploss.Entry != nil && stoploss.Entry.Sign() > 0 &&
		stoploss.Floor != nil && stoploss.Peak != nil {
		frame["stop_return"] = stoploss.Floor.Copy().Sub(stoploss.Entry).Div(stoploss.Entry)
		frame["peak_return"] = stoploss.Peak.Copy().Sub(stoploss.Entry).Div(stoploss.Entry)
	}

	return sonic.Marshal(frame)
}

/*
evaluate checks the current the current Peek, and potentially re-sets
the Peak and the Floor if the current Mark is higher than the Peak.
*/
func (stoploss *Stoploss) evaluate() {
	if stoploss.Peak == nil {
		stoploss.Peak = stoploss.Mark.Copy()
	}

	if stoploss.Mark.Cmp(stoploss.Peak) > 0 {
		stoploss.Peak = stoploss.Mark.Copy()

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
	if stoploss.exit != nil {
		err = stoploss.exit()
	}

	if stoploss.cancel != nil {
		stoploss.cancel()
	}

	return err
}
