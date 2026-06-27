package broker

import (
	"fmt"
	"sync/atomic"

	"github.com/theapemachine/datura"
)

/*
Stoploss is the desk's protective exit for one open position. It lives in the
shared tree as an artifact (role "stoploss", scope symbol); a zero quantity means
the position is closed. The stop trails the mark up and never loosens.
*/
type Stoploss struct {
	Symbol string
	Side   string
	state  atomic.Pointer[StoplossState]
}

type StoplossState struct {
	Qty    float64
	Stop   float64
	Peak   float64
	Offset float64
}

type StoplossExit struct {
	Symbol string
	Side   string
	Qty    float64
}

/*
NewStoploss arms a trailing stop for a freshly opened long at the given mark. The
offset is the fraction below the peak the stop rides at (0.01 == 1%).
*/
func NewStoploss(symbol string, qty, mark, offset float64) *Stoploss {
	stoploss := &Stoploss{
		Symbol: symbol,
		Side:   "sell",
	}
	stoploss.state.Store(&StoplossState{
		Qty:    qty,
		Stop:   mark * (1 - offset),
		Peak:   mark,
		Offset: offset,
	})

	return stoploss
}

func (stoploss *Stoploss) Snapshot() StoplossState {
	if stoploss == nil {
		return StoplossState{}
	}
	state := stoploss.state.Load()
	if state == nil {
		return StoplossState{}
	}
	return *state
}

/*
Cover updates an already armed stop to cover additional confirmed fill quantity
without ever loosening the trailing stop.
*/
func (stoploss *Stoploss) Cover(qty, mark float64) error {
	if stoploss == nil {
		return fmt.Errorf("stoploss: nil stoploss")
	}
	if stoploss.Symbol == "" || stoploss.Side == "" {
		return fmt.Errorf("stoploss: missing symbol or side")
	}
	if qty <= 0 {
		return fmt.Errorf("stoploss: non-positive quantity for %s", stoploss.Symbol)
	}
	if mark <= 0 {
		return fmt.Errorf("stoploss: non-positive mark for %s", stoploss.Symbol)
	}

	for {
		current := stoploss.state.Load()
		if current == nil {
			return fmt.Errorf("stoploss: uninitialized state for %s", stoploss.Symbol)
		}
		if current.Offset <= 0 {
			return fmt.Errorf("stoploss: non-positive offset for %s", stoploss.Symbol)
		}
		if current.Qty > 0 && qty < current.Qty {
			return fmt.Errorf("stoploss: fill quantity regressed for %s", stoploss.Symbol)
		}

		next := &StoplossState{
			Qty:    qty,
			Stop:   current.Stop,
			Peak:   current.Peak,
			Offset: current.Offset,
		}

		if next.Peak <= 0 {
			next.Peak = mark
		}
		if next.Stop <= 0 {
			next.Stop = next.Peak * (1 - next.Offset)
		}
		if mark > next.Peak {
			next.Peak = mark

			if raised := next.Peak * (1 - next.Offset); raised > next.Stop {
				next.Stop = raised
			}
		}

		if stoploss.state.CompareAndSwap(current, next) {
			return nil
		}
	}
}

/*
Ratchet raises the trailing stop as the mark makes new highs and reports whether
the mark has fallen through the stop, in which case the position must exit.
*/
func (stoploss *Stoploss) Ratchet(mark float64) (StoplossExit, bool, error) {
	if stoploss == nil {
		return StoplossExit{}, false, fmt.Errorf("stoploss: nil stoploss")
	}
	if stoploss.Symbol == "" || stoploss.Side == "" {
		return StoplossExit{}, false, fmt.Errorf("stoploss: missing symbol or side")
	}
	if mark <= 0 {
		return StoplossExit{}, false, fmt.Errorf("stoploss: non-positive mark for %s", stoploss.Symbol)
	}

	for {
		current := stoploss.state.Load()
		if current == nil || current.Qty <= 0 {
			return StoplossExit{}, false, nil
		}
		if current.Offset <= 0 {
			return StoplossExit{}, false, fmt.Errorf("stoploss: non-positive offset for %s", stoploss.Symbol)
		}

		next := &StoplossState{
			Qty:    current.Qty,
			Stop:   current.Stop,
			Peak:   current.Peak,
			Offset: current.Offset,
		}

		if mark > next.Peak {
			next.Peak = mark

			if raised := next.Peak * (1 - next.Offset); raised > next.Stop {
				next.Stop = raised
			}
		}

		if mark > next.Stop {
			if next.Peak == current.Peak && next.Stop == current.Stop {
				return StoplossExit{}, false, nil
			}
			if stoploss.state.CompareAndSwap(current, next) {
				return StoplossExit{}, false, nil
			}
			continue
		}

		next.Qty = 0
		if stoploss.state.CompareAndSwap(current, next) {
			return StoplossExit{
				Symbol: stoploss.Symbol,
				Side:   stoploss.Side,
				Qty:    current.Qty,
			}, true, nil
		}
	}
}

func (stoploss *Stoploss) Close() error {
	if stoploss == nil {
		return fmt.Errorf("stoploss: nil stoploss")
	}
	if stoploss.Symbol == "" || stoploss.Side == "" {
		return fmt.Errorf("stoploss: missing symbol or side")
	}

	for {
		current := stoploss.state.Load()
		if current == nil || current.Qty <= 0 {
			return nil
		}

		next := &StoplossState{
			Qty:    0,
			Stop:   current.Stop,
			Peak:   current.Peak,
			Offset: current.Offset,
		}

		if stoploss.state.CompareAndSwap(current, next) {
			return nil
		}
	}
}

/*
Artifact serialises the stop for storage in the shared tree.
*/
func (stoploss *Stoploss) Artifact() *datura.Artifact {
	if stoploss == nil {
		return nil
	}

	state := stoploss.state.Load()
	if state == nil {
		return nil
	}

	return datura.Acquire("broker", datura.APPJSON).
		WithRole("stoploss").
		WithScope(stoploss.Symbol).
		WithPayload(datura.Map[any]{
			"side":   stoploss.Side,
			"qty":    state.Qty,
			"stop":   state.Stop,
			"peak":   state.Peak,
			"offset": state.Offset,
		}.Marshal())
}

/*
StoplossFromArtifact reconstructs a stop from its tree artifact.
*/
func StoplossFromArtifact(artifact *datura.Artifact) *Stoploss {
	if artifact == nil {
		return nil
	}

	symbol, err := artifact.Scope()
	if err != nil || symbol == "" {
		return nil
	}

	stoploss := &Stoploss{
		Symbol: symbol,
		Side:   datura.Peek[string](artifact, "side"),
	}
	if stoploss.Side == "" {
		return nil
	}

	stoploss.state.Store(&StoplossState{
		Qty:    datura.Peek[float64](artifact, "qty"),
		Stop:   datura.Peek[float64](artifact, "stop"),
		Peak:   datura.Peek[float64](artifact, "peak"),
		Offset: datura.Peek[float64](artifact, "offset"),
	})

	return stoploss
}
