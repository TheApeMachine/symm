package depthflow

import (
	"context"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
type Signal[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	book   *Book
	trade  *Trade
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)
	sample := flow.NewSample()
	bookflow := equation.NewBookflow()
	classifier := probability.NewScoreClassifier(
		[]string{"loadedScore", "spoofScore", "thinScore", "neutralScore"},
		[]float64{
			float64(types.CategoryIndex(types.CategoryLoadedImbalance)),
			float64(types.CategoryIndex(types.CategorySpoofTrap)),
			float64(types.CategoryIndex(types.CategoryBookThinning)),
			float64(types.CategoryIndex(types.CategoryDenseNeutrality)),
		},
	)

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
		book:   NewBook(sample, bookflow, classifier),
		trade:  NewTrade(sample, bookflow, classifier),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"book", "trade"}
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
