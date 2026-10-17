package cmd

import (
	"context"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	auditpkg "github.com/theapemachine/symm/hindsight/audit"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

var (
	auditEpoch        int64
	auditPolicyEpoch  int64
	auditSymbol       string
	auditTicks        int
	auditPermutations int
	auditTakerFee     float64
	auditOutputDir    string
	auditNoPlots      bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run a read-only empirical audit over archived market and sensory observations",
	Long:  auditLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: "symm.audit.theapemachine.app",
			ServerAddress:   "http://localhost:4040",
			Logger:          nil,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[audit] error starting pyroscope profiler",
				err,
			))
		}

		// Flags override config only where given; the taker fee is required.
		audit := system.Cfg.Audit
		flags := cmd.Flags()

		if flags.Changed("epoch") {
			audit.Epoch = auditEpoch
		}

		if flags.Changed("policy-epoch") {
			audit.PolicyEpoch = auditPolicyEpoch
		}

		if flags.Changed("symbol") {
			audit.Symbol = auditSymbol
		}

		if flags.Changed("ticks") {
			audit.Ticks = auditTicks
		}

		if flags.Changed("permutations") {
			audit.Permutations = auditPermutations
		}

		if flags.Changed("out") {
			audit.OutputDir = auditOutputDir
		}

		if flags.Changed("no-plots") {
			audit.NoPlots = auditNoPlots
		}

		audit.TakerFee = auditTakerFee / 100

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		client := store.NewClient(ctx, "symm")

		if client == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit] unable to open store client",
				nil,
			))
		}

		engine, err := auditpkg.NewEngine(ctx, client)

		if err != nil {
			return errnie.Error(err)
		}

		return errnie.Error(engine.Run())
	},
}

func init() {
	auditCmd.Flags().Int64Var(&auditEpoch, "epoch", 0, "Epoch to audit (0 = config audit.epoch, else the newest epoch with stored ticks)")
	auditCmd.Flags().Int64Var(&auditPolicyEpoch, "policy-epoch", 0, "Policy epoch to judge, trained on the audited ticks (0 = the newest one; run twice to compare a gated and an ungated training)")
	auditCmd.Flags().StringVar(&auditSymbol, "symbol", "", "Symbol to audit (empty = every symbol the ticks trade)")
	auditCmd.Flags().IntVar(&auditTicks, "ticks", 1000, "Maximum number of ticks to sample (0 = all available ticks in epoch)")
	auditCmd.Flags().IntVar(&auditPermutations, "permutations", 50, "Number of permutation iterations for null hypothesis testing")
	auditCmd.Flags().Float64Var(&auditTakerFee, "taker-fee-percent", 0, "Taker fee percent per side the stored detections were made at (as given to detect --taker-fee-percent); required")

	if err := auditCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}
	auditCmd.Flags().StringVar(&auditOutputDir, "out", "audit_results", "Output directory for audit reports and plots")
	auditCmd.Flags().BoolVar(&auditNoPlots, "no-plots", false, "Skip generating Python/matplotlib visualization charts")

	rootCmd.AddCommand(auditCmd)
}

var auditLong = `
Run a read-only, component-by-component empirical audit of the SYMM sensory and representation pipeline.
Inspects ten decoupled boundaries without model checkpointing or paper trading:
  0. Hard Metric Contract Integrity (Audit:Contract)
  0.5. Ingestion Clock Timing & Synchronization (Latency, Spikes, Inversions)
  1. Metric Vitality & Redundancy (Variance, Coverage, Collinear Clones)
  2. Pair Relationships & Sympathy vs. Shuffled Null (Continuous-Time Permutation Test)
  3. Grid Region Stationarity across Disjoint Time Periods (JSD vs. Tick-Shuffled Null)
  4. Token Compression, Region Excitation & State Transitions (Dominance, Strength & Conditional Entropy)
  5. Precursor Friction Clearance Economics (Touch Execution: Entry Ask vs. Exit Bid vs. Taker Fee)
  6. Cognitive Decisiveness & Action Policy Dynamics (Abstention Rate, Active Paths, Decision Confidence)
  V1. Truthfulness vs Stored Trades & Volume Conservation (Order Book Non-Crossing & CVD Identities)

Outputs a comprehensive machine-readable JSON report, an executive AUDIT_SUMMARY.md, and visual plots.
`
