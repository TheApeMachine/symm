package types

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Actor for stoploss lifecycle. Non-nil when the stoploss is owned by a
position that has subscribed to the market ticker topic. When non-nil,
NewStoploss and Initialize subscribe the actor to the market ticker root
so onTicker receives live bids directly instead of relying on an external
routing layer like Desk.
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
later via Bind when the position opens. market is the upstream market actor
that publishes ticker frames on the "ticker" topic so the stoploss can
evaluate exits from live bid data without depending on a routing layer.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	mark *decimal.Decimal,
	exit func() error,
	market *Actor,
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

	if market != nil {
		stoploss.Actor.Initialize(Topic{
			Name: "ticker", Actor: market,
		})
	} else {
		stoploss.Actor.Initialize()
	}

	stoploss.Status = ARMED
	return stoploss
}

/*
Initialize the Stoploss if we are "recovering" after a restart.
market is the upstream market actor that publishes ticker frames on
the "ticker" topic so the stoploss can evaluate exits from live bid data.
*/
func (stoploss *Stoploss) Initialize(
	ctx context.Context,
	mark *decimal.Decimal,
	exit func() error,
	market *Actor,
) {
	stoploss.Status = PENDING
	stoploss.ctx = ctx
	stoploss.Entry = mark.Copy()
	stoploss.Mark = mark.Copy()
	stoploss.evaluate()

	stoploss.exit = exit

	if market != nil {
		stoploss.Actor.Initialize(Topic{Name: "ticker", Actor: market})
	} else {
		stoploss.Actor.Initialize()
	}

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

		if row.Bid == nil {
			continue
		}

		stoploss.Mark = row.Bid.Copy()

		if stoploss.Floor != nil && row.Bid.Cmp(stoploss.Floor) <= 0 {
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
evaluate checks the current the current Peek, and potentially re-sets
the Peak and the Floor if the current Mark is higher than the Peak.
*/
func (stoploss *Stoploss) evaluate() {
	if stoploss.Peak == nil {
		stoploss.Peak = stoploss.Mark.Copy()
	}

	if stoploss.Floor == nil {
		stoploss.Floor = decimal.ExactMul(
			stoploss.Peak, decimal.NewFromFloat64(0.98),
		)

		return
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
	if stoploss.cancel != nil {
		stoploss.cancel()
	}

	return err
}
