package audit

import (
	"fmt"
	"math"
	"sort"
)

/*
Verdicts, in the vocabulary of AUDIT_CONTRACT.md. Only VALID and SUPPORTED
count as passed; a stage that cannot fail by construction says so instead of
reporting a pass.
*/
const (
	// VerdictValid: a hard invariant was checked and held.
	VerdictValid = "VALID"
	// VerdictBreach: a hard invariant was checked and violated.
	VerdictBreach = "CONTRACT_BREACH"
	// VerdictSupported: a hypothesis separated from its empirical null.
	VerdictSupported = "SUPPORTED"
	// VerdictNotSupported: enough evidence, no separation from the null.
	VerdictNotSupported = "NOT_SUPPORTED"
	// VerdictMeasured: descriptive observations; no claim either way.
	VerdictMeasured = "MEASURED"
	// VerdictInsufficient: not enough evidence to evaluate.
	VerdictInsufficient = "INSUFFICIENT_DATA"
	// VerdictNotATest: the check compares a deterministic computation with
	// itself or restates its own construction, so it cannot fail.
	VerdictNotATest = "NOT_A_TEST"
	// VerdictInvalid: required provenance could not be established.
	VerdictInvalid = "INVALID_EXPERIMENT"
)

/*
passed reports whether a verdict is a real pass.
*/
func passed(verdict string) bool {
	return verdict == VerdictValid || verdict == VerdictSupported
}

/*
StageVerdict is one row of the report's verdict table.
*/
type StageVerdict struct {
	Stage     string `json:"stage"`
	Verdict   string `json:"verdict"`
	Criterion string `json:"criterion"`
	Evidence  string `json:"evidence"`
}

/*
Thresholds are the audit's explicit, named decision parameters. Every verdict
that is not derived from an empirical null uses one of these, and the report
records the values it ran with. None of them is tuned to make a stage pass.
*/
type Thresholds struct {
	// Significance is the false-positive rate every null comparison is held
	// to: a hypothesis is SUPPORTED when its add-one empirical p-value
	// (1 + #null >= real) / (1 + permutations) is at most this. It is also
	// the family-wise level of the Stage 0 z-magnitude bound.
	Significance float64 `json:"significance"`
	// SpikeLatency is the venue-to-local drift, in milliseconds, above which
	// one observation counts as a latency spike.
	SpikeLatencyMs float64 `json:"spike_latency_ms"`
	// SpikeFraction is the largest fraction of spikes Stage 0.5 accepts.
	SpikeFraction float64 `json:"spike_fraction"`
	// DominanceJSD is the region-distribution shift, in bits, removing one
	// family may cause before that family counts as dominating the grid.
	DominanceJSD float64 `json:"dominance_jsd"`
	// DuplicationJSD is the shift, in bits, duplicating one family may cause
	// before the grid counts as not duplication resistant.
	DuplicationJSD float64 `json:"duplication_jsd"`
}

/*
DefaultThresholds are the values the audit ran with before they were named;
Significance is the conventional 5%.
*/
func DefaultThresholds() Thresholds {
	return Thresholds{
		Significance:   0.05,
		SpikeLatencyMs: 200,
		SpikeFraction:  0.05,
		DominanceJSD:   0.40,
		DuplicationJSD: 0.45,
	}
}

/*
upperPValue is the add-one empirical p-value of real against a null where
larger values are more extreme. It is never zero: with N permutations the
smallest attainable p is 1/(N+1).
*/
func upperPValue(real float64, null []float64) float64 {
	extreme := 0

	for _, value := range null {
		if value >= real {
			extreme++
		}
	}

	return float64(1+extreme) / float64(1+len(null))
}

/*
lowerPValue is upperPValue for a statistic where smaller is more extreme.
*/
func lowerPValue(real float64, null []float64) float64 {
	extreme := 0

	for _, value := range null {
		if value <= real {
			extreme++
		}
	}

	return float64(1+extreme) / float64(1+len(null))
}

/*
hypothesisVerdict turns a p-value into SUPPORTED or NOT_SUPPORTED. A null too
small to reach the significance level at all is INSUFFICIENT_DATA rather than
a failure the experiment could never have avoided.
*/
func hypothesisVerdict(pValue float64, permutations int, significance float64) string {
	if 1/float64(1+permutations) > significance {
		return VerdictInsufficient
	}

	if pValue <= significance {
		return VerdictSupported
	}

	return VerdictNotSupported
}

/*
zBound is the largest |z| a stream of n standardized observations may show
at family-wise level significance under any finite-variance distribution:
Chebyshev bounds P(|z| >= k) by 1/k^2, so the union over n observations stays
below significance at k = sqrt(n / significance). A causal z-score uses an
estimated scale, so this is a bound on a well-behaved stream, not a test of
normality; a z beyond it is not a tail event but a broken scale.
*/
func zBound(n int, significance float64) float64 {
	return math.Sqrt(float64(max(n, 1)) / significance)
}

/*
quantileOf returns the q-quantile of values without modifying them.
*/
func quantileOf(values []float64, q float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return empiricalQuantile(sorted, q)
}

/*
contractVerdict is Stage 0's verdict: hard domains and z magnitudes either
held on every observation or did not.
*/
func contractVerdict(contract Stage0Contract) string {
	if contract.TotalMetricsChecked == 0 {
		return VerdictInsufficient
	}

	if contract.Passed {
		return VerdictValid
	}

	return VerdictBreach
}

/*
BuildVerdicts collects every stage's verdict with the criterion it was held
to, in report order.
*/
func BuildVerdicts(report *AuditReport) []StageVerdict {
	t := report.Thresholds
	verdicts := []StageVerdict{
		{"0. Contracts", contractVerdict(report.Contract),
			fmt.Sprintf("declared hard domains on every observation; |z| <= sqrt(n/%.2f) per stream", t.Significance),
			fmt.Sprintf("%d/%d series breach a domain (%d observations); %d z beyond bound",
				report.Contract.BreachingMetricsCount, report.Contract.TotalMetricsChecked,
				report.Contract.TotalBreaches, report.Contract.MetricNorm.ZMagnitudeBreaches)},
		{"0.5 Timing", report.Timing.Status,
			fmt.Sprintf("no At regression per (source, symbol); spikes > %.0fms on at most %.0f%%", t.SpikeLatencyMs, t.SpikeFraction*100),
			fmt.Sprintf("%d inversions, %d spikes of %d", report.Timing.SequenceInversions, report.Timing.LatencySpikes, report.Timing.TotalChecked)},
		{"1. Vitality", report.Vitality.Status, "descriptive",
			fmt.Sprintf("%d raw series, %d canonical cells, %d constant", report.Vitality.RawProducerMetrics,
				report.Vitality.CanonicalGridCells, report.Vitality.CanonicalDeadCells)},
		{"2. Sympathy", report.Sympathy.Status,
			fmt.Sprintf("within-symbol |r| exceedance vs block-shuffled null, p <= %.2f", t.Significance),
			fmt.Sprintf("%d pairs, %.1f%% above null p95, p=%.3f", report.Sympathy.TotalPairs, report.Sympathy.SeparationRatio*100, report.Sympathy.PValue)},
		{"3. Grid stationarity", report.GridStability.Status,
			fmt.Sprintf("JSD between halves vs tick-shuffled null; stationary when p > %.2f (ARI is NOT_A_TEST)", t.Significance),
			fmt.Sprintf("JSD %.3f bits, p=%.3f", report.GridStability.DistributionJSD, report.GridStability.StationarityPValue)},
		{"4. Token dynamics", report.TokenDynamics.Status,
			fmt.Sprintf("transition entropy below block null, p <= %.2f", t.Significance),
			fmt.Sprintf("%d emissions, H=%.3f, p=%.3f", report.TokenDynamics.TotalEmissions, report.TokenDynamics.TransitionEntropy, report.TokenDynamics.PValue)},
		{"5a. Ignition precursors", report.Precursor.IgnitionHypothesis.Status,
			fmt.Sprintf("event vs control excursions, excursion-label null, p <= %.2f", t.Significance),
			fmt.Sprintf("JSD %.3f bits, p=%.3f", report.Precursor.IgnitionHypothesis.DivergenceBits, report.Precursor.IgnitionHypothesis.PValue)},
		{"5b. Exhaustion precursors", report.Precursor.ExhaustionHypothesis.Status,
			fmt.Sprintf("late vs early half within excursions, paired swap null, p <= %.2f", t.Significance),
			fmt.Sprintf("JSD %.3f bits, p=%.3f", report.Precursor.ExhaustionHypothesis.DivergenceBits, report.Precursor.ExhaustionHypothesis.PValue)},
		{"5c. Held-out precursor skill", report.Precursor.PredictiveSkill.Status,
			fmt.Sprintf("chronological 60/40 split, held-out MCC vs label null, p <= %.2f", t.Significance),
			fmt.Sprintf("%d held-out, MCC %.3f, p=%.3f", report.Precursor.PredictiveSkill.EvaluatedSamples, report.Precursor.PredictiveSkill.MCC, report.Precursor.PredictiveSkill.PValue)},
		{"5d. Friction clearance", report.Precursor.EconomicRelevance.Status,
			"detections are defined as moves that clear friction", "restates the detector's own criterion"},
		{"6. Simulated trie", report.CognitiveTrie.Status,
			fmt.Sprintf("in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= %.2f", t.Significance),
			fmt.Sprintf("balanced accuracy %.1f%%", report.CognitiveTrie.Skill.BalancedAccuracy*100)},
	}

	if report.Equivalence != nil {
		verdicts = append(verdicts, StageVerdict{"V1. Equivalence", report.Equivalence.Status,
			"one code path compared with itself", fmt.Sprintf("deterministic=%t", report.Equivalence.Deterministic)})
	}

	if report.Truthfulness != nil {
		verdicts = append(verdicts, StageVerdict{"V2. Truthfulness", report.Truthfulness.Status,
			"cvd frames recomputed from the stored trade tape; every check exercised",
			fmt.Sprintf("%d violations; unexercised %v", report.Truthfulness.ViolationsCount, report.Truthfulness.UnexercisedChecks)})
	}

	if report.Causality != nil {
		verdicts = append(verdicts, StageVerdict{"V3. Causality", report.Causality.Status,
			"replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs",
			fmt.Sprintf("%d compared; changed: future %d, cross-symbol %d, epoch %d", report.Causality.ComparedObservations,
				report.Causality.ContaminatedCount, report.Causality.CrossSymbolContaminated, report.Causality.EpochContaminated)})
	}

	if report.Sensitivity != nil {
		verdicts = append(verdicts, StageVerdict{"V4. Sensitivity", report.Sensitivity.Status,
			fmt.Sprintf("no family moves regions > %.2f bits on removal or > %.2f on duplication (named limits)", t.DominanceJSD, t.DuplicationJSD),
			fmt.Sprintf("%d dominant; max duplication %.3f bits", report.Sensitivity.DominantFamilies, report.Sensitivity.MaxDuplicationJSD)})
	}

	return verdicts
}

/*
OverallState is the worst verdict class present. Descriptive stages and
non-tests do not raise or lower it.
*/
func OverallState(verdicts []StageVerdict) string {
	state := "ALL TESTS PASSED"
	rank := 0

	for _, row := range verdicts {
		switch {
		case (row.Verdict == VerdictBreach || row.Verdict == VerdictInvalid) && rank < 3:
			state, rank = "CONTRACT BREACHES PRESENT", 3
		case row.Verdict == VerdictNotSupported && rank < 2:
			state, rank = "HYPOTHESES NOT SUPPORTED", 2
		case row.Verdict == VerdictInsufficient && rank < 1:
			state, rank = "INCOMPLETE EVIDENCE", 1
		}
	}

	return state
}
