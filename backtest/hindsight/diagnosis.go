package hindsight

import (
	"fmt"
	"sort"
	"strings"
)

const (
	completeExecutionCoverage = 1.0

	admissionAcceptedKey            = "admission:accepted"
	admissionDirectionMarginKey     = "admission:direction_margin"
	admissionDirectionTargetKey     = "admission:direction_target"
	admissionThesisMarginKey        = "admission:thesis_score_margin"
	admissionConfidenceMarginKey    = "admission:confidence_margin"
	admissionSupportMarginKey       = "admission:support_margin"
	admissionContradictionMarginKey = "admission:contradiction_margin"
	executionCoverageKey            = "execution:visible_coverage"
	executionFrictionKey            = "execution:friction_fraction"
	executionSpreadKey              = "execution:spread_fraction"
	executionImpactKey              = "execution:impact_fraction"
)

const (
	DiagnosisAdmission     = "admission_policy"
	DiagnosisDirection     = "directional_evidence"
	DiagnosisMeasurement   = "measurement_coverage"
	DiagnosisPredictive    = "predictive_readiness"
	DiagnosisAllocation    = "allocation_capacity"
	DiagnosisExecution     = "execution_feasibility"
	DiagnosisFollowThrough = "decision_follow_through"
	DiagnosisRegulator     = "regulator_readiness"
	DiagnosisObservability = "observability_gap"
)

const (
	RecommendationTuneParameter      = "tune_parameter"
	RecommendationImproveMeasurement = "improve_measurement"
	RecommendationCollectOutcomes    = "collect_outcomes"
	RecommendationFixAllocation      = "fix_allocation"
	RecommendationFixExecution       = "fix_execution"
	RecommendationInstrumentFunnel   = "instrument_funnel"
	RecommendationValidateRegulator  = "validate_regulator"
	RecommendationImproveAudit       = "improve_observability"
)

/*
Blocker is one recorded fact that kept a missed leg outside the executable
path. Observed, target, and gap preserve the counterfactual arithmetic while
explanation states what the number means in the system that produced it.
*/
type Blocker struct {
	Key         string  `json:"key"`
	Category    string  `json:"category"`
	Label       string  `json:"label"`
	Source      string  `json:"source,omitempty"`
	Observed    float64 `json:"observed"`
	Target      float64 `json:"target,omitempty"`
	HasTarget   bool    `json:"hasTarget"`
	Gap         float64 `json:"gap"`
	Severity    float64 `json:"severity"`
	Explanation string  `json:"explanation"`
}

/*
Recommendation is the next falsifiable experiment suggested by one or more
missed legs. Suggested values are counterfactual sweep points, never direct
production settings, and Confidence measures evidence completeness rather than
promising that the recovered trade would have remained profitable live.
*/
type Recommendation struct {
	Key          string   `json:"key"`
	Kind         string   `json:"kind"`
	Target       string   `json:"target"`
	Title        string   `json:"title"`
	Action       string   `json:"action"`
	Rationale    string   `json:"rationale"`
	Current      float64  `json:"current,omitempty"`
	Suggested    float64  `json:"suggested,omitempty"`
	HasCurrent   bool     `json:"hasCurrent"`
	HasSuggested bool     `json:"hasSuggested"`
	Adjustment   string   `json:"adjustment,omitempty"`
	Confidence   float64  `json:"confidence"`
	ImpactPct    float64  `json:"impactPct"`
	Occurrences  int      `json:"occurrences"`
	Symbols      []string `json:"symbols,omitempty"`
}

/*
Diagnosis retains the complete causal answer for one missed leg: the primary
failure class, every supporting blocker, the quality of the retained evidence,
and the concrete experiment that can disprove or improve that explanation.
*/
type Diagnosis struct {
	Category        string         `json:"category"`
	Summary         string         `json:"summary"`
	EvidenceQuality float64        `json:"evidenceQuality"`
	EvidenceStatus  string         `json:"evidenceStatus"`
	Blockers        []Blocker      `json:"blockers"`
	Recommendation  Recommendation `json:"recommendation"`
}

/* RootCauseSummary aggregates missed value without blending symbol evidence. */
type RootCauseSummary struct {
	Category    string   `json:"category"`
	ImpactPct   float64  `json:"impactPct"`
	Occurrences int      `json:"occurrences"`
	Symbols     []string `json:"symbols"`
}

type blockerCandidate struct {
	Blocker
	priority int
}

type recommendationAggregate struct {
	recommendation Recommendation
	weightedTrust  float64
	weight         float64
	symbols        map[string]struct{}
	current        float64
	currentSet     bool
	currentStable  bool
}

func diagnoseOpportunity(
	context SignalContext,
	leg Leg,
	recorded bool,
) Diagnosis {
	quality, status := evidenceQuality(context, recorded)

	if !recorded {
		blocker := Blocker{
			Key:      "audit:decision_missing",
			Category: DiagnosisObservability,
			Label:    "decision evidence missing",
			Gap:      leg.ProfitPct,
			Severity: 1,
			Explanation: "No decision for this symbol was retained before or during " +
				"the leg, so parameter or measurement attribution would be invented.",
		}
		recommendation := recommendationFor(blocker, leg, quality)

		return Diagnosis{
			Category:        blocker.Category,
			Summary:         missedClause(leg) + blocker.Explanation,
			EvidenceQuality: quality,
			EvidenceStatus:  status,
			Blockers:        []Blocker{blocker},
			Recommendation:  recommendation,
		}
	}

	candidates := blockerCandidates(context, leg)

	if len(candidates) == 0 {
		candidates = append(candidates, blockerCandidate{
			Blocker: Blocker{
				Key:      "funnel:unresolved",
				Category: DiagnosisFollowThrough,
				Label:    "decision path unresolved",
				Gap:      leg.ProfitPct,
				Severity: 1,
				Explanation: firstNonEmpty(
					context.Reason,
					context.Cause,
					"the retained decision does not identify the stage that stopped entry",
				),
			},
			priority: 7,
		})
	}

	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].priority != candidates[right].priority {
			return candidates[left].priority < candidates[right].priority
		}

		return candidates[left].Key < candidates[right].Key
	})

	blockers := make([]Blocker, 0, len(candidates))

	for _, candidate := range candidates {
		blockers = append(blockers, candidate.Blocker)
	}

	primary := blockers[0]
	recommendation := recommendationFor(primary, leg, quality)

	return Diagnosis{
		Category:        primary.Category,
		Summary:         missedClause(leg) + primary.Explanation,
		EvidenceQuality: quality,
		EvidenceStatus:  status,
		Blockers:        blockers,
		Recommendation:  recommendation,
	}
}

func evidenceQuality(context SignalContext, recorded bool) (float64, string) {
	if !recorded {
		return 0, "missing"
	}

	checks := []bool{
		context.Action != "",
		context.Reason != "" || context.Cause != "",
		context.Alternatives != nil,
		hasAdmissionEvidence(context.Alternatives),
		hasMeasurementEvidence(context.Alternatives),
		context.PredictiveReady || context.PredictiveStatus != "",
		context.GraphScore != 0 || context.AdmissionThreshold != 0 ||
			strings.Contains(strings.ToLower(context.Reason), "graph"),
	}
	present := 0

	for _, available := range checks {
		if available {
			present++
		}
	}

	quality := float64(present) / float64(len(checks))

	if present == len(checks) {
		return quality, "complete"
	}

	return quality, "partial"
}

func hasAdmissionEvidence(alternatives map[string]float64) bool {
	for _, key := range []string{
		admissionAcceptedKey,
		admissionDirectionMarginKey,
		admissionThesisMarginKey,
		admissionConfidenceMarginKey,
		admissionSupportMarginKey,
		admissionContradictionMarginKey,
	} {
		if _, exists := alternatives[key]; exists {
			return true
		}
	}

	return false
}

func hasMeasurementEvidence(alternatives map[string]float64) bool {
	for key := range alternatives {
		if strings.HasPrefix(key, "meas:") {
			return true
		}
	}

	return false
}

func measurementSource(name string) string {
	parts := strings.Split(name, ":")

	if len(parts) < 4 {
		return name
	}

	return parts[len(parts)-2]
}

func missedClause(leg Leg) string {
	return fmt.Sprintf(
		"missed %s→%s (%0.2f%%): ",
		fmtPrice(leg.BuyPrice),
		fmtPrice(leg.SellPrice),
		leg.ProfitPct*100,
	)
}

func contextRecorded(context SignalContext) bool {
	return context.Action != "" || context.Reason != "" || context.Cause != "" ||
		context.Opportunity || context.Type != "" || context.Alternatives != nil ||
		context.GraphScore != 0 || context.ThesisScore != 0 ||
		context.ThesisConfidence != 0 || context.ThesisSupport != 0 ||
		context.ThesisContradiction != 0 || context.Direction != 0 ||
		context.Confidence != 0 || context.AdmissionThreshold != 0 ||
		context.PredictiveReady || context.PredictiveStatus != ""
}

/* diagnose preserves the original summary-only API for existing callers. */
func diagnose(context SignalContext, leg Leg) string {
	return diagnoseOpportunity(context, leg, contextRecorded(context)).Summary
}
