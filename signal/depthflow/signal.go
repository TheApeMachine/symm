package depthflow

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
DepthFlow is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Multi-level distance weighting is
owned by nomagique's bookflow primitive.

Spoof Trap is currently scored from L2 book shape contradicted by touch pressure.
A faithful spoof read from add/delete order behavior still needs L3 per-order
events; this implementation does not pretend L2 can prove cancel/fill intent.

1. Loaded Imbalance - book weight agrees with trade pressure.
2. Spoof Trap - deep-book shape contradicts touch pressure.
3. Book Thinning - defensive depth disappears relative to the weighted book.
4. Dense Neutrality - balanced thick depth with low pressure.

# Summary of DepthFlow Categories

| Category         | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"        |
|:-----------------|:-------------------------|:------------------|:---------------------|
| Loaded Imbalance | High                     | High (Agrees)     | Structural Gravity   |
| Spoof Trap       | High                     | Low (Contradicts) | Manipulated/Fake     |
| Book Thinning    | Rapidly Falling          | Variable          | Exhaustion/Crumbling |
| Dense Neutrality | Balanced                 | Low               | Robust Stability     |
*/

/*
Signal routes book and trade rows into the shared depth-flow pipeline.
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
