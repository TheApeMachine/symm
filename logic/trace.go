package logic

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
decisionTrace is one diagnostic row describing how far a single measurement
progressed through the decision ladder, and, once a verdict is reached, why it
was allowed or blocked. It exists so the exact filtering point can be
reconstructed from the audit log alone rather than inferred.

Stage is the terminal stage for the measurement:

	no_category | stale_source | stale_batch | no_clamps |
	price_zero | causal_warmup | blocked | allow
*/
type decisionTrace struct {
	Tick           int64           `json:"tick"`
	Symbol         string          `json:"symbol"`
	Source         string          `json:"source"`
	Stage          string          `json:"stage"`
	Verdict        string          `json:"verdict,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	Fraction       float64         `json:"fraction"`
	Price          decimal.Decimal `json:"price"`
	Momentum       float64         `json:"momentum"`
	Flow           float64         `json:"flow"`
	Stress         float64         `json:"stress"`
	Coupling       float64         `json:"coupling"`
	PredBaseline   float64         `json:"predictiveBaseline"`
	Uplift         float64         `json:"uplift"`
	Intervention   float64         `json:"intervention"`
	Beta           float64         `json:"beta"`
	Panic          float64         `json:"panic"`
	Residual       float64         `json:"residual"`
	CausalStrength float64         `json:"causalStrength"`
	CausalBaseline float64         `json:"causalBaseline"`
	RhoMass        float64         `json:"rhoMass"`
}

/*
record writes one decision trace to the audit recorder. It is a no-op when no
recorder is configured, so instrumentation never gates on audit being enabled.
*/
func (decision *Decision) record(trace decisionTrace) {
	if decision == nil || decision.recorder == nil || !decision.traceEnabled {
		return
	}

	if err := audit.Record(decision.recorder, "decision", trace); err != nil {
		errnie.Error(err)
	}
}

/*
stageTrace builds a trace for an early terminal stage, before any physical or
causal evidence exists for the measurement.
*/
func stageTrace(
	tick int64,
	symbol string,
	source types.SourceType,
	stage string,
) decisionTrace {
	return decisionTrace{
		Tick:   tick,
		Symbol: symbol,
		Source: string(source),
		Stage:  stage,
	}
}

/*
warmupTrace builds a trace for a measurement that reached the loop but stopped
before a verdict, enriching it with whatever manifold and resonance evidence the
partial evaluation produced so a warmup stall is visible in its own terms.
*/
func warmupTrace(
	tick int64,
	symbol string,
	source types.SourceType,
	evaluation decisionEvaluation,
) decisionTrace {
	trace := stageTrace(tick, symbol, source, evaluation.stage)

	if evaluation.manifold != nil {
		trace.Price = evaluation.manifold.Price
		trace.Momentum = evaluation.manifold.Momentum
		trace.RhoMass = evaluation.manifold.Summary.Mass
	}

	if evaluation.resonance != nil {
		trace.Flow = evaluation.resonance.Flow
		trace.Stress = evaluation.resonance.Stress
		trace.Coupling = evaluation.resonance.Coupling
		trace.PredBaseline = evaluation.resonance.Baseline
	}

	return trace
}

/*
verdictTrace builds a trace for a fully evaluated measurement, capturing the
gate verdict and every quantity the gate rules compare, so a blocked decision
reveals exactly which value starved it.
*/
func verdictTrace(
	tick int64,
	symbol string,
	source types.SourceType,
	action *Action,
	evidence decisionEvidence,
) decisionTrace {
	return decisionTrace{
		Tick:           tick,
		Symbol:         symbol,
		Source:         string(source),
		Stage:          action.Verdict,
		Verdict:        action.Verdict,
		Reason:         action.Reason,
		Fraction:       action.Fraction,
		Price:          action.Price,
		Momentum:       evidence.momentum,
		Flow:           evidence.predictive.flow,
		Stress:         evidence.predictive.stress,
		Coupling:       evidence.predictive.coupling,
		PredBaseline:   evidence.predictive.baseline,
		Uplift:         evidence.counterfactual.uplift,
		Intervention:   evidence.counterfactual.intervention,
		Beta:           evidence.counterfactual.beta,
		Panic:          evidence.counterfactual.panic,
		Residual:       evidence.counterfactual.residual,
		CausalStrength: evidence.counterfactual.strength,
		CausalBaseline: evidence.counterfactual.baseline,
		RhoMass:        evidence.physical.rho.mass,
	}
}
