package cmd

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
)

var (
	detectEpochs  []int64
	detectSymbols []string
	detectFeePct  string
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Label stored spot:trade tape with excursion detections (source=detector)",
	Long: `detect runs the excursion Detector over the stored spot:trade tape of each
requested epoch and symbol, under an explicit taker fee (percent per side), and
appends the resulting source=detector rows to S3 storage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		system.NewConfig()

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		client := store.NewClient(ctx, "symm")

		if client == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[detect] unable to open store client",
				nil,
			))
		}

		storeTee := hindsight.NewStoreTee(ctx, "detector.store")
		storeTee.Transition(nmruntime.READY)
		storeTee.Run()

		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		private := network.NewReplayer(ctx)
		public := network.NewReplayer(ctx)

		instrument := broker.NewInstrument(ctx, public)
		price := broker.NewPrice(ctx, books, private, instrument, normalizer)

		detector := strategy.NewDetector(ctx, storeTee, price)

		done := false

		for !done {
			trade, err := public.Read("spot:trade")

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[detect] error reading spot:trade",
					err,
				))
			}

			if trade == nil {
				done = true
				continue
			}

			var measurement *data.Measurement
			err = measurement.UnmarshalBinary(trade)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[detect] error unmarshalling trade",
					err,
				))
			}

			detector.Step(measurement)
		}

		return errnie.Error(detector.Error())
	},
}

func init() {
	detectCmd.Flags().Int64SliceVar(&detectEpochs, "epoch", nil, "Run epoch to label (repeatable; default: all completed epochs)")
	detectCmd.Flags().StringSliceVar(&detectSymbols, "symbols", nil, "Symbols to label, e.g. BTC/USD,ETH/USD (default: all symbols observed in epochs)")
	detectCmd.Flags().StringVar(&detectFeePct, "taker-fee-percent", "", "Taker fee in percent per side that defines friction, e.g. 0.8")

	if err := detectCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}

	rootCmd.AddCommand(detectCmd)
}
