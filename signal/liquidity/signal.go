package liquidity

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in thin markets
by ranking a symbol's volume against the broader types.
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
		types.ExtremeScarcity,
		types.MedianDepth,
		types.RobustLiquidity,
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

func (signal *Signal[T]) Close() error {
	signal.cancel()
	return signal.err
}
