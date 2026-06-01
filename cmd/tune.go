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

		rows, err := optimizer.LoadMeasurements(path)

		if err != nil {
			return err
		}

		if len(rows) == 0 {
			return fmt.Errorf("tune: no measurements in %s", path)
		}

		scanOptions, err := tuneScanOptions(cmd)

		if err != nil {
			return err
		}

		outputPath := tunePerspectivesPath()
		summary, err := optimizer.TuneMeasurements(
			cmd.Context(),
			rows,
			optimizer.TuneOptions{
				OutputPath:          outputPath,
				CandidateReportPath: tuneCandidateReportPath(cmd),
				Workers:             scanOptions.Workers,
				MaxThresholds:       scanOptions.MaxThresholds,
				BeamWidth:           scanOptions.BeamWidth,
				CandidateLimit:      scanOptions.CandidateLimit,
				OnBest: func(best optimizer.BestTree) {
					fmt.Fprintf(
						os.Stderr,
						"symm tune: best candidate=%d branches=%d score=%.6f -> %s\n",
						best.Iteration,
						len(best.Branches),
						best.Score,
						outputPath,
					)
				},
			},
		)

		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "symm tune: %s\n", summary)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuneCmd)

	tuneCmd.Flags().Int(
		tuneWorkersFlag,
		runtime.NumCPU(),
		"parallel scan workers",
	)
	tuneCmd.Flags().Int(
		tuneMaxThresholdsFlag,
		optimizer.DefaultScanMaxThresholds,
		"max threshold values per category/unit; 0 scans all unique values",
	)
	tuneCmd.Flags().Int(
		tuneBeamWidthFlag,
		optimizer.DefaultScanBeamWidth,
		"top entry/exit candidates to combine as sibling branches",
	)
	tuneCmd.Flags().Int(
		tuneCandidateLimitFlag,
		optimizer.DefaultScanCandidateLimit,
		"max candidate trees to score; 0 scans the generated space",
	)
	tuneCmd.Flags().String(
		tuneCandidateReportFlag,
		"",
		"JSONL path for every scored candidate's profit/loss",
	)
}

const tuneLong = `
Run the optimizer against a recorded measurement capture.

Use the default config (cmd/cfg/config.yml). Measurements are read directly from
trading.record.file and each improved tree is written to market/perspectives/cfg/perspectives.yaml.
`

const defaultPerspectivesOutputPath = "market/perspectives/cfg/perspectives.yaml"
const tuneWorkersFlag = "workers"
const tuneMaxThresholdsFlag = "max-thresholds"
const tuneBeamWidthFlag = "beam-width"
const tuneCandidateLimitFlag = "candidate-limit"
const tuneCandidateReportFlag = "candidate-report"

func tuneMeasurementPath() (string, error) {
	path := strings.TrimSpace(viper.GetString("trading.record.file"))

	if path != "" {
		return path, nil
	}

	path = strings.TrimSpace(viper.GetString("trading.replay.file"))

	if path == "" {
		return "", fmt.Errorf("tune: trading.record.file or trading.replay.file is required")
	}

	return path, nil
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

func tuneScanOptions(cmd *cobra.Command) (optimizer.ScanOptions, error) {
	workers, err := cmd.Flags().GetInt(tuneWorkersFlag)

	if err != nil {
		return optimizer.ScanOptions{}, err
	}

	maxThresholds, err := cmd.Flags().GetInt(tuneMaxThresholdsFlag)

	if err != nil {
		return optimizer.ScanOptions{}, err
	}

	beamWidth, err := cmd.Flags().GetInt(tuneBeamWidthFlag)

	if err != nil {
		return optimizer.ScanOptions{}, err
	}

	candidateLimit, err := cmd.Flags().GetInt(tuneCandidateLimitFlag)

	if err != nil {
		return optimizer.ScanOptions{}, err
	}

	return validTuneScanOptions(optimizer.ScanOptions{
		Workers:        workers,
		MaxThresholds:  maxThresholds,
		BeamWidth:      beamWidth,
		CandidateLimit: candidateLimit,
	})
}

func validTuneScanOptions(
	options optimizer.ScanOptions,
) (optimizer.ScanOptions, error) {
	if options.Workers <= 0 {
		return optimizer.ScanOptions{}, fmt.Errorf("tune: workers must be > 0")
	}

	if options.MaxThresholds < 0 {
		return optimizer.ScanOptions{}, fmt.Errorf("tune: max-thresholds must be >= 0")
	}

	if options.BeamWidth <= 0 {
		return optimizer.ScanOptions{}, fmt.Errorf("tune: beam-width must be > 0")
	}

	if options.CandidateLimit < 0 {
		return optimizer.ScanOptions{}, fmt.Errorf("tune: candidate-limit must be >= 0")
	}

	return options, nil
}
