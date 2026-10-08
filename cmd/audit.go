package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/audit"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/system"
)

var (
	auditEpoch        int64
	auditSymbol       string
	auditTicks        int
	auditPermutations int
	auditOutputDir    string
	auditNoPlots      bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run a read-only empirical audit over archived market and sensory observations",
	Long:  auditLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		system.NewConfig()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tables.Open(ctx)
		if catalog == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit] unable to open iceberg catalog",
				nil,
			))
		}

		opts := audit.AuditOptions{
			Epoch:        auditEpoch,
			Symbol:       auditSymbol,
			MaxTicks:     auditTicks,
			Permutations: auditPermutations,
			OutputDir:    auditOutputDir,
			NoPlots:      auditNoPlots,
		}

		report, err := audit.Run(ctx, catalog, opts)
		if err != nil {
			return errnie.Error(err)
		}

		absOut, _ := filepath.Abs(opts.OutputDir)
		fmt.Printf("%s", "\n" + stringsRepeat("=", 70) + "\n")
		fmt.Printf("🎯 SYMM PIPELINE EMPIRICAL AUDIT COMPLETED\n")
		fmt.Printf("%s", stringsRepeat("=", 70) + "\n\n")
		fmt.Printf("Target Run Epoch: %d\n", report.Epoch)
		fmt.Printf("Symbol Audited:   %s (%d ticks)\n", report.Symbol, report.TotalTicks)
		fmt.Printf("Audit State:      %s\n\n", formatAuditState(report))
		fmt.Printf("Stage 0 (Contract):   %s\n", report.Contract.SummaryText)
		fmt.Printf("Stage 1 (Vitality):   %s\n", report.Vitality.SummaryText)
		fmt.Printf("Stage 2 (Sympathy):   %s\n", report.Sympathy.SummaryText)
		fmt.Printf("Stage 3 (Stability):  %s\n", report.GridStability.SummaryText)
		fmt.Printf("Stage 4 (Dynamics):   %s\n", report.TokenDynamics.SummaryText)
		fmt.Printf("Stage 5 (Precursor):  %s\n", report.Precursor.SummaryText)
		fmt.Printf("Stage 6 (Cognitive):  %s\n\n", report.CognitiveTrie.SummaryText)
		fmt.Printf("📂 Full Results & Visualizations:\n")
		fmt.Printf("   Report: %s/AUDIT_SUMMARY.md\n", absOut)
		fmt.Printf("   JSON:   %s/audit_results.json\n", absOut)
		fmt.Printf("   Plots:  %s/plots/\n", absOut)
		fmt.Printf("%s", stringsRepeat("=", 70) + "\n\n")

		return nil
	},
}

func formatAuditState(report *audit.AuditReport) string {
	if report == nil {
		return "INVALID_EXPERIMENT"
	}
	if !report.Contract.Passed {
		return "CONTRACT_BREACH"
	}
	if report.Vitality.Status == "INSUFFICIENT_DATA" ||
		report.Sympathy.Status == "INSUFFICIENT_DATA" ||
		report.GridStability.Status == "INSUFFICIENT_DATA" ||
		report.TokenDynamics.Status == "INSUFFICIENT_DATA" ||
		report.Precursor.IgnitionHypothesis.Status == "INSUFFICIENT_DATA" ||
		report.Precursor.ExhaustionHypothesis.Status == "INSUFFICIENT_DATA" ||
		report.CognitiveTrie.Status == "INSUFFICIENT_DATA" {
		return "INCOMPLETE_EVIDENCE"
	}
	return "MEASURED"
}

func stringsRepeat(s string, count int) string {
	var result string
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

func init() {
	auditCmd.Flags().Int64Var(&auditEpoch, "epoch", 0, "Specific run epoch to audit (0 = latest run)")
	auditCmd.Flags().StringVar(&auditSymbol, "symbol", "", "Market symbol to audit (empty = all symbols across market tape)")
	auditCmd.Flags().IntVar(&auditTicks, "ticks", 1000, "Maximum number of ticks to sample (0 = all available ticks in epoch)")
	auditCmd.Flags().IntVar(&auditPermutations, "permutations", 50, "Number of permutation iterations for null hypothesis testing")
	auditCmd.Flags().StringVar(&auditOutputDir, "out", "audit_results", "Output directory for audit reports and plots")
	auditCmd.Flags().BoolVar(&auditNoPlots, "no-plots", false, "Skip generating Python/matplotlib visualization charts")

	rootCmd.AddCommand(auditCmd)
}

var auditLong = `
Run a read-only, component-by-component empirical audit of the SYMM sensory and representation pipeline.
Inspects six decoupled boundaries without model checkpointing or paper trading:
  0. Hard Metric Contract Integrity
  1. Metric Vitality & Redundancy (Variance, Coverage, Collinear Clones)
  2. Pair Relationships & Sympathy vs. Shuffled Null (Permutation Test)
  3. Grid Partitioning & Temporal Stability across Disjoint Time Periods (Adjusted Rand Index)
  4. Token Compression, Region Excitation & State Transitions (Dominance, Strength & Conditional Entropy)
  5. Precursor Separation (B/C Token Divergence vs. Background Noise)
  6. Cognitive Engine & Radix Trie Learning Dynamics (Prequential Recall, Label Null, Memory Retention)

Outputs a comprehensive machine-readable JSON report, an executive AUDIT_SUMMARY.md, and visual plots.
`
