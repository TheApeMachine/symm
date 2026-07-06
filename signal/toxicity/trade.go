package toxicity

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	engine *Engine
}

func NewTrade(engine *Engine) *Trade {
	return &Trade{
		engine: engine,
	}
}

func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
	measurement, err := trade.engine.MeasureTrade(row)

	if err != nil || measurement == nil {
		return nil, err
	}

	return []*types.Measurement{measurement}, nil
}
