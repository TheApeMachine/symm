package toxicity

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Level3 struct {
	engine *Engine
}

func NewLevel3(engine *Engine) *Level3 {
	return &Level3{
		engine: engine,
	}
}

func (level3 *Level3) Measure(row kraken.Level3Data) (*logic.Measurement, error) {
	return level3.engine.MeasureLevel3(row)
}
