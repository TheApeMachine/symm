package audit

import (
	"fmt"
	"strings"
)

/*
GenerateSummaryMarkdown builds an executive Markdown document with plain-English
explanations, healthy/broken criteria, and embedded chart links.
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
	sb.WriteString("This audit tests whether market representation retains genuine statistical structure across five operational boundaries, comparing real measurements against deliberately scrambled empirical nulls.\n\n")

	sb.WriteString("| Stage | Question Tested | Status | Key Metric |\n")
	sb.WriteString("| :--- | :--- | :---: | :--- |\n")

	stage1Badge := "✅ PASS"
	if !report.Vitality.Passed {
		stage1Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **1. Metric Vitality** | Are sensors alive and non-redundant? | %s | %d/%d healthy, %d dead, %d redundant pairs |\n",
		stage1Badge, report.Vitality.HealthyMetrics, report.Vitality.TotalMetrics, report.Vitality.DeadMetrics, len(report.Vitality.RedundantPairs),
	))

	stage2Badge := "✅ PASS"
	if !report.Sympathy.Passed {
		stage2Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **2. Sympathy Null** | Do metrics move together beyond noise? | %s | %.1f%% exceed null envelope (KS: %.3f) |\n",
		stage2Badge, report.Sympathy.SeparationRatio*100, report.Sympathy.KSStatistic,
	))

	stage3Badge := "✅ PASS"
	if !report.GridStability.Passed {
		stage3Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **3. Grid Stability** | Does partitioning repeat across time? | %s | ARI: %.3f (Overlap: %.1f%%, Max Share: %.1f%%) |\n",
		stage3Badge, report.GridStability.AdjustedRandIdx, report.GridStability.OverlapFraction*100, report.GridStability.GridA.MaxRegionShare*100,
	))

	stage4Badge := "✅ PASS"
	if !report.TokenDynamics.Passed {
		stage4Badge = "❌ FAIL"
	}
	sb.WriteString(fmt.Sprintf("| **4. Token Dynamics** | Does region compression preserve structure? | %s | Max Dominance: %.1f%%, Entropy Reduction: %.3f bits |\n",
		stage4Badge, report.TokenDynamics.MaxTokenDominance*100, report.TokenDynamics.EntropyReductionBits,
	))

	stage5Badge := "✅ PASS"
	if !report.Precursor.Passed {
		stage5Badge = "❌ FAIL"
	}
	if report.Precursor.PrecursorTicks == 0 {
		stage5Badge = "ℹ️ INFO"
	}
	sb.WriteString(fmt.Sprintf("| **5. Precursor Separation** | Are patterns before B/C distinguishable? | %s | %s |\n\n",
		stage5Badge, report.Precursor.SummaryText,
	))

	sb.WriteString("---\n\n")

	// Stage 1
	sb.WriteString("### Stage 1: Metric Vitality & Redundancy\n\n")
	sb.WriteString("> **The Plain-English Question:** *Are the individual metrics actually moving and present, or are we feeding the grid dead constants and duplicated signals?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Healthy Metrics:** `%d` of `%d` (`%.1f%%`)\n",
		report.Vitality.HealthyMetrics, report.Vitality.TotalMetrics,
		float64(report.Vitality.HealthyMetrics)/float64(max(1, report.Vitality.TotalMetrics))*100,
	))
	sb.WriteString(fmt.Sprintf("- **Dead / Constant Metrics:** `%d` (metrics with zero variance)\n", report.Vitality.DeadMetrics))
	sb.WriteString(fmt.Sprintf("- **Sporadic Metrics:** `%d` (observed on < 20%% of ticks)\n", report.Vitality.SporadicMetrics))
	sb.WriteString(fmt.Sprintf("- **Redundant Duplicate Pairs (|r| >= 0.95):** `%d`\n\n", len(report.Vitality.RedundantPairs)))

	sb.WriteString("![Stage 1 Metric Vitality](plots/stage1_metric_vitality.png)\n\n")
	sb.WriteString("![Stage 1 Metric Redundancy](plots/stage1_metric_redundancy.png)\n\n")

	// Stage 2
	sb.WriteString("### Stage 2: Pair Relationships & Sympathy (Real vs. Shuffled Null)\n\n")
	sb.WriteString("> **The Plain-English Question:** *Do metrics exhibit genuine simultaneous sympathy, or could an identical grid be built from scrambled noise?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Pairs Analyzed:** `%d`\n", report.Sympathy.TotalPairs))
	sb.WriteString(fmt.Sprintf("- **Positive Relationships:** `%d` | **Inverse (Negative) Relationships:** `%d`\n", report.Sympathy.PositivePairs, report.Sympathy.InversePairs))
	sb.WriteString(fmt.Sprintf("- **Separation vs Shuffled Null:** `%.1f%%` of pairs exceed the 95th percentile null envelope.\n", report.Sympathy.SeparationRatio*100))
	sb.WriteString(fmt.Sprintf("- **Kolmogorov-Smirnov Distance:** `%.3f` (higher is better; > 0.20 indicates distinct non-random distribution)\n\n", report.Sympathy.KSStatistic))

	sb.WriteString("![Stage 2 Sympathy Null](plots/stage2_sympathy_null.png)\n\n")
	sb.WriteString("![Stage 2 Orientation Balance](plots/stage2_orientation_balance.png)\n\n")

	// Stage 3
	sb.WriteString("### Stage 3: Grid Partitioning & Temporal Stability\n\n")
	sb.WriteString("> **The Plain-English Question:** *Does the Impulse Map discover the same metric friendships across disjoint chronological periods, or does it redraw arbitrary clusters every time?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Early Period Grid:** `%d` regions across `%d` cells (Largest region holds `%.1f%%`)\n",
		report.GridStability.GridA.RegionCount, report.GridStability.GridA.CellCount, report.GridStability.GridA.MaxRegionShare*100,
	))
	sb.WriteString(fmt.Sprintf("- **Late Period Grid:** `%d` regions across `%d` cells (Largest region holds `%.1f%%`)\n",
		report.GridStability.GridB.RegionCount, report.GridStability.GridB.CellCount, report.GridStability.GridB.MaxRegionShare*100,
	))
	sb.WriteString(fmt.Sprintf("- **Universe Overlap:** `%.1f%%` (`%d` shared cells)\n",
		report.GridStability.OverlapFraction*100, report.GridStability.SharedUniverse,
	))
	sb.WriteString(fmt.Sprintf("- **Adjusted Rand Index (ARI):** `%.3f` (1.0 = identical partitions; 0.0 = random agreement; > 0.30 indicates stable clustering)\n\n",
		report.GridStability.AdjustedRandIdx,
	))

	sb.WriteString("![Stage 3 Region Partitioning](plots/stage3_region_partitioning.png)\n\n")

	// Stage 4
	sb.WriteString("### Stage 4: Token Dynamics & State Transitions\n\n")
	sb.WriteString("> **The Plain-English Question:** *When metrics light regions on a frozen grid, does the resulting token stream have structured dynamics or degenerate collapse?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Emissions:** `%d` | **Active Regions Emitted:** `%d`\n", report.TokenDynamics.TotalEmissions, report.TokenDynamics.UniqueTokens))
	sb.WriteString(fmt.Sprintf("- **Max Token Dominance:** `%.1f%%` (< 80%% is healthy; higher indicates stuck state collapse)\n", report.TokenDynamics.MaxTokenDominance*100))
	sb.WriteString(fmt.Sprintf("- **Transition Entropy:** `%.3f` bits (Null Shuffled Entropy: `%.3f` bits)\n", report.TokenDynamics.TransitionEntropy, report.TokenDynamics.NullTransitionEntropy))
	sb.WriteString(fmt.Sprintf("- **Entropy Reduction:** `%.3f` bits (higher is better; indicates organized, non-random transition dynamics)\n\n", report.TokenDynamics.EntropyReductionBits))

	sb.WriteString("![Stage 4 Token Dynamics](plots/stage4_token_dynamics.png)\n\n")
	sb.WriteString("![Stage 4 Transition Heatmap](plots/stage4_transition_matrix.png)\n\n")
	sb.WriteString("![Stage 4 Transition Entropy](plots/stage4_transition_entropy.png)\n\n")

	// Stage 5
	sb.WriteString("### Stage 5: Precursor Informativeness & Null Separation\n\n")
	sb.WriteString("> **The Plain-English Question:** *Are the token sequences or sensory states preceding B (ignition) statistically distinguishable from ambient market noise, or is the trie being asked to memorize noise?*\n\n")
	sb.WriteString(fmt.Sprintf("- **Detections Found:** `%d` (Classes: `%s`)\n", report.Precursor.DetectionsFound, strings.Join(report.Precursor.ExcursionsFound, ", ")))
	sb.WriteString(fmt.Sprintf("- **Precursor Window Ticks:** `%d`\n", report.Precursor.PrecursorTicks))
	sb.WriteString(fmt.Sprintf("- **Precursor Divergence (JSD):** `%.3f` bits\n", report.Precursor.PrecursorDivergence))
	sb.WriteString(fmt.Sprintf("- **Empirical Shuffled Null 95th Percentile:** `%.3f` bits (Mean: `%.3f` bits)\n", report.Precursor.NullDivergence95, report.Precursor.NullDivergenceMean))
	sb.WriteString(fmt.Sprintf("- **Separation Ratio:** `%.2fx` (> 1.0x indicates real precursor signal exceeding 95%% null envelope)\n\n", report.Precursor.SeparationRatio))

	sb.WriteString("![Stage 5 Precursor Separation](plots/stage5_precursor_separation.png)\n\n")

	return sb.String()
}
