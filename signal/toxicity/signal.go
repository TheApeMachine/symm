package toxicity

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Toxicity is the Quality perspective, analyzing the honesty of the book by
tracking per-order liquidity behavior near touch.

Cancel-to-fill asymmetry is an L3-plus-tape story: level3 add/delete/modify
events provide order identity and price level, while the trade tape provides
near-price execution evidence. Public data does not expose an exact order-to-
trade match, so deletes are fill-classified only when the trade tape supports
the price; otherwise they are cancel-classified. L2 book quantity deltas are not
used as a fallback for cancel/fill labels.

1. Liquidity Vacuum

One side is retreating and creating a void.
Indicators: High cancel/fill asymmetry with one side retracting.
Semantic Meaning: Vacuum surcharge - the void itself drives price.

2. Toxic Bluff

Near-touch blocks disappear rather than fill.
Indicators: High cancel/fill ratio at near-touch levels.
Semantic Meaning: Manipulated/fake - a bluff wall about to crumble.

3. Hard Support

Liquidity fills rather than cancels on approach.
Indicators: Low cancel/fill ratio, high fill rate, no side retracting.
Semantic Meaning: Robust/sincere - the wall will hold on contact.

# Summary of Toxicity Categories

| Category         | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:-----------------|:------------------|:----------------|:-----------------------|
| Liquidity Vacuum | High Asymmetry    | One Side        | Vacuum Surcharge       |
| Toxic Bluff      | High              | Near-Touch      | Manipulated / Fake     |
| Hard Support     | Low (High Fill)   | None            | Robust / Sincere       |
*/
type Signal[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	level3 *Level3
	trade  *Trade
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)
	engine := NewEngine()

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
		level3: NewLevel3(engine),
		trade:  NewTrade(engine),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"level3", "trade"}
}

func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.Level3Data:
		return signal.level3.Measure(row)
	case kraken.TradeData:
		return signal.trade.Measure(row)
	}

	return nil, nil
}

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
