package toxicity

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Level3 struct {
	engine *Engine
}

func NewLevel3(engine *Engine) *Level3 {
	return &Level3{
		engine: engine,
	}
}

func (level3 *Level3) Measure(row kraken.Level3Data) ([]*types.Measurement, error) {
	measurement, err := level3.engine.MeasureLevel3(row)

	if err != nil || measurement == nil {
		return nil, err
	}

	return []*types.Measurement{measurement}, nil
}
