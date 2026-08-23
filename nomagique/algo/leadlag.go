package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolLeadLagCorrelation = nomagique.MustIntern("leadlag/correlation")
	SymbolInefficiency       = nomagique.MustIntern("leadlag/inefficiency")
	SymbolSync               = nomagique.MustIntern("leadlag/sync")
	SymbolDecoupled          = nomagique.MustIntern("leadlag/decoupled")
	SymbolStall              = nomagique.MustIntern("leadlag/stall")
	SymbolLeadLagStrength    = nomagique.MustIntern("leadlag/strength")
	SymbolLeadLagSeparation  = nomagique.MustIntern("leadlag/hypothesis_separation")
	SymbolLagDirection       = nomagique.MustIntern("leadlag/direction")
)

/*
LeadLag composes the asynchronous cross-lag equation with normalized evidence
projection. It owns no path or cohort state; the caller supplies committed
anchor and follower Path Frames.
*/
func LeadLag() nomagique.Primitive {
	return nomagique.Pipe(
		equation.CrossLag(),
		leadLagScores,
	)
}

func leadLagScores(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	ready, _ := input.Get(equation.SymbolLeadLagReady)

	if ready == 0 {
		return state, input, nil
	}

	lagReady, _ := input.Get(equation.SymbolLagReady)
	lagCorrelation := input.MustGet(equation.SymbolLagCorrelation)
	contemporaneous := input.MustGet(equation.SymbolContempCorrelation)
	lagFraction := input.MustGet(equation.SymbolLagFraction)
	lagThreshold := input.MustGet(equation.SymbolSignificance)
	contemporaneousThreshold := input.MustGet(equation.SymbolContempSignificance)
	inefficiency := 0.0

	if lagReady != 0 {
		inefficiency = math.Abs(lagCorrelation) * lagFraction
	}

	syncScore := 0.0

	if math.Abs(contemporaneous) > contemporaneousThreshold {
		syncScore = math.Abs(contemporaneous) * (1 - lagFraction)
	}

	relation := math.Max(math.Abs(contemporaneous), math.Abs(lagCorrelation)*lagReady)
	decoupled := math.Max(0, lagThreshold-relation) / lagThreshold
	stall := 0.0
	strength := math.Max(math.Max(inefficiency, syncScore), decoupled)
	selectedCorrelation := contemporaneous

	if lagReady != 0 && math.Abs(lagCorrelation) > math.Abs(contemporaneous) {
		selectedCorrelation = lagCorrelation
	}

	output := input
	output.Put(SymbolLeadLagCorrelation, math.Abs(selectedCorrelation))
	output.Put(SymbolSignedCorrelation, selectedCorrelation)
	output.Put(SymbolInefficiency, inefficiency)
	output.Put(SymbolSync, syncScore)
	output.Put(SymbolDecoupled, decoupled)
	output.Put(SymbolStall, stall)
	output.Put(SymbolLeadLagStrength, strength)
	output.Put(
		SymbolLeadLagSeparation,
		hypothesisSeparation(inefficiency, syncScore, decoupled, stall),
	)
	output.Put(SymbolLagDirection, signed(input.MustGet(equation.SymbolLagBars))*lagReady)

	return state, output, nil
}

func signed(value float64) float64 {
	if value < 0 {
		return -1
	}

	if value > 0 {
		return 1
	}

	return 0
}
