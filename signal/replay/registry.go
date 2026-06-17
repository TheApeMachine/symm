package replay

import (
	"math"
	"sync"

	"github.com/theapemachine/nomagique/statistic"
)

const (
	decaySeriesCapacity = 32
	verticalityCapacity = 64
)

type floatRing struct {
	values []float64
}

func (ring *floatRing) push(value float64) {
	if !math.IsNaN(value) && !math.IsInf(value, 0) {
		ring.values = append(ring.values, value)
	}

	if len(ring.values) > decaySeriesCapacity {
		ring.values = ring.values[len(ring.values)-decaySeriesCapacity:]
	}
}

type decayScopeState struct {
	lastPrice  float64
	bidDepths  floatRing
	askDepths  floatRing
	densities  floatRing
	spreads    floatRing
	pressures  floatRing
	imbalances floatRing
}

type verticalityScopeState struct {
	prices       []float64
	volumes      []float64
	spreads      []float64
	baselineRate float64
}

type quoteState struct {
	changePct float64
	move      float64
}

type registry struct {
	tickSizes   sync.Map
	decayStates sync.Map
	verticality sync.Map
	quotes      sync.Map
}

var marketRegistry registry

func decayState(scope string) *decayScopeState {
	raw, _ := marketRegistry.decayStates.LoadOrStore(scope, &decayScopeState{})

	return raw.(*decayScopeState)
}

func verticalityState(scope string) *verticalityScopeState {
	raw, _ := marketRegistry.verticality.LoadOrStore(scope, &verticalityScopeState{})

	return raw.(*verticalityScopeState)
}

func quote(scope string) *quoteState {
	raw, _ := marketRegistry.quotes.LoadOrStore(scope, &quoteState{})

	return raw.(*quoteState)
}

func tickSize(scope string) float64 {
	raw, ok := marketRegistry.tickSizes.Load(scope)

	if !ok {
		return 0
	}

	value, ok := raw.(float64)

	if !ok || value <= 0 {
		return 0
	}

	return value
}

func setTickSize(scope string, increment float64) {
	if scope == "" || increment <= 0 {
		return
	}

	marketRegistry.tickSizes.Store(scope, increment)
}

func appendSeries(values []float64, sample float64, capacity int) []float64 {
	if sample <= 0 || math.IsNaN(sample) || math.IsInf(sample, 0) {
		return values
	}

	values = append(values, sample)

	if len(values) > capacity {
		return values[len(values)-capacity:]
	}

	return values
}

func medianPositive(values []float64) float64 {
	positive := make([]float64, 0, len(values))

	for _, value := range values {
		if value > 0 {
			positive = append(positive, value)
		}
	}

	if len(positive) == 0 {
		return 0
	}

	return statistic.MedianOf(positive)
}
