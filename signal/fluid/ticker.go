package fluid

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type Ticker struct {
	registry *Registry
}

func NewTicker(registry *Registry) *Ticker {
	return &Ticker{registry: registry}
}

func (ticker *Ticker) Measure(row kraken.TickerData) error {
	if row.Timestamp.IsZero() {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: ticker event timestamp required",
			nil,
		))
	}

	state := ticker.registry.loadSymbol(row.Symbol)

	if state == nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		)
	}

	if err := state.FeedTicker(row, row.Timestamp.UTC()); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)
	}

	return nil
}
