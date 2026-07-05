package exhaust

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	book   *Book
	trade  *Trade
}

func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	engine := NewEngine()

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		book:   NewBook(engine),
		trade:  NewTrade(engine),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade"}
}

func (signal *Signal) Measure(
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	measurements := make([]*logic.Measurement, 0)

	if input.Role == "book" {
		for _, row := range input.Book {
			measurement, err := signal.book.Measure(row)
			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent, err.Error(), err,
				))
			}

			if measurement == nil {
				continue
			}

			measurements = append(measurements, measurement)
		}

		return measurements, nil
	}

	if input.Role == "trade" {
		for _, row := range input.Trade {
			measurement, err := signal.trade.Measure(row)
			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent, err.Error(), err,
				))
			}

			if measurement == nil {
				continue
			}

			measurements = append(measurements, measurement)
		}

		return measurements, nil
	}

	return nil, nil
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
