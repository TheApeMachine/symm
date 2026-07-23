package fluid

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Trade feeds executed flow into shared symbol state so later book measurements
include tape-driven liquidity removal.
*/
type Trade struct {
	registry *Registry
}

func NewTrade(registry *Registry) *Trade {
	return &Trade{registry: registry}
}

/*
Measure validates and applies one trade row. It emits no measurement because
book updates own publication of the combined fluid state.
*/
func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
	if row.Timestamp.IsZero() {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: trade event timestamp required",
			nil,
		))
	}

	state, err := trade.registry.loadSymbol(row.Symbol)

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"fluid: trade symbol state required",
			err,
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
	); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)
	}

	return nil, nil
}
