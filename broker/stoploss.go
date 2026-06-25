package broker

import "github.com/theapemachine/datura"

/*
Stoploss is the desk's protective exit for one open position. It lives in the
shared tree as an artifact (role "stoploss", scope symbol); a zero quantity means
the position is closed. The stop trails the mark up and never loosens.
*/
type Stoploss struct {
	Symbol string
	Side   string
	Qty    float64
	Stop   float64
	Peak   float64
	Offset float64
}

/*
NewStoploss arms a trailing stop for a freshly opened long at the given mark. The
offset is the fraction below the peak the stop rides at (0.01 == 1%).
*/
func NewStoploss(symbol string, qty, mark, offset float64) *Stoploss {
	return &Stoploss{
		Symbol: symbol,
		Side:   "sell",
		Qty:    qty,
		Stop:   mark * (1 - offset),
		Peak:   mark,
		Offset: offset,
	}
}

/*
Ratchet raises the trailing stop as the mark makes new highs and reports whether
the mark has fallen through the stop, in which case the position must exit.
*/
func (stoploss *Stoploss) Ratchet(mark float64) (breached bool) {
	if stoploss.Qty <= 0 || mark <= 0 || stoploss.Offset <= 0 {
		return false
	}

	if mark > stoploss.Peak {
		stoploss.Peak = mark

		if raised := stoploss.Peak * (1 - stoploss.Offset); raised > stoploss.Stop {
			stoploss.Stop = raised
		}
	}

	return mark <= stoploss.Stop
}

/*
Artifact serialises the stop for storage in the shared tree.
*/
func (stoploss *Stoploss) Artifact() *datura.Artifact {
	return datura.Acquire("broker", datura.APPJSON).
		WithRole("stoploss").
		WithScope(stoploss.Symbol).
		WithPayload(datura.Map[any]{
			"side":   stoploss.Side,
			"qty":    stoploss.Qty,
			"stop":   stoploss.Stop,
			"peak":   stoploss.Peak,
			"offset": stoploss.Offset,
		}.Marshal())
}

/*
StoplossFromArtifact reconstructs a stop from its tree artifact.
*/
func StoplossFromArtifact(artifact *datura.Artifact) *Stoploss {
	symbol, _ := artifact.Scope()

	return &Stoploss{
		Symbol: symbol,
		Side:   datura.Peek[string](artifact, "side"),
		Qty:    datura.Peek[float64](artifact, "qty"),
		Stop:   datura.Peek[float64](artifact, "stop"),
		Peak:   datura.Peek[float64](artifact, "peak"),
		Offset: datura.Peek[float64](artifact, "offset"),
	}
}
