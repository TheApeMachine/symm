package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	ptypes "github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer"
)

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Run the optimizer against recorded measurements",
	Long:  tuneLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := tuneMeasurementPath()

		if err != nil {
			return err
		}

		maxMeasurements, err := tuneMaxMeasurements(cmd)

		if err != nil {
			return err
		}

		options := optimizer.DefaultTuneOptions(runtime.NumCPU())
		options.OutputPath = tunePerspectivesPath()
		options.CandidateReportPath = tuneCandidateReportPath(cmd)
		options.MaxMeasurements = maxMeasurements
		options.BeamWidth, _ = cmd.Flags().GetInt(tuneBeamWidthFlag)
		options.MaxRounds, _ = cmd.Flags().GetInt(tuneMaxRoundsFlag)
		options.MaxNodes, _ = cmd.Flags().GetInt(tuneMaxNodesFlag)

		if workers, _ := cmd.Flags().GetInt(tuneWorkersFlag); workers > 0 {
			options.Workers = workers
		}

		optimizer.TuneLog("loading measurements from %s", path)

		rows, skipped, err := loadTuneMeasurements(path, maxMeasurements)

		if err != nil {
			return err
		}

		if skipped > 0 {
			fmt.Fprintf(
				os.Stderr,
				"symm tune: skipped %d malformed measurement lines in %s\n",
				skipped,
				path,
			)
		}

		options.OnBest = func(best optimizer.BestTree) {
			fmt.Fprintf(
				os.Stderr,
				"symm tune: best strategies=%d nodes=%d trades=%d score=%.6f -> %s\n",
				len(best.Thoughts),
				best.Nodes,
				best.Trades,
				best.Score,
				options.OutputPath,
			)
		}
		options.OnCandidate = func(candidate optimizer.CandidateScore) {
			if candidate.ClosedTrades <= 0 {
				return
			}

			fmt.Fprintf(
				os.Stderr,
				"symm tune: candidate depth=%d strategies=%d trades=%d realized_eur=%.4f return_pct=%.4f avg_trade_eur=%.4f score=%.6f\n",
				candidate.ReasoningDepth(),
				candidate.RegistryWidth(),
				candidate.ClosedTrades,
				candidate.ProfitLoss(),
				candidate.ReturnPct(),
				candidate.AvgTradeEUR(),
				candidate.Score,
			)
		}

		summary, err := optimizer.TuneMeasurements(cmd.Context(), rows, options)

		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "symm tune: %s\n", summary)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuneCmd)

	tuneCmd.Flags().String(
		tuneCandidateReportFlag,
		"",
		"optional JSONL path for every scored candidate (advanced)",
	)
	tuneCmd.Flags().Int(
		tuneMaxMeasurementsFlag,
		0,
		"maximum valid measurement rows to load from the capture before tuning; 0 loads all rows",
	)
	tuneCmd.Flags().Int(
		tuneBeamWidthFlag,
		0,
		"beam width for the forest search; 0 uses the search default",
	)
	tuneCmd.Flags().Int(
		tuneMaxRoundsFlag,
		0,
		"maximum expansion rounds; 0 uses the search default",
	)
	tuneCmd.Flags().Int(
		tuneMaxNodesFlag,
		0,
		"maximum nodes per forest; 0 uses the search default",
	)
	tuneCmd.Flags().Int(
		tuneWorkersFlag,
		0,
		"parallel scoring workers; 0 uses all CPUs",
	)
}

const tuneLong = `
Run the optimizer against a recorded measurement capture.

Measurements are read from trading.record.file in the config. Realized round-trip
PnL is the only objective; structure and depth are discovered, not preset. An
improved tree is written to
market/perspectives/cfg/perspectives.yaml whenever a new best-scoring candidate
with closed round trips appears.
`

const defaultPerspectivesOutputPath = "market/perspectives/cfg/perspectives.yaml"
const tuneCandidateReportFlag = "candidate-report"
const tuneMaxMeasurementsFlag = "max-measurements"
const tuneBeamWidthFlag = "beam-width"
const tuneMaxRoundsFlag = "max-rounds"
const tuneMaxNodesFlag = "max-nodes"
const tuneWorkersFlag = "workers"

func tuneMeasurementPath() (string, error) {
	path := strings.TrimSpace(viper.GetString("trading.record.file"))

	if path != "" {
		return path, nil
	}

	path = strings.TrimSpace(viper.GetString("trading.replay.file"))

	if path != "" {
		return path, nil
	}

	// Fall back to the capture file `make run --record` writes, so the two
	// commands agree even with a bare config.
	return defaultCapturePath, nil
}

func loadTuneMeasurements(path string, maxMeasurements int) ([]ptypes.Measurement, int, error) {
	if maxMeasurements > 0 {
		return optimizer.LoadMeasurementsLimit(path, maxMeasurements)
	}

	return optimizer.LoadMeasurements(path)
}

func tunePerspectivesPath() string {
	path := strings.TrimSpace(os.Getenv("SYMM_PERSPECTIVES_FILE"))

	if path != "" {
		return path
	}

	return defaultPerspectivesOutputPath
}

func tuneMaxMeasurements(cmd *cobra.Command) (int, error) {
	maxMeasurements, err := cmd.Flags().GetInt(tuneMaxMeasurementsFlag)

	if err != nil {
		return 0, err
	}

	if maxMeasurements < 0 {
		return 0, fmt.Errorf("symm tune: --%s must be non-negative", tuneMaxMeasurementsFlag)
	}

	return maxMeasurements, nil
}

func tuneCandidateReportPath(cmd *cobra.Command) string {
	if cmd.Flags().Changed(tuneCandidateReportFlag) {
		path, _ := cmd.Flags().GetString(tuneCandidateReportFlag)

		return strings.TrimSpace(path)
	}

	path := strings.TrimSpace(viper.GetString("optimizer.tune.candidate_report"))

	if path != "" {
		return path
	}

	return ""
}
