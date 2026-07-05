package exhaust

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Exhaust is the "Exit Thesis" perspective, tracking microstructure decay to
advise on the urgency of closing an open position.

1. What it measures exactly (in isolation)

The Exhaust signal tracks microstructure decay to advise on the urgency of
closing an open position. Unlike entry signals that look for momentum ignition,
Exhaust looks for momentum rot.

Book Thinning: Measures the trend of bid/ask depth; if depth is disappearing
as price moves, the move is "hollow".

Pressure Fade: Tracks the decay in trade pressure; it signals when the
aggressive "hitters" have run out of ammunition.

Spread Widening: Monitors the bid/ask spread; widening spreads during a trend
indicate increasing mechanical resistance and risk.

Imbalance Flip: Detects when the "weight" of the book flips from the support
side to the resistance side.

---

2. Semantically, what story does it tell?

The Exhaust signal tells the story of when to leave: momentum rot and thesis
decay.

1. Mechanical Collapse - book thinning dominates.
2. Thermal Exhaustion - trade pressure fade dominates.
3. Fragile Expansion - spread widening dominates.
4. Active Reversal - book imbalance flip dominates.

# Summary of Exhaust Categories

| Category            | Primary Metric  | Urgency  | Market "Feel"                    |
|:--------------------|:----------------|:---------|:---------------------------------|
| Mechanical Collapse | Book Thinning   | High     | Crumbling Walls / Flash-Risk     |
| Thermal Exhaustion  | Pressure Fade   | Moderate | Dying Momentum / Topping Out     |
| Fragile Expansion   | Spread Widen    | Moderate | Increasing Friction / Risky Hold |
| Active Reversal     | Imbalance Flip  | High     | Sentiment Flip / Counter-Attack  |

Current implementation consumes book and trade artifacts and uses nomagique's
decay primitive. L3 can improve per-order delete/fill attribution, but this
signal does not claim order-event truth from L2.
*/

/*
Signal routes book and trade rows into the shared exhaust decay pipeline.
*/
type Signal[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	book   *Book
	trade  *Trade
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)
	engine := NewEngine()

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
		book:   NewBook(engine),
		trade:  NewTrade(engine),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"book", "trade"}
}

func (signal *Signal[T]) Categories() []types.CategoryType {
	return []types.CategoryType{
		types.MechanicalCollapse,
		types.ThermalExhaustion,
		types.FragileExpansion,
		types.ActiveReversal,
	}
}

func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.BookData:
		return signal.book.Measure(row)
	case kraken.TradeData:
		return signal.trade.Measure(row)
	}

	return nil, nil
}

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() error {
	signal.cancel()
	return nil
}
