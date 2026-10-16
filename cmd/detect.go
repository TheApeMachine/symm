package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
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

		price, err := offlinePrice(ctx, detectFeePct, detectSymbols)
		if err != nil {
			return err
		}

		detector := strategy.NewDetector(ctx, price)

		prefixes := make([]string, 0)
		if len(detectEpochs) > 0 && len(detectSymbols) > 0 {
			for _, epoch := range detectEpochs {
				for _, symbol := range detectSymbols {
					prefixes = append(prefixes, fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch))
				}
			}
		} else {
			prefixes = append(prefixes, "spot:trade/")
		}

		for value := range store.Values(client, prefixes...) {
			if value == nil {
				continue
			}
			detector.Step(value)
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
