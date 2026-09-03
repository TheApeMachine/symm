package equation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Standardizer calculates online causal z-scores without heap allocations.
It embeds WelfordEngine by value, guaranteeing O(1) time and 0 allocs/op.
*/
type Standardizer struct {
	Engine *adaptive.WelfordEngine

	welford adaptive.WelfordEngine
	lastZ   types.Number
}

func (standardizer *Standardizer) Step(number types.Number) types.Number {
	engine := &standardizer.welford

	if standardizer.Engine != nil {
		engine = standardizer.Engine
	}

	mean, stdDev := engine.Update(float64(number))

	if stdDev <= 0 || math.IsNaN(stdDev) {
		standardizer.lastZ = 0

		return 0
	}

	standardizer.lastZ = types.Number((float64(number) - mean) / stdDev)

	return standardizer.lastZ
}

func (standardizer *Standardizer) Mean() float64 {
	if standardizer.Engine != nil {
		return standardizer.Engine.Mean()
	}

	return standardizer.welford.Mean()
}

func (standardizer *Standardizer) Dispersion() float64 {
	if standardizer.Engine != nil {
		return standardizer.Engine.Dispersion()
	}

	return standardizer.welford.Dispersion()
}

func (standardizer *Standardizer) Variance() float64 {
	if standardizer.Engine != nil {
		return standardizer.Engine.Variance()
	}

	return standardizer.welford.Variance()
}

func (standardizer *Standardizer) Count() float64 {
	if standardizer.Engine != nil {
		return standardizer.Engine.Count()
	}

	return standardizer.welford.Count()
}
