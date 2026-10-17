package cmd

import (
	"context"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

var (
	trainEpochs  []int64
	trainSymbols []string
	trainFeePct  string
)

var trainCmd = &cobra.Command{
	Use:   "train",
	Short: "Train the action tree on detected excursions and the stored tick tape",
	Long: `train replays each epoch's stored ticks (data.TickPrefix) up to detect's
watermark (data.DetectorWatermark; an epoch without one is refused, and
ticks collect stored after detect ran are left out) through the signal
grid, the chord-level surprise gate (resonance.*) and grammar per symbol,
attributes the excursions detect wrote for the epoch to the grammar paths
that preceded them, and publishes the policies under a fresh policy epoch,
data.PolicyPrefixes(policy epoch), each tagged with the gate settings and
the tick epoch (data.PolicyGate). Earlier trainings are kept, so a gated and
an ungated training of the same ticks can be audited side by side
(symm audit --policy-epoch).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: "symm.train.theapemachine.app",
			ServerAddress:   "http://localhost:4040",
			Logger:          nil,
		}); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[train] error starting pyroscope profiler", err))
		}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		client := store.NewClient(ctx, "symm")

		if client == nil {
			return errnie.Error(errnie.Err(errnie.Validation, "[train] unable to open store client", nil))
		}

		epochs, err := storedEpochs(client, trainEpochs)

		if err != nil {
			return errnie.Error(err)
		}

		price, err := offlinePrice(ctx, trainFeePct, trainSymbols)

		if err != nil {
			return err
		}

		if err := strategy.NewTrainer(ctx, client, price, trainSymbols).Generate(epochs...); err != nil {
			return err
		}

		return errnie.Error(client.Drain())
	},
}

func init() {
	trainCmd.Flags().Int64SliceVar(&trainEpochs, "epoch", nil, "Epoch to train on (repeatable; default: every epoch with stored ticks)")
	trainCmd.Flags().StringSliceVar(&trainSymbols, "symbols", nil, "Symbols to train on, e.g. BTC/USD,ETH/USD (default: every symbol the ticks trade)")
	trainCmd.Flags().StringVar(&trainFeePct, "taker-fee-percent", "", "Taker fee in percent per side that defines friction, e.g. 0.8")

	if err := trainCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}

	rootCmd.AddCommand(trainCmd)
}
