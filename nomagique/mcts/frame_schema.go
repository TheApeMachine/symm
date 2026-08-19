package mcts

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

const (
	ActionExit  float64 = -1
	ActionWait  float64 = 0
	ActionEnter float64 = 1
	ActionScale float64 = 2
)

const (
	ColumnContextConfidence = iota
	ColumnTreatment
	ColumnTarget
	ColumnFlow
	ColumnLiquidityImpact
	ColumnHawkes
	ColumnCoherence
	ColumnRegime
	ColumnSurprise
	ColumnPosition
	ColumnHorizon
	ColumnMaximumHorizon
	ColumnArchetype
	ColumnVelocity
	ColumnStoredEnergy
	ColumnCausalExpectation
	ColumnSpread
	ReasoningColumnCount
)

var (
	SymbolContextConfidence = nomagique.MustIntern("context_confidence")
	SymbolTreatment         = nomagique.MustIntern("treatment")
	SymbolTarget            = nomagique.MustIntern("target")
	SymbolFlow              = nomagique.MustIntern("flow")
	SymbolLiquidityImpact   = nomagique.MustIntern("liquidity_impact")
	SymbolHawkes            = nomagique.MustIntern("hawkes")
	SymbolCoherence         = nomagique.MustIntern("coherence")
	SymbolRegime            = nomagique.MustIntern("regime")
	SymbolSurprise          = nomagique.MustIntern("surprise")
	SymbolPosition          = nomagique.MustIntern("position")
	SymbolHorizon           = nomagique.MustIntern("horizon")
	SymbolMaximumHorizon    = nomagique.MustIntern("max_horizon")
	SymbolArchetype         = nomagique.MustIntern("archetype")
	SymbolVelocity          = nomagique.MustIntern("velocity")
	SymbolStoredEnergy      = nomagique.MustIntern("stored_energy")
	SymbolCausalExpectation = nomagique.MustIntern("causal_expectation")
	SymbolSpread            = nomagique.MustIntern("spread")
)

var reasoningSymbols = [...]nomagique.Symbol{
	SymbolContextConfidence,
	SymbolTreatment,
	SymbolTarget,
	SymbolFlow,
	SymbolLiquidityImpact,
	SymbolHawkes,
	SymbolCoherence,
	SymbolRegime,
	SymbolSurprise,
	SymbolPosition,
	SymbolHorizon,
	SymbolMaximumHorizon,
	SymbolArchetype,
	SymbolVelocity,
	SymbolStoredEnergy,
	SymbolCausalExpectation,
	SymbolSpread,
}

var reasoningSymbolNames = [...]string{
	"context_confidence",
	"treatment",
	"target",
	"flow",
	"liquidity_impact",
	"hawkes",
	"coherence",
	"regime",
	"surprise",
	"position",
	"horizon",
	"max_horizon",
	"archetype",
	"velocity",
	"stored_energy",
	"causal_expectation",
	"spread",
}

var DefaultControlColumns = []int{
	ColumnContextConfidence,
	ColumnHawkes,
	ColumnCoherence,
	ColumnRegime,
	ColumnSurprise,
	ColumnLiquidityImpact,
}

var DefaultFeatureColumns = []int{
	ColumnContextConfidence,
	ColumnTreatment,
	ColumnFlow,
	ColumnLiquidityImpact,
	ColumnHawkes,
	ColumnCoherence,
	ColumnRegime,
	ColumnSurprise,
	ColumnPosition,
	ColumnHorizon,
}

/*
ValidateReasoningFrame ensures the semantic state is complete before search.
*/
func ValidateReasoningFrame(frame nomagique.Frame) error {
	for symbolIndex, symbol := range reasoningSymbols {
		if _, found := frame.Get(symbol); found {
			continue
		}

		return fmt.Errorf(
			"mcts: reasoning frame is missing %s",
			reasoningSymbolNames[symbolIndex],
		)
	}

	maximumHorizon, _ := frame.Get(SymbolMaximumHorizon)

	if maximumHorizon < 1 {
		return fmt.Errorf("mcts: max_horizon must be positive")
	}

	horizon, _ := frame.Get(SymbolHorizon)

	if horizon < 0 || horizon > maximumHorizon {
		return fmt.Errorf(
			"mcts: horizon %.0f is outside [0, %.0f]",
			horizon, maximumHorizon,
		)
	}

	return nil
}

/*
FrameToRow converts named state into the fixed SCM column contract.
*/
func FrameToRow(frame nomagique.Frame) ([]float64, error) {
	if err := ValidateReasoningFrame(frame); err != nil {
		return nil, err
	}

	row := make([]float64, ReasoningColumnCount)

	for column, symbol := range reasoningSymbols {
		row[column], _ = frame.Get(symbol)
	}

	return row, nil
}

/*
RowToFrame converts the stable SCM row contract back into named state.
*/
func RowToFrame(row []float64) (nomagique.Frame, error) {
	if len(row) != ReasoningColumnCount {
		return nomagique.Frame{}, fmt.Errorf(
			"mcts: reasoning row has width %d; expected %d",
			len(row), ReasoningColumnCount,
		)
	}

	frame := nomagique.Frame{}

	for column, symbol := range reasoningSymbols {
		frame.Put(symbol, row[column])
	}

	return frame, ValidateReasoningFrame(frame)
}

/*
FrameValues returns a stable named projection for UI inspection.
*/
func FrameValues(frame nomagique.Frame) map[string]float64 {
	values := make(map[string]float64, ReasoningColumnCount)

	for symbolIndex, symbol := range reasoningSymbols {
		value, found := frame.Get(symbol)

		if !found {
			continue
		}

		values[reasoningSymbolNames[symbolIndex]] = value
	}

	return values
}

/*
ActionName returns the strategic intervention represented by an action value.
*/
func ActionName(action float64) string {
	switch action {
	case ActionExit:
		return "exit"
	case ActionWait:
		return "wait"
	case ActionEnter:
		return "enter"
	case ActionScale:
		return "scale"
	default:
		return fmt.Sprintf("unknown:%g", action)
	}
}
