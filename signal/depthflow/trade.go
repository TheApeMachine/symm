package depthflow

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Trade struct {
	engine *Engine
}

func NewTrade(engine *Engine) *Trade {
	return &Trade{
		engine: engine,
	}
}

func (trade *Trade) Measure(row kraken.TradeData) (*logic.Measurement, error) {
	return trade.engine.MeasureTrade(row)
}
