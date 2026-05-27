package sentiment

import (
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/numeric/learned"
)

type symbolState struct {
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
	forecast  *learned.Forecast
}

func newSymbolState(pair asset.Pair) *symbolState {
	return &symbolState{
		pair:     pair,
		forecast: learned.NewForecast(0.35),
	}
}

func (state *symbolState) forecastLearner() *learned.Forecast {
	if state.forecast == nil {
		state.forecast = learned.NewForecast(0.35)
	}

	return state.forecast
}

func (state *symbolState) forecastScale() float64 {
	return state.forecastLearner().Scale()
}

func (state *symbolState) calibratedConfidence(confidence float64) float64 {
	if confidence <= 0 {
		return 0
	}

	scale := state.forecastScale()

	if scale <= 0 {
		return 0
	}

	scaled := confidence * scale

	if scaled > 1 {
		return 1
	}

	return scaled
}
