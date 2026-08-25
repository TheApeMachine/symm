package algo

import (
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Pearl is a composite Primitive assembled from the shared causal and probability
atomic units. It evaluates Pearl's ladder of causation over a retained row
window held in the generic sample slots:

	observed association (causal.Association)
	-> kernel backdoor-adjusted effect (causal.BackdoorEffect)
	-> dispersion scales (causal.EffectScales)
	-> intervention percentile (causal.Percentile)
	-> do-expectation (causal.DoExpectationFrame)
	-> abductive counterfactual (causal.CounterfactualFrame)

The four ladder channels the caller consumes are left on the frame for the
downstream classification composition to lift; Pearl is an orchestrator of
primitives, not the source of the math. It reads no external state and carries
no judgment: every stage names a measurement.

The frame contract the caller (a keyed Number) satisfies:

	SymbolRowCount  -> number of retained rows
	sample/0..      -> the row window, row-major
	SymbolTarget    -> the target column index
	SymbolTreatment -> the treatment column index
	SymbolLevel     -> the intervention level (or percentile fraction)
	SymbolBandwidth -> the kernel bandwidth
*/
func Pearl() types.Primitive {
	return types.Pipe(
		causal.Association,
		causal.BackdoorEffect,
		causal.EffectScales,
		causal.Percentile,
		causal.DoExpectationFrame,
		causal.CounterfactualFrame,
		probability.Softmax(),
		probability.Argmax(),
		probability.EvidenceShare(),
	)
}
