package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

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

		if len(detectEpochs) == 0 {
			errnie.Info("[detect] discovering epochs from store under run/epoch/...")
			seen := make(map[int64]bool)

			for page := range client.List("run/epoch/", "/") {
				for _, commonPrefix := range page.CommonPrefixes {
					if commonPrefix.Prefix == nil {
						continue
					}

					prefixStr := strings.TrimPrefix(*commonPrefix.Prefix, "run/epoch/")
					prefixStr = strings.Trim(prefixStr, "/")
					epoch, err := strconv.ParseInt(prefixStr, 10, 64)

					if err != nil || epoch <= 0 {
						continue
					}

					if seen[epoch] {
						continue
					}

					seen[epoch] = true
					detectEpochs = append(detectEpochs, epoch)
				}

				for _, object := range page.Contents {
					if object.Key == nil {
						continue
					}

					parts := strings.Split(*object.Key, "/")

					if len(parts) < 3 {
						continue
					}

					epoch, err := strconv.ParseInt(parts[2], 10, 64)

					if err != nil || epoch <= 0 {
						continue
					}

					if seen[epoch] {
						continue
					}

					seen[epoch] = true
					detectEpochs = append(detectEpochs, epoch)
				}
			}

			if len(detectEpochs) == 0 {
				return errnie.Error(errnie.Err(
					errnie.NotFound,
					"[detect] no completed epochs found in store",
					nil,
				))
			}
		}

		errnie.Info(fmt.Sprintf(
			"[detect] targeting %d epoch(s): %v",
			len(detectEpochs), detectEpochs,
		))

		if detectFeePct == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[detect] --taker-fee-percent is required and must be specified",
				nil,
			))
		}

		price, err := offlinePrice(ctx, detectFeePct, detectSymbols)

		if err != nil {
			return err
		}

		storeTee := hindsight.NewStoreTee(ctx, "detector.store")
		storeTee.Transition(nmruntime.READY)
		storeTee.Run()

		detector := strategy.NewDetector(ctx, storeTee, price)

		if err := detector.Error(); err != nil {
			return err
		}

		for _, epoch := range detectEpochs {
			if epoch <= 0 {
				continue
			}

			prefixes := []string{"spot:trade/"}

			if len(detectSymbols) > 0 {
				prefixes = nil

				for _, symbol := range detectSymbols {
					prefixes = append(prefixes, fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch))
				}
			}

			errnie.Info(fmt.Sprintf(
				"[detect] scanning epoch %d across prefixes: %v",
				epoch, prefixes,
			))

			for _, prefix := range prefixes {
				tradesSeen := 0

				for measurement := range client.Get(prefix) {
					if measurement == nil || measurement.Epoch != epoch {
						continue
					}

					if len(detectSymbols) > 0 && !slices.Contains(detectSymbols, measurement.Label) {
						continue
					}

					tradesSeen++
					detector.Step(measurement)
				}

				if tradesSeen == 0 {
					errnie.Info(fmt.Sprintf(
						"[detect] no trades observed under prefix %s for epoch %d",
						prefix, epoch,
					))
				}

				if tradesSeen > 0 {
					errnie.Info(fmt.Sprintf(
						"[detect] scanned %d trades under prefix %s for epoch %d",
						tradesSeen, prefix, epoch,
					))
				}
			}
		}

		if err := detector.FlushAll(); err != nil {
			return err
		}

		pending := storeTee.Pending()

		if pending > 0 {
			errnie.Info(fmt.Sprintf(
				"[detect] waiting for %d pending excursions to persist...",
				pending,
			))
		}

		for storeTee.Pending() > 0 {
			time.Sleep(10 * time.Millisecond)
		}

		errnie.Info(fmt.Sprintf(
			"[detect] completed: %d total trades processed, %d total excursions emitted",
			detector.Steps(), detector.Emitted(),
		))

		return nil
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
