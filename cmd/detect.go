package cmd

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

var (
	detectEpochs  []int64
	detectSymbols []string
	detectFeePct  string
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Label the stored tick tape with excursion detections (origin=detector)",
	Long: `detect runs one excursion Detector per symbol over the stored ticks of each
requested epoch (data.TickPrefix), under an explicit taker fee (percent per
side), and writes every excursion to the store under
data.ExcursionPrefixes(symbol, epoch). An epoch's earlier detections
are removed before it is relabelled.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		client := store.NewClient(ctx, "symm")

		if client == nil {
			return errnie.Error(errnie.Err(errnie.Validation, "[detect] unable to open store client", nil))
		}

		epochs, err := storedEpochs(client, detectEpochs)

		if err != nil {
			return errnie.Error(err)
		}

		price, err := offlinePrice(ctx, detectFeePct, detectSymbols)

		if err != nil {
			return err
		}

		for _, epoch := range epochs {
			if err := detectEpoch(ctx, client, price, epoch); err != nil {
				return errnie.Error(err)
			}
		}

		return errnie.Error(client.Drain())
	},
}

/*
detectEpoch streams epoch's ticks in order through a Detector per symbol.
*/
func detectEpoch(ctx context.Context, client *store.Client, price *broker.Price, epoch uint64) error {
	detectors := make(map[string]*strategy.Detector)
	order := make([]string, 0)
	written := make(map[string]bool)
	counts := make(map[string]int)
	ticks := 0

	write := func(results []*data.Value) error {
		for _, result := range results {
			// Two excursions on one key would overwrite each other.
			if written[result.Prefix] {
				return errnie.Err(errnie.Conflict, "[detect] two excursions share key "+result.Prefix, nil)
			}

			written[result.Prefix] = true
			counts[result.Role]++

			if _, err := io.Copy(client, result); err != nil {
				return errnie.Err(errnie.IO, "[detect] writing "+result.Prefix, err)
			}
		}

		return nil
	}

	for value := range store.Values(client, data.TickPrefix(epoch)) {
		if err := ctx.Err(); err != nil {
			return err
		}

		symbolTicks, err := broker.ReadTick(value)

		if err != nil {
			return err
		}

		ticks++

		for _, symbolTick := range symbolTicks {
			if len(detectSymbols) > 0 && !slices.Contains(detectSymbols, symbolTick.Symbol) {
				continue
			}

			detector, ok := detectors[symbolTick.Symbol]

			if !ok {
				if detector, err = strategy.NewDetector(ctx, price, epoch, symbolTick.Symbol); err != nil {
					return err
				}

				for _, prefix := range data.ExcursionPrefixes(symbolTick.Symbol, epoch) {
					if err := client.DeletePrefix(prefix); err != nil {
						return errnie.Err(errnie.IO, "[detect] cannot clear "+prefix+" before relabelling it", err)
					}
				}

				detectors[symbolTick.Symbol] = detector
				order = append(order, symbolTick.Symbol)
			}

			for _, trade := range symbolTick.Trades {
				results, err := detector.Step(value.Tick, trade, symbolTick.Price)

				if err != nil {
					return err
				}

				if err := write(results); err != nil {
					return err
				}
			}
		}
	}

	if ticks == 0 {
		return errnie.Err(errnie.NotFound, "[detect] no stored ticks under "+data.TickPrefix(epoch), nil)
	}

	for _, symbol := range order {
		results, err := detectors[symbol].Close()

		if err != nil {
			return err
		}

		if err := write(results); err != nil {
			return err
		}
	}

	errnie.Info(fmt.Sprintf(
		"[detect] epoch %d: %d ticks, %d symbols, excursions up=%d down=%d chop=%d flat=%d",
		epoch, ticks, len(order), counts["up"], counts["down"], counts["chop"], counts["flat"],
	))

	return nil
}

func init() {
	detectCmd.Flags().Int64SliceVar(&detectEpochs, "epoch", nil, "Epoch to label (repeatable; default: every epoch with stored ticks)")
	detectCmd.Flags().StringSliceVar(&detectSymbols, "symbols", nil, "Symbols to label, e.g. BTC/USD,ETH/USD (default: every symbol the ticks trade)")
	detectCmd.Flags().StringVar(&detectFeePct, "taker-fee-percent", "", "Taker fee in percent per side that defines friction, e.g. 0.8")

	if err := detectCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}

	rootCmd.AddCommand(detectCmd)
}
