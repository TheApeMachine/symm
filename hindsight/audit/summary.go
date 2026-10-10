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

	sb.WriteString("# SYMM Pipeline Empirical Audit\n\n")
	sb.WriteString(fmt.Sprintf(
		"**State:** %s | **Epoch:** `%d` | **Symbol:** `%s` | **Ticks:** `%d` | **Generated:** `%s`\n\n",
		report.Overall, report.Epoch, report.Symbol, report.TotalTicks, report.Timestamp,
	))
	sb.WriteString("Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.\n\n")
	sb.WriteString(fmt.Sprintf(
		"Thresholds: significance `%.2f`, latency spike `%.0fms` on at most `%.0f%%`, dominance `%.2f` bits, duplication `%.2f` bits.\n\n",
		report.Thresholds.Significance, report.Thresholds.SpikeLatencyMs, report.Thresholds.SpikeFraction*100,
		report.Thresholds.DominanceJSD, report.Thresholds.DuplicationJSD,
	))
	sb.WriteString("| Stage | Verdict | Criterion | Evidence |\n")
	sb.WriteString("| :--- | :---: | :--- | :--- |\n")

	for _, row := range report.Verdicts {
		sb.WriteString(fmt.Sprintf("| **%s** | **%s** | %s | %s |\n", row.Stage, row.Verdict, row.Criterion, row.Evidence))
	}

	sb.WriteString("\n")

	sb.WriteString("---\n\n")
	sb.WriteString("### Stage 0: Declared mathematical contracts\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Series checked: `%d`\n"+
			"- Hard-domain breaches: `%d` series (`%d` observations)\n"+
			"- Standardized z-score bound breaches: `%d` observations beyond Chebyshev bound |z| <= sqrt(N/%.2f) (max |z|=`%.2f`, saturated=`%.1f%%`)\n"+
			"- Normalization breaches: `%d`, Standardization breaches: `%d`\n\n",
		report.Contract.TotalMetricsChecked, report.Contract.BreachingMetricsCount,
		report.Contract.TotalBreaches,
		report.Contract.MetricNorm.ZMagnitudeBreaches, report.Thresholds.Significance,
		report.Contract.MetricNorm.MaxAbsoluteZ, report.Contract.MetricNorm.SaturatedNormFraction*100,
		report.Contract.MetricNorm.NormalizationBreaches, report.Contract.MetricNorm.StandardizationBreaches,
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

	sb.WriteString("### Stage 0.5: Ingestion clock timing & synchronization\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Observations checked: `%d`\n"+
			"- Ingestion latency (Timestamp - At): mean `%.1fms`, p95 `%.1fms`, max `%.1fms`\n"+
			"- Latency spikes: `%d`\n"+
			"- Sequence inversions (timestamp regressions): `%d`\n"+
			"- Feed status: `%s`\n\n",
		report.Timing.TotalChecked,
		report.Timing.MeanDriftMs,
		report.Timing.P95DriftMs,
		report.Timing.MaxDriftMs,
		report.Timing.LatencySpikes,
		report.Timing.SequenceInversions,
		report.Timing.Status,
	))
	sb.WriteString("> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.\n\n")

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

	deadCanon := make([]MetricStat, 0)
	for _, cell := range report.Vitality.CanonicalCells {
		if cell.Status == "DEAD" || cell.Status == "ZERO" {
			deadCanon = append(deadCanon, cell)
		}
	}

	if len(deadCanon) > 0 {
		sb.WriteString("#### Constant / Dead Canonical Grid Cells\n\n")
		sb.WriteString("| Canonical Cell | Coverage | Zero Fraction | Range | Status |\n")
		sb.WriteString("| :--- | :---: | :---: | :---: | :---: |\n")

		for index, cell := range deadCanon {
			if index >= 50 {
				sb.WriteString(fmt.Sprintf("| ... and %d more dead cells | | | | |\n", len(deadCanon)-50))
				break
			}

			sb.WriteString(fmt.Sprintf(
				"| `%s` | `%.1f%%` | `%.1f%%` | `[%.3f, %.3f]` | `%s` |\n",
				cell.Name, cell.Coverage*100, cell.ZeroFraction*100, cell.Min, cell.Max, cell.Status,
			))
		}

		sb.WriteString("\n")
	}

	starvingCanon := make([]MetricStat, 0)
	for _, cell := range report.Vitality.CanonicalCells {
		if cell.Status == "HEALTHY" && cell.ZeroFraction >= 0.8 {
			starvingCanon = append(starvingCanon, cell)
		}
	}

	if len(starvingCanon) > 0 {
		sb.WriteString("#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)\n\n")
		sb.WriteString("| Canonical Cell | Coverage | Zero Fraction | Mean | Status |\n")
		sb.WriteString("| :--- | :---: | :---: | :---: | :---: |\n")
		for index, cell := range starvingCanon {
			if index >= 15 {
				sb.WriteString(fmt.Sprintf("| ... and %d more stagnant cells | | | | |\n", len(starvingCanon)-15))
				break
			}
			sb.WriteString(fmt.Sprintf(
				"| `%s` | `%.1f%%` | `%.1f%%` | `%.4f` | `COLD_START` |\n",
				cell.Name, cell.Coverage*100, cell.ZeroFraction*100, cell.Mean,
			))
		}
		sb.WriteString("\n")
	}

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

	sb.WriteString("### Stage 4: Held-out token dynamics & excitation strength\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Held-out emissions: `%d` across `%d` regions\n"+
			"- Excitation strength: mean `%.3f`, peak `%.3f`, runner-up margin `%.3f`\n"+
			"- Active cell coverage: `%.1f%%` mean\n"+
			"- Maximum observed token share: `%.1f%%`\n"+
			"- Real transition entropy: `%.3f` bits\n"+
			"- Empirical dwell-block null mean: `%.3f` bits\n"+
			"- Difference (null - real): `%.3f` bits\n\n",
		report.TokenDynamics.TotalEmissions, report.TokenDynamics.UniqueTokens,
		report.TokenDynamics.MeanExcitationStrength, report.TokenDynamics.PeakExcitationStrength,
		report.TokenDynamics.MeanRunnerUpMargin,
		report.TokenDynamics.MeanActiveCoverage*100,
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
		"#### 1. Statistical Separation\n"+
			"- Detections: `%d` (%s)\n"+
			"- A->B Ignition: `%s`, event/control `%d/%d`, JSD `%.3f` vs null95 `%.3f`\n"+
			"- B->C Exhaustion: `%s`, event/control `%d/%d`, JSD `%.3f` vs null95 `%.3f`\n"+
			"- Supplemental non-excursion background observations: `%d`\n\n"+
			"#### 2. Predictive Skill (Anticipation)\n"+
			"- Balanced Accuracy: `%.1f%%` | MCC: `%.3f`\n"+
			"- Precision / Recall: `%.1f%%` / `%.1f%%`\n"+
			"- Mutual Information (Predictive Gain): `%.3f` bits\n"+
			"- Prior Base Rate: `%.1f%%` | Top Precursor Tokens: `%s`\n\n"+
			"#### 3. Economic Relevance (Friction Clearance)\n"+
			"- Evaluated Excursions: `%d` | Round-Trip Taker Fee: `%.2f` bps\n"+
			"- Friction Clearance Rate: `%.1f%%` (`%d` profitable / `%d` unprofitable)\n"+
			"- Gross Mean Return: `%.2f%%` | Net Mean Return after Fees: `%.2f%%`\n\n",
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
		report.Precursor.PredictiveSkill.BalancedAccuracy*100,
		report.Precursor.PredictiveSkill.MCC,
		report.Precursor.PredictiveSkill.Precision*100,
		report.Precursor.PredictiveSkill.Recall*100,
		report.Precursor.PredictiveSkill.PredictiveGainBits,
		report.Precursor.PredictiveSkill.PriorBaseRate*100,
		strings.Join(report.Precursor.PredictiveSkill.TopPrecursorTokens, ", "),
		report.Precursor.EconomicRelevance.EvaluatedExcursions,
		report.Precursor.EconomicRelevance.RoundTripFeeRate*10000,
		report.Precursor.EconomicRelevance.FrictionClearanceRate*100,
		report.Precursor.EconomicRelevance.ProfitableExcursions,
		report.Precursor.EconomicRelevance.UnprofitableExcursions,
		report.Precursor.EconomicRelevance.GrossMeanReturn*100,
		report.Precursor.EconomicRelevance.NetMeanReturn*100,
	))
	sb.WriteString("> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.\n\n")
	sb.WriteString("![Stage 5](plots/stage5_precursor_separation.png)\n\n")

	sb.WriteString("### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics\n\n")
	sb.WriteString(fmt.Sprintf(
		"- Evaluated excursions: `%d` forming `%d` sequential phases (enter: `%d`, exit: `%d`, wait: `%d`)\n"+
			"- Balanced accuracy: `%.1f%%` vs best baseline (`%s`): `%.1f%%` (Raw hit rate: `%d/%d` `%.1f%%`)\n"+
			"- Matthews Correlation Coefficient (MCC): `%.3f`\n"+
			"- Enter action precision / recall: `%.1f%%` / `%.1f%%`\n"+
			"- Label-shuffled empirical null balanced accuracy: mean `%.1f%%`, 95th percentile `%.1f%%` (empirical p-value: `%.3f`)\n"+
			"- Separates from null: `%t`\n"+
			"- Post-teach memory retention: `%d/%d` (`%.1f%%`)\n"+
			"- Trie topology: `%d` nodes, max depth `%d`, mean depth `%.1f`, branching factor `%.2f`\n"+
			"- Basin geometry: records `%.0f`, span `%.0f`, active enter basins `%.0f`, active exit basins `%.0f` (total: `%d`)\n"+
			"- Decisiveness: abstention rate `%.1f%%`, mean confidence `%.3f`, mean contrast `%.3f`\n"+
			"- Unseen background false-alarm rate: `%.2f%%` spurious triggers on continuous tape\n\n",
		report.CognitiveTrie.DetectionsEvaluated,
		report.CognitiveTrie.PhasesFormed,
		report.CognitiveTrie.ActionCounts["enter"],
		report.CognitiveTrie.ActionCounts["exit"],
		report.CognitiveTrie.ActionCounts["wait"],
		report.CognitiveTrie.Skill.BalancedAccuracy*100,
		report.CognitiveTrie.Skill.BestBaselinePolicy,
		report.CognitiveTrie.Skill.BaselineBalancedAccuracy*100,
		report.CognitiveTrie.Skill.Hits,
		report.CognitiveTrie.Skill.TotalCalls,
		report.CognitiveTrie.Skill.HitRate*100,
		report.CognitiveTrie.Skill.MCC,
		report.CognitiveTrie.Skill.EnterPrecision*100,
		report.CognitiveTrie.Skill.EnterRecall*100,
		report.CognitiveTrie.Skill.NullMeanBalancedAccuracy*100,
		report.CognitiveTrie.Skill.Null95thBalancedAccuracy*100,
		report.CognitiveTrie.Skill.EmpiricalPValue,
		report.CognitiveTrie.Skill.SeparatesFromNull,
		report.CognitiveTrie.Retention.RetainedCount,
		report.CognitiveTrie.Retention.TotalTaught,
		report.CognitiveTrie.Retention.RetentionRate*100,
		report.CognitiveTrie.Topology.NodeStats.TotalNodes,
		report.CognitiveTrie.Topology.NodeStats.MaxDepth,
		report.CognitiveTrie.Topology.NodeStats.MeanDepth,
		report.CognitiveTrie.Topology.NodeStats.BranchingFactor,
		report.CognitiveTrie.Topology.RecordsCount,
		report.CognitiveTrie.Topology.SpanCount,
		report.CognitiveTrie.Topology.EnterBasins,
		report.CognitiveTrie.Topology.ExitBasins,
		report.CognitiveTrie.Topology.TotalBasins,
		report.CognitiveTrie.AbstentionRate*100,
		report.CognitiveTrie.MeanConfidence,
		report.CognitiveTrie.MeanContrast,
		report.CognitiveTrie.SpuriousTriggerRate*100,
	))
	if report.CognitiveTrie.Skill.TotalCalls > 0 && report.CognitiveTrie.Skill.BalancedAccuracy < report.CognitiveTrie.Skill.BaselineBalancedAccuracy {
		sb.WriteString(fmt.Sprintf(
			"> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (%.1f%%) trails baseline policy (%.1f%%). Memory retention is at %.1f%%.\n\n",
			report.CognitiveTrie.Skill.BalancedAccuracy*100, report.CognitiveTrie.Skill.BaselineBalancedAccuracy*100, report.CognitiveTrie.Retention.RetentionRate*100,
		))
	}
	sb.WriteString("> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.\n\n")
	sb.WriteString("![Stage 6 Skill](plots/stage6_trie_skill.png)\n\n")
	sb.WriteString("![Stage 6 Structure](plots/stage6_trie_structure.png)\n\n")

	if report.Equivalence != nil {
		sb.WriteString("### Validation 1: Production-vs-Audit Equivalence\n\n")
		sb.WriteString(fmt.Sprintf(
			"- Replayed ticks: `%d`\n"+
				"- Tokens verified: `%d`\n"+
				"- Token mismatches: `%d`\n"+
				"- Metric/brightness mismatches: `%d`\n"+
				"- Executable path: `%s`\n"+
				"- Verdict: `%s` (one code path compared with itself)\n\n",
			report.Equivalence.TotalTicksReplayed,
			report.Equivalence.TotalTokensChecked,
			report.Equivalence.TokenMismatches,
			report.Equivalence.MetricMismatches,
			report.Equivalence.ExecutablePath,
			report.Equivalence.Status,
		))
	}

	if report.Truthfulness != nil {
		sb.WriteString("### Validation 2: Metric Truthfulness\n\n")
		sb.WriteString(fmt.Sprintf(
			"- Measurements audited: `%d`\n"+
				"- Physical invariant violations: `%d`\n"+
				"- Same-time ambiguous trades skipped: `%d`\n"+
				"- Comparisons per check: `%v`\n"+
				"- Violations per check: `%v`\n"+
				"- Unexercised checks: `%v`\n"+
				"- Verdict: `%s`\n\n",
			report.Truthfulness.TotalChecked,
			report.Truthfulness.ViolationsCount,
			report.Truthfulness.AmbiguousTrades,
			report.Truthfulness.Comparisons,
			report.Truthfulness.ViolationsByCheck,
			report.Truthfulness.UnexercisedChecks,
			report.Truthfulness.Status,
		))
	}

	if report.Causality != nil {
		sb.WriteString("### Validation 3: Causality & State Isolation\n\n")
		sb.WriteString(fmt.Sprintf(
			"- Future perturbation ticks: `%d`\n"+
				"- Lookahead leakage detected: `%t` (compared: `%d`, contaminated: `%d`)\n"+
				"- Cross-symbol contamination: `%t`\n"+
				"- Epoch isolation passed: `%t`\n"+
				"- Verdict: `%s`\n\n",
			report.Causality.FuturePerturbationTicks,
			report.Causality.LeakageDetected,
			report.Causality.ComparedObservations,
			report.Causality.ContaminatedCount,
			report.Causality.CrossSymbolLeakage,
			report.Causality.EpochIsolationPassed,
			report.Causality.Status,
		))
	}

	if report.Sensitivity != nil {
		sb.WriteString("### Validation 4: Grid Dependence & Sensitivity\n\n")
		sb.WriteString(fmt.Sprintf(
			"- Families evaluated (LOFO): `%d`\n"+
				"- Duplication resistant: `%t`\n"+
				"- Verdict: `%s`\n\n",
			len(report.Sensitivity.FamiliesTested),
			report.Sensitivity.DuplicationResistant,
			report.Sensitivity.Status,
		))
		for _, f := range report.Sensitivity.FamiliesTested {
			sb.WriteString(fmt.Sprintf("  - Family `%s`: Removed JSD = `%.3f` bits (dominant: `%t`)\n", f.Family, f.RemovedJSD, f.IsDominant))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
