package broker

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type StoplossState uint8

const (
	UNKNOWN StoplossState = iota
	HOLDING
	EXITING
)

/*
Stoploss is the desk's protective exit for one open position. A zero quantity
means the position is closed. The stop trails the mark up and never loosens.
*/
type Stoploss struct {
	Symbol string
	State  StoplossState
	order  *datura.Artifact
}

/*
NewStoploss arms a trailing stop for a freshly opened long at the given mark. The
offset is the fraction below the peak the stop rides at (0.01 == 1%).
*/
func NewStoploss(order *datura.Artifact, symbol string) *Stoploss {
	if order == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"stoploss: nil order",
			nil,
		))
		return nil
	}

	stoploss := &Stoploss{
		Symbol: symbol,
		State:  HOLDING,
		order:  order,
	}

	price := datura.Peek[float64](order, "params", "limit_price")

	stoploss.order.Poke(
		int(stoploss.State), "stoploss", "state",
	).Poke(
		[]float64{price}, "stoploss", "marks",
	).Poke(
		price, "stoploss", "peak",
	).Poke(
		price*(1-price+(price/10)), "stoploss", "stop",
	).Poke(
		price+(price/10), "stoploss", "offset",
	)

	return stoploss
}

/*
Ratchet raises the trailing stop as the mark makes new highs and reports whether
the mark has fallen through the stop, in which case the position must exit.
*/
func (stoploss *Stoploss) Ratchet(mark float64) *Stoploss {
	state := datura.Peek[map[string]any](stoploss.order, "stoploss")
	peak := state["peak"].(float64)
	stop := state["stop"].(float64)
	offset := state["offset"].(float64)

	if stop <= 0 && offset > 0 {
		stop = peak * (1 - offset)
	}

	if mark > peak {
		peak = mark

		if raised := peak * (1 - offset); raised > stop {
			stop = raised
		}
	}

	state["marks"] = append(state["marks"].([]float64), mark)
	stoploss.order.Poke(state, "stoploss")

	return stoploss
}

func (stoploss *Stoploss) Close() {
	if stoploss.order != nil {
		stoploss.order = nil
	}

	stoploss.State = UNKNOWN
}
