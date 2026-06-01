package cmd

import (
	"fmt"
	"os"
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

		outputPath := tunePerspectivesPath()
		summary, err := optimizer.TuneMeasurements(
			cmd.Context(),
			rows,
			optimizer.TuneOptions{
				OutputPath: outputPath,
				OnBest: func(best optimizer.BestTree) {
					fmt.Fprintf(
						os.Stderr,
						"symm tune: best iteration=%d branches=%d score=%.6f -> %s\n",
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
}

const tuneLong = `
Run the optimizer against a recorded measurement capture.

Use the default config (cmd/cfg/config.yml). Measurements are read directly from
trading.record.file and each improved tree is written to market/perspectives/cfg/perspectives.yaml.
`

const defaultPerspectivesOutputPath = "market/perspectives/cfg/perspectives.yaml"

func tuneMeasurementPath() (string, error) {
	path := strings.TrimSpace(viper.GetString("trading.record.file"))

	if path != "" {
		return path, nil
	}

	path = strings.TrimSpace(viper.GetString("trading.replay.file"))

	if path == "" {
		return "", fmt.Errorf("tune: trading.record.file is required")
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
