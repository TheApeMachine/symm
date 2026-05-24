package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theapemachine/symm/eval"
)

var (
	evalReplayFile string
	evalFormat     string
	evalOutput     string
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run offline replay calibration",
	Long:  "Replay captured Kraken websocket frames and emit signal calibration metrics.",
	RunE: func(cmd *cobra.Command, args []string) error {
		replayFile := strings.TrimSpace(evalReplayFile)

		if replayFile == "" {
			replayFile = strings.TrimSpace(os.Getenv("SYMM_REPLAY_FILE"))
		}

		if replayFile == "" {
			return fmt.Errorf("replay file is required (--file or SYMM_REPLAY_FILE)")
		}

		report, err := eval.Run(cmd.Context(), eval.Options{
			ReplayFile: replayFile,
		})
		if err != nil {
			return err
		}

		writer := os.Stdout

		if path := strings.TrimSpace(evalOutput); path != "" {
			file, fileErr := os.Create(path)
			if fileErr != nil {
				return fmt.Errorf("create output file: %w", fileErr)
			}

			defer file.Close()
			writer = file
		}

		switch strings.ToLower(strings.TrimSpace(evalFormat)) {
		case "", "json":
			return eval.WriteJSON(writer, report)
		case "csv":
			return eval.WriteCSV(writer, report)
		default:
			return fmt.Errorf("unsupported format %q (use json or csv)", evalFormat)
		}
	},
}

func init() {
	evalCmd.Flags().StringVar(&evalReplayFile, "file", "", "JSONL replay file path")
	evalCmd.Flags().StringVar(&evalFormat, "format", "json", "Output format: json or csv")
	evalCmd.Flags().StringVarP(&evalOutput, "output", "o", "", "Write report to file instead of stdout")
	rootCmd.AddCommand(evalCmd)
}
