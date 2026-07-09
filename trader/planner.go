package trader

import (
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
)

type Planner struct {
	price *broker.Price

	entryErrors map[string]*adaptive.TimeElastic
	exitErrors  map[string]*adaptive.TimeElastic
	velocity    map[string]*adaptive.TimeElastic
}

func NewPlanner(price *broker.Price) *Planner {
	return &Planner{
		price:       price,
		entryErrors: map[string]*adaptive.TimeElastic{},
		exitErrors:  map[string]*adaptive.TimeElastic{},
		velocity:    map[string]*adaptive.TimeElastic{},
	}
}

func (planner *Planner) Entry(thesis *strategy.Thesis) (strategy.Intent, bool) {
	resonance, ok := Evidence[logic.ResonanceOutcome](thesis, "resonance")

	if !ok {
		return strategy.Intent{}, false
	}

	causal, ok := Evidence[algorithm.PearlOutput](thesis, "causal")

	if !ok {
		return strategy.Intent{}, false
	}

	// This requires Thesis to expose Symbol, or symbol must be stored as evidence.
	symbol, ok := Evidence[string](thesis, "symbol")

	if !ok {
		return strategy.Intent{}, false
	}

	edge := resonance.ReturnForecast
	confidence := causal.Confidence
	velocity := edge * confidence

	return strategy.Intent{
		Symbol:     symbol,
		Action:     strategy.ActionBuy,
		Edge:       edge,
		Velocity:   velocity,
		Confidence: confidence,
		Thesis:     thesis,
	}, edge > 0
}

func (planner *Planner) Exit(
	position broker.PositionData,
	replacement strategy.Intent,
) (strategy.Intent, bool) {
	currentVelocity := position.ReturnPct
	rotationEdge := replacement.Velocity - currentVelocity

	return strategy.Intent{
		Symbol:   position.Symbol,
		Action:   strategy.ActionSell,
		Edge:     rotationEdge,
		Velocity: rotationEdge,
		Thesis:   replacement.Thesis,
	}, rotationEdge > 0
}

func Evidence[T any](thesis *strategy.Thesis, key string) (T, bool) {
	var zero T

	snapshot, ok := thesis.Evidence(key)

	if !ok {
		return zero, false
	}

	value, ok := snapshot.(T)

	if !ok {
		return zero, false
	}

	return value, true
}
