package audit

import (
	"fmt"
	"strings"
)

/*
GenerateSummaryMarkdown renders observations and experiment status without turning
descriptive statistics into arbitrary health verdicts.
*/
func GenerateSummaryMarkdown(report *AuditReport) string {
	var sb strings.Builder

	overall := "AUDIT MEASURED"
	if report.Vitality.Status == "INSUFFICIENT_DATA" ||
		report.Sympathy.Status == "INSUFFICIENT_DATA" ||
		report.GridStability.Status == "INSUFFICIENT_DATA" ||
		report.TokenDynamics.Status == "INSUFFICIENT_DATA" ||
		report.Precursor.IgnitionHypothesis.Status == "INSUFFICIENT_DATA" {
		overall = "INCOMPLETE EVIDENCE"
	}

	if !report.Contract.Passed {
		overall = "CONTRACT_BREACHES PRESENT"
	}

	sb.WriteString("# SYMM Pipeline Empirical Audit\n\n")
	sb.WriteString(fmt.Sprintf(
		"**State:** %s | **Epoch:** `%d` | **Symbol:** `%s` | **Ticks:** `%d` | **Generated:** `%s`\n\n",
		overall, report.Epoch, report.Symbol, report.TotalTicks, report.Timestamp,
	))
	sb.WriteString("This report follows [the empirical audit contract](../hindsight/audit/AUDIT_CONTRACT.md): hard mathematical contracts may fail; descriptive stages report measurements; missing evidence is explicit.\n\n")

	sb.WriteString("| Stage | Question | Experiment state | Observation |\n")
	sb.WriteString("| :--- | :--- | :---: | :--- |\n")

	contractStatus := "VALID"
	if !report.Contract.Passed {
		contractStatus = "CONTRACT_BREACH"
	}
	sb.WriteString(fmt.Sprintf(
		"| **0. Contracts** | Do declared hard domains hold? | **%s** | %d/%d series breached (%d observations) |\n",
		contractStatus, report.Contract.BreachingMetricsCount,
		report.Contract.TotalMetricsChecked, report.Contract.TotalBreaches,
	))
	sb.WriteString(fmt.Sprintf(
		"| **1. Vitality** | What raw/canonical evidence actually exists? | **%s** | %d raw series; %d canonical cells; %d constant canonical cells |\n",
		report.Vitality.Status, report.Vitality.RawProducerMetrics,
		report.Vitality.CanonicalGridCells, report.Vitality.CanonicalDeadCells,
	))
	sb.WriteString(fmt.Sprintf(
		"| **2. Sympathy** | Do observed deformations relate beyond a mask-preserving shuffled null? | **%s** | %d pairs; |null| p95 %.3f; %.1f%% real |r| above it; KS %.3f |\n",
		report.Sympathy.Status, report.Sympathy.TotalPairs,
		report.Sympathy.NullDistribution.Percentile95,
		report.Sympathy.SeparationRatio*100, report.Sympathy.KSStatistic,
	))
	sb.WriteString(fmt.Sprintf(
		"| **3. Grid reproducibility** | Do disjoint periods recover the same co-memberships? | **%s** | ARI %.3f; %.1f%% universe overlap (%d shared cells) |\n",
		report.GridStability.Status, report.GridStability.AdjustedRandIdx,
		report.GridStability.OverlapFraction*100, report.GridStability.SharedUniverse,
	))
	sb.WriteString(fmt.Sprintf(
		"| **4. Token dynamics** | What does a frozen grid emit on unseen tape? | **%s** | %d emissions; %d regions; H=%.3f vs null mean %.3f |\n",
		report.TokenDynamics.Status, report.TokenDynamics.TotalEmissions,
		report.TokenDynamics.UniqueTokens, report.TokenDynamics.TransitionEntropy,
		report.TokenDynamics.NullTransitionEntropy,
	))
	sb.WriteString(fmt.Sprintf(
		"| **5. Precursors** | Are A->B and B->C populations measurable? | **A->B %s / B->C %s** | A->B N=%d/%d, JSD %.3f vs null95 %.3f; B->C N=%d/%d |\n\n",
		report.Precursor.IgnitionHypothesis.Status,
		report.Precursor.ExhaustionHypothesis.Status,
		report.Precursor.IgnitionHypothesis.EventTokenCount,
		report.Precursor.IgnitionHypothesis.ControlTokenCount,
		report.Precursor.IgnitionHypothesis.DivergenceBits,
		report.Precursor.IgnitionHypothesis.NullDivergence95,
		report.Precursor.ExhaustionHypothesis.EventTokenCount,
		report.Precursor.ExhaustionHypothesis.ControlTokenCount,
	))

	sb.WriteString("---\n\n")
	sb.WriteString("### Stage 0: Declared mathematical contracts\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Series checked: `%d`\n- Series with hard-domain breaches: `%d`\n- Breach observations: `%d`\n\n",
		report.Contract.TotalMetricsChecked, report.Contract.BreachingMetricsCount,
		report.Contract.TotalBreaches,
	))
	if len(report.Contract.Breaches) > 0 {
		sb.WriteString("| Metric | Unit | Declared domain | Observed range | Breaches |\n")
		sb.WriteString("| :--- | :---: | :---: | :---: | ---: |\n")
		for index, breach := range report.Contract.Breaches {
			if index >= 12 {
				break
			}
			sb.WriteString(fmt.Sprintf(
				"| `%s` | `%s` | `%s` | `[%.3f, %.3f]` | %d/%d |\n",
				breach.Metric, breach.DeclaredUnit, breach.DeclaredDomain,
				breach.MinVal, breach.MaxVal, breach.BreachCount, breach.TotalSamples,
			))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.\n\n")
	sb.WriteString("![Stage 0](plots/stage0_metric_contracts.png)\n\n")

	sb.WriteString("### Stage 1: Observed metric population\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Raw named series: `%d` (varying `%d`, constant `%d`)\n"+
			"- Canonical grid cells: `%d` (varying `%d`, constant `%d`)\n"+
			"- High-correlation canonical pairs shown by the current reference filter: `%d`\n\n",
		report.Vitality.RawProducerMetrics, report.Vitality.RawHealthyMetrics,
		report.Vitality.RawDeadMetrics, report.Vitality.CanonicalGridCells,
		report.Vitality.CanonicalHealthyCells, report.Vitality.CanonicalDeadCells,
		len(report.Vitality.RedundantPairs),
	))
	sb.WriteString("> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.\n\n")
	sb.WriteString("![Stage 1 Vitality](plots/stage1_metric_vitality.png)\n\n")
	sb.WriteString("![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)\n\n")

	sb.WriteString("### Stage 2: Sympathy against an empirical null\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Simultaneously observed pairs: `%d`\n"+
			"- Direct relationships: `%d`; inverse relationships: `%d`\n"+
			"- Signed real mean: `%.3f`; signed shuffled mean: `%.3f`\n"+
			"- 95th percentile of `|null r|`: `%.3f`\n"+
			"- Real `|r|` above that empirical bound: `%.1f%%`\n"+
			"- KS distance in `|r|` space: `%.3f`\n\n",
		report.Sympathy.TotalPairs, report.Sympathy.PositivePairs,
		report.Sympathy.InversePairs, report.Sympathy.RealMean,
		report.Sympathy.NullDistribution.MeanConcordance,
		report.Sympathy.NullDistribution.Percentile95,
		report.Sympathy.SeparationRatio*100, report.Sympathy.KSStatistic,
	))
	sb.WriteString("> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.\n\n")
	sb.WriteString("![Stage 2 Sympathy](plots/stage2_sympathy_null.png)\n\n")
	sb.WriteString("![Stage 2 Orientation](plots/stage2_orientation_balance.png)\n\n")

	sb.WriteString("### Stage 3: Grid reproducibility\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Early grid: `%d` cells / `%d` regions\n"+
			"- Late grid: `%d` cells / `%d` regions\n"+
			"- Shared universe: `%d` cells (`%.1f%%`)\n"+
			"- Adjusted Rand Index: `%.3f`\n\n",
		report.GridStability.GridA.CellCount, report.GridStability.GridA.RegionCount,
		report.GridStability.GridB.CellCount, report.GridStability.GridB.RegionCount,
		report.GridStability.SharedUniverse, report.GridStability.OverlapFraction*100,
		report.GridStability.AdjustedRandIdx,
	))
	sb.WriteString("> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.\n\n")
	sb.WriteString("![Stage 3](plots/stage3_region_partitioning.png)\n\n")

	sb.WriteString("### Stage 4: Held-out token dynamics\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Held-out emissions: `%d` across `%d` regions\n"+
			"- Maximum observed token share: `%.1f%%`\n"+
			"- Real transition entropy: `%.3f` bits\n"+
			"- Empirical dwell-block null mean: `%.3f` bits\n"+
			"- Difference (null - real): `%.3f` bits\n\n",
		report.TokenDynamics.TotalEmissions, report.TokenDynamics.UniqueTokens,
		report.TokenDynamics.MaxTokenDominance*100,
		report.TokenDynamics.TransitionEntropy,
		report.TokenDynamics.NullTransitionEntropy,
		report.TokenDynamics.EntropyReductionBits,
	))
	sb.WriteString("> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.\n\n")
	sb.WriteString("![Stage 4 Tokens](plots/stage4_token_dynamics.png)\n\n")
	sb.WriteString("![Stage 4 Entropy](plots/stage4_transition_entropy.png)\n\n")
	sb.WriteString("![Stage 4 Matrix](plots/stage4_transition_matrix.png)\n\n")

	sb.WriteString("### Stage 5: Event-centred precursor populations\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Detections: `%d` (%s)\n"+
			"- A->B: `%s`, event/control `%d/%d`, JSD `%.3f`, null95 `%.3f`\n"+
			"- B->C: `%s`, event/control `%d/%d`, JSD `%.3f`, null95 `%.3f`\n"+
			"- Supplemental non-excursion background observations: `%d`\n\n",
		report.Precursor.DetectionsFound,
		strings.Join(report.Precursor.ExcursionsFound, ", "),
		report.Precursor.IgnitionHypothesis.Status,
		report.Precursor.IgnitionHypothesis.EventTokenCount,
		report.Precursor.IgnitionHypothesis.ControlTokenCount,
		report.Precursor.IgnitionHypothesis.DivergenceBits,
		report.Precursor.IgnitionHypothesis.NullDivergence95,
		report.Precursor.ExhaustionHypothesis.Status,
		report.Precursor.ExhaustionHypothesis.EventTokenCount,
		report.Precursor.ExhaustionHypothesis.ControlTokenCount,
		report.Precursor.ExhaustionHypothesis.DivergenceBits,
		report.Precursor.ExhaustionHypothesis.NullDivergence95,
		totalTokenCount(report.Precursor.BackgroundTokens),
	))
	sb.WriteString("> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.\n\n")
	sb.WriteString("![Stage 5](plots/stage5_precursor_separation.png)\n\n")

	return sb.String()
}
