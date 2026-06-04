package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

		options := optimizer.DefaultTuneOptions(runtime.NumCPU())
		options.OutputPath = tunePerspectivesPath()
		options.CandidateReportPath = tuneCandidateReportPath(cmd)

		total, countSkipped, err := optimizer.CountMeasurementLines(path)

		if err != nil {
			return err
		}

		sampleCap := optimizer.DeriveMeasurementSampleCap(total, options.Workers)

		optimizer.TuneLog("loading measurements from %s", path)

		rows, skipped, err := optimizer.LoadMeasurements(path, sampleCap)

		if err != nil {
			return err
		}

		if skipped > 0 || countSkipped > 0 {
			fmt.Fprintf(
				os.Stderr,
				"symm tune: skipped %d malformed measurement lines in %s\n",
				skipped+countSkipped,
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
				"symm tune: candidate depth=%d strategies=%d trades=%d profit_loss=%.6f return_per_trade=%.4f%%\n",
				candidate.ReasoningDepth(),
				candidate.RegistryWidth(),
				candidate.ClosedTrades,
				candidate.ProfitLoss(),
				candidate.ReturnPct(),
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

func tunePerspectivesPath() string {
	path := strings.TrimSpace(os.Getenv("SYMM_PERSPECTIVES_FILE"))

	if path != "" {
		return path
	}

	return defaultPerspectivesOutputPath
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
