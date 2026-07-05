package causal

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Ticker struct {
	engine *Engine
}

func NewTicker(engine *Engine) *Ticker {
	return &Ticker{
		engine: engine,
	}
}

func (ticker *Ticker) Measure(row kraken.TickerData) (*logic.Measurement, error) {
	return ticker.engine.MeasureTicker(row)
}
