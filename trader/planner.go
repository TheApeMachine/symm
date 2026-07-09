package trader

import (
	"math"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
)

type Planner struct {
	desk  *broker.Desk
	price *broker.Price

	baseFraction float64
	entryErrors  map[string]*adaptive.TimeElastic
	exitErrors   map[string]*adaptive.TimeElastic
	velocity     map[string]*adaptive.TimeElastic
}

func NewPlanner(desk *broker.Desk, price *broker.Price) *Planner {
	return &Planner{
		desk:         desk,
		price:        price,
		baseFraction: viper.GetViper().GetFloat64("trading.sizing.base_fraction"),
		entryErrors:  map[string]*adaptive.TimeElastic{},
		exitErrors:   map[string]*adaptive.TimeElastic{},
		velocity:     map[string]*adaptive.TimeElastic{},
	}
}

func (planner *Planner) Update(thesis *strategy.Thesis) ([]strategy.Intent, error) {
	if planner.desk == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader planner: broker desk required",
			nil,
		))
	}

	if planner.price == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader planner: broker price required",
			nil,
		))
	}

	if math.IsNaN(planner.baseFraction) ||
		math.IsInf(planner.baseFraction, 0) ||
		planner.baseFraction <= 0 ||
		planner.baseFraction > 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader planner: trading.sizing.base_fraction must be within the quote balance",
			nil,
		))
	}

	holdings := planner.desk.Holdings()
	thesis.AddEvidence("holdings", holdings)

	entry, ok := planner.Entry(thesis)
	if !ok {
		return nil, nil
	}

	if _, held := holdings[entry.Symbol]; held {
		return nil, nil
	}

	thesis.AddEvidence("entry", entry)

	entryPrice, ok := planner.price.Entry(entry.Symbol)
	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"trader planner: entry price missing for "+entry.Symbol,
			nil,
		))
	}

	intents := make([]strategy.Intent, 0, 2)
	exit, rotate := planner.Exit(thesis)

	if rotate {
		if err := planner.desk.Sell(exit.Symbol); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"trader planner: failed to submit exit for "+exit.Symbol,
				err,
			))
		}

		thesis.AddEvidence("exit", exit)
		intents = append(intents, exit)
	}

	if err := planner.desk.Buy(
		entry.Symbol,
		planner.baseFraction,
		entryPrice,
		rotate,
	); err != nil {
		return intents, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader planner: failed to submit entry for "+entry.Symbol,
			err,
		))
	}

	entry.Size = *decimal.NewFromFloat64(planner.baseFraction)
	thesis.AddEvidence("entry", entry)
	intents = append(intents, entry)

	return intents, nil
}

func (planner *Planner) Entry(thesis *strategy.Thesis) (strategy.Intent, bool) {
	if planner.price == nil {
		return strategy.Intent{}, false
	}

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

	if math.IsNaN(edge) || math.IsInf(edge, 0) ||
		math.IsNaN(confidence) || math.IsInf(confidence, 0) ||
		confidence <= causal.EntryBaseline {
		return strategy.Intent{}, false
	}

	frictionRat, ok := planner.price.RoundTripFriction(symbol)

	if !ok {
		return strategy.Intent{}, false
	}

	friction, _ := frictionRat.Float64()
	utility := confidence*math.Expm1(edge) - friction

	if math.IsNaN(utility) || math.IsInf(utility, 0) {
		return strategy.Intent{}, false
	}

	intent := strategy.Intent{
		Symbol:     symbol,
		Action:     strategy.ActionBuy,
		Size:       *decimal.NewFromFloat64(planner.baseFraction),
		Edge:       utility,
		Velocity:   utility,
		Confidence: confidence,
		Thesis:     thesis,
	}

	if utility <= 0 {
		return intent, false
	}

	return intent, true
}

func (planner *Planner) Exit(thesis *strategy.Thesis) (strategy.Intent, bool) {
	replacement, ok := Evidence[strategy.Intent](thesis, "entry")

	if !ok || replacement.Action != strategy.ActionBuy {
		return strategy.Intent{}, false
	}

	holdings, ok := Evidence[map[string]broker.PositionData](thesis, "holdings")

	if !ok {
		return strategy.Intent{}, false
	}

	var position broker.PositionData
	rotationEdge := 0.0
	rotationFound := false

	for _, holding := range holdings {
		if strings.EqualFold(holding.Symbol, replacement.Symbol) {
			continue
		}

		holdUtility := holding.ReturnPct
		edge := replacement.Velocity - holdUtility

		if math.IsNaN(edge) || math.IsInf(edge, 0) {
			return strategy.Intent{}, false
		}

		if !rotationFound || edge > rotationEdge {
			position = holding
			rotationEdge = edge
			rotationFound = true
		}
	}

	if !rotationFound || rotationEdge <= 0 {
		return strategy.Intent{}, false
	}

	return strategy.Intent{
		Symbol:     position.Symbol,
		Action:     strategy.ActionSell,
		Edge:       rotationEdge,
		Velocity:   rotationEdge,
		Confidence: replacement.Confidence,
		Thesis:     thesis,
	}, true
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
