package cmd

import (
	"context"
	"slices"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
)

var (
	trainEpochs  []int64
	trainSymbols []string
	trainFeePct  string
)

var trainCmd = &cobra.Command{
	Use:   "train",
	Short: "Train action tree on detected excursions and continuous sensory tape",
	Long: `train discovers the optimal precursor window length using Monte Carlo Tree Search (MCTS)
over historical excursions and continuous sensory tape, constructing an unpruned learned
action tree published to S3 storage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		system.NewConfig()

		_, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: "symm.train.theapemachine.app",
			ServerAddress:   "http://localhost:4040",
			Logger:          nil,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[train] error starting pyroscope profiler",
				err,
			))
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tables.Open(ctx)

		if catalog == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[train] unable to open iceberg catalog",
				nil,
			))
		}

		if len(trainEpochs) == 0 {
			runs, err := catalog.Runs(ctx)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[train] failed to read recorded runs from catalog",
					err,
				))
			}

			for _, run := range runs {
				if run.Epoch > 0 && !slices.Contains(trainEpochs, run.Epoch) {
					trainEpochs = append(trainEpochs, run.Epoch)
				}
			}

			if len(trainEpochs) == 0 {
				return errnie.Error(errnie.Err(
					errnie.NotFound,
					"[train] no completed epochs found in catalog",
					nil,
				))
			}

			slices.Sort(trainEpochs)
		}

		if trainFeePct == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[train] --taker-fee-percent is required and must be specified",
				nil,
			))
		}

		price, err := offlinePrice(ctx, trainFeePct, trainSymbols)

		if err != nil {
			return err
		}

		trainer := strategy.NewTrainer(ctx, catalog, price)

		if len(trainSymbols) > 0 {
			trainer.SetSymbols(trainSymbols...)
		}

		return trainer.Generate(trainEpochs...)
	},
}

func init() {
	trainCmd.Flags().Int64SliceVar(&trainEpochs, "epoch", nil, "Run epoch to train on (repeatable; default: all completed epochs)")
	trainCmd.Flags().StringSliceVar(&trainSymbols, "symbols", nil, "Symbols to evaluate, e.g. BTC/USD,ETH/USD")
	trainCmd.Flags().StringVar(&trainFeePct, "taker-fee-percent", "0.8", "Taker fee in percent per side that defines friction")
	rootCmd.AddCommand(trainCmd)
}
