package types

import (
	"context"

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
	*Actor
	ctx    context.Context
	cancel context.CancelFunc
	Symbol string
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
	ctx context.Context, symbol string, mark *decimal.Decimal, exit func() error,
) *Stoploss {
	errnie.Info("creating stoploss")

	ctx, cancel := context.WithCancel(ctx)
	initial := mark.Copy()

	stoploss := &Stoploss{
		ctx:    ctx,
		cancel: cancel,
		Symbol: symbol,
		Peak:   initial.Copy(),
		Mark:   initial,
		Floor:  initial.Mul(decimal.NewFromFloat64(0.98)),
		exit:   exit,
	}

	stoploss.Actor = NewActor(ctx, "stoploss", map[string]Handler{
		"ticker": {Topic: "stoploss", Fn: stoploss.onTicker},
	})

	stoploss.Actor.Initialize()
	return stoploss
}

/*
onTicker advances the independent stop state from live ticker bids and invokes
the bound exit callback immediately when the floor is breached.
*/
func (stoploss *Stoploss) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data

	for _, row := range rows {
		if row.Symbol == stoploss.Symbol && row.Bid != nil {
			if row.Bid.Cmp(stoploss.Floor) <= 0 {
				if stoploss.exit == nil {
					return errnie.Error(errnie.Err(
						errnie.ExpectationFailed,
						"stoploss: exit called but not set",
						nil,
					))
				}

				if err := stoploss.exit(); err != nil {
					return errnie.Error(errnie.Err(
						errnie.ExpectationFailed,
						"stoploss: exit failed",
						err,
					))
				}
			}

			if row.Bid.Cmp(stoploss.Peak) > 0 {
				stoploss.Peak = row.Bid.Copy()
				stoploss.Floor = stoploss.Peak.Mul(decimal.NewFromFloat64(0.98))
			}

			stoploss.Mark = row.Bid.Copy()
		}
	}

	return stoploss
}

/*
Close cancels the regulator context and delegates exit to the same callback so
manual close and floor breach share one order path.
*/
func (stoploss *Stoploss) Close() {
	stoploss.exit()

	if stoploss.cancel != nil {
		stoploss.cancel()
	}
}
