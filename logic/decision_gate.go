package logic

import "github.com/theapemachine/symm/types"

type decisionGate struct {
	rules []decisionRule
}

type decisionRule struct {
	reason   string
	source   types.SourceType
	category func(decisionEvidence) types.CategoryType
	blocked  func(decisionEvidence) bool
}

type decisionResult struct {
	verdict    string
	reason     string
	actionType string
	side       string
	source     types.SourceType
	category   types.CategoryType
}

func newDecisionGate() *decisionGate {
	return &decisionGate{rules: []decisionRule{
		{
			reason:   "physical_field_empty",
			source:   types.SourceManifold,
			category: physicalCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.physical.rho.mass <= 0 },
		},
		{
			reason:   "decision_direction_absent",
			source:   types.SourceManifold,
			category: physicalCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.momentum == 0 },
		},
		{
			reason:   "predictive_stress_dominates",
			source:   types.SourceResonance,
			category: predictiveCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.predictive.stress >= evidence.predictive.flow },
		},
		{
			reason:   "predictive_coupling_dominates",
			source:   types.SourceResonance,
			category: predictiveCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.predictive.coupling >= evidence.predictive.flow },
		},
		{
			reason:   "predictive_state_below_entry",
			source:   types.SourceResonance,
			category: predictiveCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.predictive.flow < evidence.predictive.baseline },
		},
		{
			// The supervised task head predicts the adaptive-horizon forward
			// return directly. Block entries it expects to be non-positive, so a
			// learned directional read can veto a move the unsupervised flow
			// categories would otherwise admit. Zero (head still warming up, or
			// no signal) does not block — the other rules still govern.
			reason:   "predictive_forecast_negative",
			source:   types.SourceResonance,
			category: predictiveCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.predictive.forecast < 0 },
		},
		{
			reason:   "causal_counterfactual_absent",
			source:   types.SourceCausal,
			category: causalCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.counterfactual.uplift == 0 },
		},
		{
			reason:   "causal_beta_dominates",
			source:   types.SourceCausal,
			category: causalCategory,
			blocked: func(evidence decisionEvidence) bool {
				return evidence.counterfactual.beta >= evidence.counterfactual.strength
			},
		},
		{
			reason:   "causal_liquidity_dominates",
			source:   types.SourceCausal,
			category: causalCategory,
			blocked: func(evidence decisionEvidence) bool {
				return evidence.counterfactual.panic >= evidence.counterfactual.strength
			},
		},
		{
			reason:   "causal_residual_dominates",
			source:   types.SourceCausal,
			category: causalCategory,
			blocked: func(evidence decisionEvidence) bool {
				return evidence.counterfactual.residual >= evidence.counterfactual.strength
			},
		},
		{
			reason:   "causal_intervention_absent",
			source:   types.SourceCausal,
			category: causalCategory,
			blocked:  func(evidence decisionEvidence) bool { return evidence.counterfactual.intervention <= 0 },
		},
		{
			reason:   "causal_state_below_entry",
			source:   types.SourceCausal,
			category: causalCategory,
			blocked: func(evidence decisionEvidence) bool {
				return evidence.counterfactual.strength < evidence.counterfactual.baseline
			},
		},
	}}
}

func (gate *decisionGate) Decide(evidence decisionEvidence) decisionResult {
	intent := newDecisionIntent(evidence)
	for _, rule := range gate.rules {
		if rule.blocked(evidence) {
			return decisionResult{
				verdict:    "blocked",
				reason:     rule.reason,
				actionType: intent.actionType,
				side:       intent.side,
				source:     rule.source,
				category:   rule.category(evidence),
			}
		}
	}

	return decisionResult{
		verdict:    "allow",
		reason:     "physical_predictive_causal_match",
		actionType: intent.actionType,
		side:       intent.side,
		source:     types.SourceCausal,
		category:   evidence.counterfactual.category,
	}
}

func physicalCategory(evidence decisionEvidence) types.CategoryType {
	return evidence.physical.category
}

func predictiveCategory(evidence decisionEvidence) types.CategoryType {
	return evidence.predictive.category
}

func causalCategory(evidence decisionEvidence) types.CategoryType {
	return evidence.counterfactual.category
}
