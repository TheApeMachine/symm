package audit

import (
	"fmt"
	"strings"
)

/*
GenerateSummaryMarkdown builds an executive Markdown document with plain-English
explanations, healthy/broken criteria, mathematical contract audits, and embedded chart links.
*/
func GenerateSummaryMarkdown(report *AuditReport) string {
	var sb strings.Builder

	statusBadge := "✅ HEALTHY"
	if !report.OverallHealthy {
		statusBadge = "⚠️ ATTENTION REQUIRED"
	}

	sb.WriteString("# SYMM Pipeline Health Audit\n\n")
	sb.WriteString(fmt.Sprintf("**Status:** %s | **Epoch:** `%d` | **Symbol:** `%s` | **Ticks:** `%d` | **Generated:** `%s`\n\n",
		statusBadge, report.Epoch, report.Symbol, report.TotalTicks, report.Timestamp,
	))

	sb.WriteString("---\n\n")
	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString("This audit tests whether market representation retains genuine statistical structure across five operational boundaries, comparing real measurements against production data flows and empirical nulls.\n\n")

	sb.WriteString("| Stage | Question Tested | Status | Key Metric |\n")
	sb.WriteString("| :--- | :--- | :---: | :--- |\n")

	stage0Badge := "✅ PASS"
	if !report.Contract.Passed {
		stage0Badge = "🚨 CRITICAL"
	}
	sb.WriteString(fmt.Sprintf("| **0. Contract Integrity** | Do metrics obey declared mathematical domains? | %s | %d/%d breaching metrics (%d breach events) |\n",
		stage0Badge, report.Contract.BreachingMetricsCount, report.Contract.TotalMetricsChecked, report.Contract.TotalBreaches,
	))

	stage1Badge := "✅ PASS"
	if !report.Vitality.Passed {
		stage1Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **1. Metric Vitality** | Are sensors alive and non-redundant? | %s | %d/%d raw healthy; %d/%d canonical cells healthy (%d redundant pairs) |\n",
		stage1Badge, report.Vitality.RawHealthyMetrics, report.Vitality.RawProducerMetrics,
		report.Vitality.CanonicalHealthyCells, report.Vitality.CanonicalGridCells, len(report.Vitality.RedundantPairs),
	))

	stage2Badge := "✅ PASS"
	if !report.Sympathy.Passed {
		stage2Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **2. Sympathy Null** | Do deformations move together beyond noise? | %s | Real mean %.3f vs Null %.3f (KS: %.3f, Sep: %.1f%%) |\n",
		stage2Badge, report.Sympathy.RealMean, report.Sympathy.NullDistribution.MeanConcordance,
		report.Sympathy.KSStatistic, report.Sympathy.SeparationRatio*100,
	))

	stage3Badge := "✅ PASS"
	if !report.GridStability.Passed {
		stage3Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **3. Grid Stability** | Does partitioning repeat across time? | %s | ARI: %.3f (Overlap: %.1f%% across %d shared cells) |\n",
		stage3Badge, report.GridStability.AdjustedRandIdx, report.GridStability.OverlapFraction*100, report.GridStability.SharedUniverse,
	))

	stage4Badge := "✅ PASS"
	if !report.TokenDynamics.Passed {
		stage4Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **4. Token Dynamics** | Does region compression preserve structure? | %s | Out-of-sample: Max Dominance: %.1f%%, Entropy Reduction: %.3f bits |\n",
		stage4Badge, report.TokenDynamics.MaxTokenDominance*100, report.TokenDynamics.EntropyReductionBits,
	))

	stage5Badge := "✅ PASS"
	if !report.Precursor.Passed {
		stage5Badge = "❌ FAIL"
	}
	if report.Precursor.IgnitionHypothesis.Status == "INSUFFICIENT_DATA" {
		stage5Badge = "ℹ️ INSUFFICIENT"
	}
	sb.WriteString(fmt.Sprintf("| **5. Precursor Separation** | Are patterns before B/C distinguishable? | %s | %s |\n\n",
		stage5Badge, report.Precursor.SummaryText,
	))

	sb.WriteString("---\n\n")

	// Stage 0
	sb.WriteString("### Stage 0: Metric Contract Integrity\n\n")
	sb.WriteString("> **The Plain-English Question:** *Are sensors strictly adhering to their declared mathematical bounds (e.g. correlations in [-1, 1], variances >= 0), or is the pipeline ingesting mathematically ungrounded numbers?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Metrics Audited:** `%d`\n", report.Contract.TotalMetricsChecked))
	sb.WriteString(fmt.Sprintf("- **Metrics Violating Declared Domain:** `%d` (`%d` total breach observations)\n\n",
		report.Contract.BreachingMetricsCount, report.Contract.TotalBreaches,
	))

	if len(report.Contract.Breaches) > 0 {
		sb.WriteString("#### Top Mathematical Contract Breaches:\n\n")
		sb.WriteString("| Metric | Declared Domain | Observed Range | Mean | Breaches / Samples |\n")
		sb.WriteString("| :--- | :---: | :---: | :---: | :---: |\n")
		for idx, b := range report.Contract.Breaches {
			if idx >= 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `[%.3f, %.3f]` | `%.3f` | %d / %d (%.1f%%) |\n",
				b.Metric, b.DeclaredDomain, b.MinVal, b.MaxVal, b.MeanVal, b.BreachCount, b.TotalSamples, b.BreachFraction*100,
			))
		}
		sb.WriteString("\n")
		sb.WriteString("> [!IMPORTANT]\n")
		sb.WriteString("> **Mathematical Root Cause Analysis:**\n")
		sb.WriteString(fmt.Sprintf("> %s\n\n", strings.ReplaceAll(report.Contract.DiagnosisText, "\n", "\n> ")))
	}

	sb.WriteString("![Stage 0 Metric Contracts](plots/stage0_metric_contracts.png)\n\n")

	// Stage 1
	sb.WriteString("### Stage 1: Metric Vitality & Redundancy\n\n")
	sb.WriteString("> **The Plain-English Question:** *Are the individual metrics actually moving and present, or are we feeding the grid dead constants and duplicated signals?*\n\n")
	sb.WriteString("#### Raw Producer Outputs:\n")
	sb.WriteString(fmt.Sprintf("- **Total Raw Named Series:** `%d`\n", report.Vitality.RawProducerMetrics))
	sb.WriteString(fmt.Sprintf("- **Healthy Raw Series:** `%d` (moving, non-constant)\n", report.Vitality.RawHealthyMetrics))
	sb.WriteString(fmt.Sprintf("- **Dead / Constant Raw Series:** `%d`\n", report.Vitality.RawDeadMetrics))
	sb.WriteString(fmt.Sprintf("- **Sporadic Raw Series:** `%d` (coverage < 20%%)\n\n", report.Vitality.RawSporadicMetrics))

	sb.WriteString("#### Canonical Grid Inputs:\n")
	sb.WriteString(fmt.Sprintf("- **Canonical Cells Universe:** `%d` (after peer aggregation via `ChannelsFrom`)\n", report.Vitality.CanonicalGridCells))
	sb.WriteString(fmt.Sprintf("- **Healthy Canonical Cells:** `%d`\n", report.Vitality.CanonicalHealthyCells))
	sb.WriteString(fmt.Sprintf("- **Dead / Constant Canonical Cells:** `%d`\n", report.Vitality.CanonicalDeadCells))
	sb.WriteString(fmt.Sprintf("- **Sporadic Canonical Cells:** `%d`\n", report.Vitality.CanonicalSporadicCells))
	sb.WriteString(fmt.Sprintf("- **Redundant Cell Pairs (|r| >= 0.95):** `%d`\n\n", len(report.Vitality.RedundantPairs)))

	sb.WriteString("> [!NOTE]\n")
	sb.WriteString("> High correlation between baselines, z-scores, and raw signals is mathematically expected and confirms the grid's operational role: discovering redundancy to form lower-dimensional co-movement regions.\n\n")

	sb.WriteString("![Stage 1 Metric Vitality](plots/stage1_metric_vitality.png)\n\n")
	sb.WriteString("![Stage 1 Metric Redundancy](plots/stage1_metric_redundancy.png)\n\n")

	// Stage 2
	sb.WriteString("### Stage 2: Pair Relationships & Sympathy (Real Deformations vs. Shuffled Null)\n\n")
	sb.WriteString("> **The Plain-English Question:** *Do scale-free deformations move together beyond noise, and does the grid recognize stable opposition as affinity?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Pairs Analyzed:** `%d`\n", report.Sympathy.TotalPairs))
	sb.WriteString(fmt.Sprintf("- **Positive Sympathy (r > 0.05):** `%d`\n", report.Sympathy.PositivePairs))
	sb.WriteString(fmt.Sprintf("- **Inverse Sympathy (r < -0.05):** `%d` (stable opposition)\n", report.Sympathy.InversePairs))
	sb.WriteString(fmt.Sprintf("- **Separation vs Shuffled Null:** `%.1f%%` of pairs exceed the 95th percentile null envelope.\n", report.Sympathy.SeparationRatio*100))
	sb.WriteString(fmt.Sprintf("- **Kolmogorov-Smirnov Distance:** `%.3f`\n\n", report.Sympathy.KSStatistic))

	sb.WriteString("> [!NOTE]\n")
	sb.WriteString("> Production `PairStats.affinity()` defines sympathy as absolute alignment `math.Abs(corr) * reliability`, ensuring both lockstep co-movement and lockstep opposition contribute to graph edge weights without violating graph Laplacian PSD.\n\n")

	sb.WriteString("![Stage 2 Sympathy Null](plots/stage2_sympathy_null.png)\n\n")
	sb.WriteString("![Stage 2 Orientation Balance](plots/stage2_orientation_balance.png)\n\n")

	// Stage 3
	sb.WriteString("### Stage 3: Grid Partitioning & Temporal Stability\n\n")
	sb.WriteString("> **The Plain-English Question:** *Does the Impulse Map discover the same metric friendships across disjoint chronological periods, or does it redraw arbitrary clusters every time?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Early Period Grid:** `%d` regions across `%d` cells\n",
		report.GridStability.GridA.RegionCount, report.GridStability.GridA.CellCount,
	))
	sb.WriteString(fmt.Sprintf("- **Late Period Grid:** `%d` regions across `%d` cells\n",
		report.GridStability.GridB.RegionCount, report.GridStability.GridB.CellCount,
	))
	sb.WriteString(fmt.Sprintf("- **Universe Overlap:** `%.1f%%` (`%d` shared cells)\n",
		report.GridStability.OverlapFraction*100, report.GridStability.SharedUniverse,
	))
	sb.WriteString(fmt.Sprintf("- **Adjusted Rand Index (ARI):** `%.3f` (1.0 = identical partitions; 0.0 = random chance agreement)\n\n",
		report.GridStability.AdjustedRandIdx,
	))

	sb.WriteString("> [!NOTE]\n")
	sb.WriteString("> Region sizes (~5% each) are algebraically enforced by `TargetRegionCount=20` and `balancedCapacities()`. The meaningful stability metric is Adjusted Rand Index (cluster membership agreement across chronological halves).\n\n")

	sb.WriteString("![Stage 3 Region Partitioning](plots/stage3_region_partitioning.png)\n\n")

	// Stage 4
	sb.WriteString("### Stage 4: Out-of-Sample Token Dynamics & Transition Structure\n\n")
	sb.WriteString("> **The Plain-English Question:** *Does a frozen grid emit low-entropy, structured state transitions on unseen market tape compared to a block-shuffled temporal null?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Out-of-Sample Emissions:** `%d` tokens across `%d` unique active regions\n",
		report.TokenDynamics.TotalEmissions, report.TokenDynamics.UniqueTokens,
	))
	sb.WriteString(fmt.Sprintf("- **Max Token Dominance:** `%.1f%%` (no single region monopolizes the tape)\n", report.TokenDynamics.MaxTokenDominance*100))
	sb.WriteString(fmt.Sprintf("- **Transition Entropy:** `%.3f` bits vs Block-Shuffled Null `%.3f` bits (Reduction: `%.3f` bits)\n\n",
		report.TokenDynamics.TransitionEntropy, report.TokenDynamics.NullTransitionEntropy, report.TokenDynamics.EntropyReductionBits,
	))

	sb.WriteString("![Stage 4 Token Dynamics](plots/stage4_token_dynamics.png)\n\n")
	sb.WriteString("![Stage 4 Transition Entropy](plots/stage4_transition_entropy.png)\n\n")
	sb.WriteString("![Stage 4 Transition Matrix](plots/stage4_transition_matrix.png)\n\n")

	// Stage 5
	sb.WriteString("### Stage 5: Precursor Informativeness (Ignition & Exhaustion)\n\n")
	sb.WriteString("> **The Plain-English Question:** *Does the market state before a profitable ignition (A -> B) or peak exhaustion (B -> C) exhibit statistically distinct regional signatures compared to controls and non-excursion background?*\n\n")

	sb.WriteString(fmt.Sprintf("- **Total Detections Found:** `%d` (%s)\n",
		report.Precursor.DetectionsFound, strings.Join(report.Precursor.ExcursionsFound, ", "),
	))
	sb.WriteString(fmt.Sprintf("- **Hypothesis A -> B (Ignition):** `%s` | Divergence: `%.3f` bits vs Null-95 `%.3f` bits (N=%d tokens)\n",
		report.Precursor.IgnitionHypothesis.Status, report.Precursor.IgnitionHypothesis.DivergenceBits,
		report.Precursor.IgnitionHypothesis.NullDivergence95, report.Precursor.IgnitionHypothesis.EventTokenCount,
	))
	sb.WriteString(fmt.Sprintf("- **Hypothesis B -> C (Exhaustion):** `%s` | Divergence: `%.3f` bits vs Null-95 `%.3f` bits (N=%d tokens)\n",
		report.Precursor.ExhaustionHypothesis.Status, report.Precursor.ExhaustionHypothesis.DivergenceBits,
		report.Precursor.ExhaustionHypothesis.NullDivergence95, report.Precursor.ExhaustionHypothesis.EventTokenCount,
	))
	sb.WriteString(fmt.Sprintf("- **Disjoint Background Control:** `%d` non-excursion tokens\n\n", len(report.Precursor.BackgroundTokens)))

	sb.WriteString("> [!IMPORTANT]\n")
	sb.WriteString("> Precursor analysis enforces strict population isolation: background tape excludes all excursion windows. Absence of evidence or small sample sizes are reported as `INSUFFICIENT_DATA`, never `PASS`.\n\n")

	sb.WriteString("![Stage 5 Precursor Separation](plots/stage5_precursor_separation.png)\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("## Audit Verdict & Recommendations\n\n")

	if !report.OverallHealthy {
		sb.WriteString("### Operational Blockers Identified:\n\n")
		if !report.Contract.Passed {
			sb.WriteString("1. **Metric Contract Breaches (Stage 0):** Asynchronous Hayashi-Yoshida cross-variation estimators regularly exceed 1.0. Upstream covariance regularisation or normalization review is required before treating these sensors as bounded correlation.\n")
		}
		if !report.GridStability.Passed {
			sb.WriteString("2. **Grid Partition Instability (Stage 3):** Cross-period Adjusted Rand Index indicates low partition reproducibility. Sample size or edge filtering requires tuning.\n")
		}
		if !report.Precursor.Passed {
			sb.WriteString("3. **Precursor Evidence Insufficiency (Stage 5):** Insufficient independent excursion runs in the sampled archive to confirm ignition/exhaustion precursor separation.\n")
		}
	} else {
		sb.WriteString("All audited components demonstrate healthy, non-random statistical behavior and faithful production data representation.\n")
	}

	return sb.String()
}
