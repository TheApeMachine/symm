package types

import (
	"fmt"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
This file is the legacy semantic reasoning frame used by the UI-only
ReasoningFrame / ReasoningTopology views. It is NOT part of the live causal
decision path: live reasoning consumes actual Measurement coordinates through
the Influence Graph and CausalSchema. The fixed columns here exist only to
keep the legacy UI projection compiling.
*/

const (
	ReasoningActionExit  float64 = -1
	ReasoningActionWait  float64 = 0
	ReasoningActionEnter float64 = 1
	ReasoningActionScale float64 = 2
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
	SymbolContextConfidence = nmtypes.MustIntern("context_confidence")
	SymbolTreatment         = nmtypes.MustIntern("treatment")
	SymbolTarget            = nmtypes.MustIntern("target")
	SymbolFlow              = nmtypes.MustIntern("flow")
	SymbolLiquidityImpact   = nmtypes.MustIntern("liquidity_impact")
	SymbolHawkes            = nmtypes.MustIntern("hawkes")
	SymbolCoherence         = nmtypes.MustIntern("coherence")
	SymbolRegime            = nmtypes.MustIntern("regime")
	SymbolSurprise          = nmtypes.MustIntern("surprise")
	SymbolPosition          = nmtypes.MustIntern("position")
	SymbolHorizon           = nmtypes.MustIntern("horizon")
	SymbolMaximumHorizon    = nmtypes.MustIntern("max_horizon")
	SymbolArchetype         = nmtypes.MustIntern("archetype")
	SymbolVelocity          = nmtypes.MustIntern("velocity")
	SymbolStoredEnergy      = nmtypes.MustIntern("stored_energy")
	SymbolCausalExpectation = nmtypes.MustIntern("causal_expectation")
	SymbolSpread            = nmtypes.MustIntern("spread")
)

var reasoningSymbols = [...]nmtypes.Symbol{
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

/*
ValidateReasoningFrame ensures the legacy semantic state is complete. It is a
UI-legacy helper; the live search never requires this frame.
*/
func ValidateReasoningFrame(frame nmtypes.Frame) error {
	for symbolIndex, symbol := range reasoningSymbols {
		if _, found := frame.Get(symbol); found {
			continue
		}

		return fmt.Errorf(
			"reasoning: legacy frame is missing %s",
			reasoningSymbolNames[symbolIndex],
		)
	}

	maximumHorizon, _ := frame.Get(SymbolMaximumHorizon)

	if maximumHorizon < 1 {
		return fmt.Errorf("reasoning: max_horizon must be positive")
	}

	horizon, _ := frame.Get(SymbolHorizon)

	if horizon < 0 || horizon > maximumHorizon {
		return fmt.Errorf(
			"reasoning: horizon %.0f is outside [0, %.0f]",
			horizon, maximumHorizon,
		)
	}

	return nil
}

/*
FrameToRow converts the legacy named state into the fixed legacy column
contract. UI-legacy only.
*/
func FrameToRow(frame nmtypes.Frame) ([]float64, error) {
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
RowToFrame converts the legacy row contract back into named state.
UI-legacy only.
*/
func RowToFrame(row []float64) (nmtypes.Frame, error) {
	if len(row) != ReasoningColumnCount {
		return nmtypes.Frame{}, fmt.Errorf(
			"reasoning: legacy row has width %d; expected %d",
			len(row), ReasoningColumnCount,
		)
	}

	frame := nmtypes.Frame{}

	for column, symbol := range reasoningSymbols {
		frame.Put(symbol, row[column])
	}

	return frame, ValidateReasoningFrame(frame)
}

/*
FrameValues returns a stable named projection for UI inspection.
UI-legacy only.
*/
func FrameValues(frame nmtypes.Frame) map[string]float64 {
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
ActionName returns the name of a legacy float64 intervention value.
UI-legacy only.
*/
func ActionName(action float64) string {
	switch action {
	case ReasoningActionExit:
		return "exit"
	case ReasoningActionWait:
		return "wait"
	case ReasoningActionEnter:
		return "enter"
	case ReasoningActionScale:
		return "scale"
	default:
		return fmt.Sprintf("unknown:%g", action)
	}
}
