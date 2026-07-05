package fluid

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	registry *Registry
}

func NewTicker(registry *Registry) *Ticker {
	return &Ticker{registry: registry}
}

func (ticker *Ticker) Measure(row kraken.TickerData) ([]*types.Measurement, error) {
	if row.Timestamp.IsZero() {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: ticker event timestamp required",
			nil,
		))
	}

	state := ticker.registry.loadSymbol(row.Symbol)

	if state == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		)
	}

	if err := state.FeedTicker(row, row.Timestamp.UTC()); errnie.Error(err) != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)
	}

	return nil, nil
}
