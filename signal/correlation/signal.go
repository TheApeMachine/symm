package correlation

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it.
*/
type Signal[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	ticker *Ticker
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal[T]) Categories() []types.CategoryType {
	return []types.CategoryType{
		types.SystemicHerd,
		types.DecoupledAlpha,
		types.StochasticNoise,
		types.DivergentStress,
	}
}

func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.TickerData:
		return signal.ticker.Measure(row, crossSection)
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
