package cmd

import (
	"context"
	"io"
	"slices"
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cli/progress"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
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
	Short: "Label stored ticks with excursion detections",
	Long: `detect reads each stored tick, rebuilds that tick's book and fee from its
layers, and runs the excursion detector against that book. The taker-fee flag
is the friction rate when a tick's price layer has no fee. Excursions are
written back with io.Copy.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		system.NewConfig()

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		bucket := "symm"

		if system.Cfg != nil && system.Cfg.Storage != nil && system.Cfg.Storage.S3 != nil && system.Cfg.Storage.S3.Bucket != "" {
			bucket = system.Cfg.Storage.S3.Bucket
		}

		client := store.NewClient(ctx, bucket)

		if client == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[detect] unable to open store client",
				nil,
			))
		}

		defer client.Close()

		feePct, err := strconv.ParseFloat(detectFeePct, 64)

		if err != nil || feePct <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[detect] taker-fee-percent must be a positive percent per side",
				err,
			))
		}

		price := broker.NewPrice(ctx, nil, nil, nil, nil)
		cash := 200.0

		if system.Cfg != nil && system.Cfg.Market != nil && system.Cfg.Market.Balance > 0 {
			cash = float64(system.Cfg.Market.Balance)
		}

		price.SetReferenceCash(decimal.NewFromFloat64(cash))
		price.SetDefaultFee(kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(feePct)})

		detector := strategy.NewDetector(ctx, price)

		if err := detector.Error(); err != nil {
			return errnie.Error(err)
		}

		epochs, err := detectRunEpochs(client)

		if err != nil {
			return errnie.Error(err)
		}

		want := map[string]struct{}{}

		for _, symbol := range detectSymbols {
			want[symbol] = struct{}{}
		}

		var tickTotal int64

		for _, epoch := range epochs {
			tickTotal += int64(len(client.ListKeys(data.TickPrefix(epoch))))
		}

		bar := progress.New("detect ticks", tickTotal)
		defer bar.Finish()

		for _, epoch := range epochs {
			for frame, err := range store.Values(client, data.TickPrefix(epoch)) {
				if err != nil {
					return errnie.Error(err)
				}

				bar.Step()

				ticks, err := broker.ReadTick(frame)

				if err != nil {
					errnie.Error(err)
					continue
				}

				for _, tick := range ticks {
					if len(want) > 0 {
						if _, ok := want[tick.Symbol]; !ok {
							continue
						}
					}

					for _, value := range detector.Replay(frame, tick) {
						if _, err := io.Copy(client, value); err != nil {
							return errnie.Error(err)
						}
					}
				}
			}
		}

		for _, value := range detector.Finish() {
			if _, err := io.Copy(client, value); err != nil {
				return errnie.Error(err)
			}
		}

		return nil
	},
}

func detectRunEpochs(client *store.Client) ([]uint64, error) {
	if len(detectEpochs) > 0 {
		epochs := make([]uint64, 0, len(detectEpochs))

		for _, epoch := range detectEpochs {
			if epoch <= 0 {
				return nil, errnie.Err(errnie.Validation, "[detect] epoch must be positive", nil)
			}

			epochs = append(epochs, uint64(epoch))
		}

		return epochs, nil
	}

	var epochs []uint64

	for page := range client.List(data.TickPrefix(), "/") {
		if page == nil {
			continue
		}

		for _, prefix := range page.CommonPrefixes {
			if prefix.Prefix == nil {
				continue
			}

			epoch, err := data.KeyEpoch(data.TickPrefix(), *prefix.Prefix)

			if err != nil {
				continue
			}

			epochs = append(epochs, epoch)
		}
	}

	slices.Sort(epochs)

	return epochs, nil
}

func init() {
	detectCmd.Flags().Int64SliceVar(&detectEpochs, "epoch", nil, "Run epoch to label (repeatable; default: every stored epoch)")
	detectCmd.Flags().StringSliceVar(&detectSymbols, "symbols", nil, "Symbols to label, e.g. BTC/USD,ETH/USD (default: every symbol on the tick)")
	detectCmd.Flags().StringVar(&detectFeePct, "taker-fee-percent", "", "Taker fee in percent per side when the tick has no fee, e.g. 0.8")

	if err := detectCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}

	rootCmd.AddCommand(detectCmd)
}
