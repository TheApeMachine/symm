package fluid

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	registry *Registry
}

func NewTrade(registry *Registry) *Trade {
	return &Trade{registry: registry}
}

func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
	if row.Timestamp.IsZero() {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: trade event timestamp required",
			nil,
		))
	}

	state := trade.registry.loadSymbol(row.Symbol)

	if state == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		)
	}

	if row.Price.Float64() <= 0 || row.Qty <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"fluid: trade price and qty required",
			nil,
		)
	}

	if err := state.FeedTrade(
		row.Timestamp.UTC(), row.Price.Float64(), row.Qty, row.Side,
	); errnie.Error(err) != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)
	}

	return nil, nil
}
