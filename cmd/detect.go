package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/cli/progress"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

var (
	detectEpochs  []int64
	detectSymbols []string
	detectFeePct  string
	detectTrace   string
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Label stored ticks with excursion detections",
	Long: `detect reads each stored run once, stitches a continuous tape per symbol,
and writes that symbol's detections under draft/. A later commit's learned
pullback is applied when it is stored.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		bucket := system.Cfg.Storage.S3.Bucket
		client := store.NewClient(ctx, bucket)

		if client == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[detect] unable to open store client",
				nil,
			))
		}

		defer client.Close()

		epochs, err := detectRunEpochs(client)

		if err != nil {
			return errnie.Error(err)
		}

		want := map[string]struct{}{}

		for _, symbol := range detectSymbols {
			want[symbol] = struct{}{}
		}

		retrace := hindsight.LearnedRetrace(client)

		for _, epoch := range epochs {
			listing := progress.Spin("listing ticks")
			prefix := fmt.Sprintf("workspace/tick/aggregate/%019d", epoch)
			var keys []string

			for key := range client.ListKeys(prefix) {
				keys = append(keys, key)
			}

			listing.Finish()

			if hindsight.ScanUpToDate(client, epoch, retrace, keys) {
				bar := progress.New("detect ticks", int64(len(keys)))
				bar.Finish()
				continue
			}

			bar := progress.New("detect ticks", int64(len(keys)))
			loaded, err := hindsight.Assemble(ctx, client, keys, bar.Step)
			bar.Finish()

			if err != nil {
				return errnie.Error(err)
			}

			if err := hindsight.WriteRunManifest(client, epoch, keys); err != nil {
				return errnie.Error(err)
			}

			drafts := hindsight.ScanRun(loaded, retrace, want)

			if err := hindsight.StoreDrafts(client, epoch, drafts); err != nil {
				return errnie.Error(err)
			}

			if err := hindsight.WriteScanStamp(client, epoch, retrace, len(drafts)); err != nil {
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
				return nil, errnie.Error(errnie.Err(
					errnie.Validation, "[detect] epoch must be positive", nil,
				))
			}

			epochs = append(epochs, uint64(epoch))
		}

		return epochs, nil
	}

	return listedEpochs(client)
}

func listedEpochs(client *store.Client) ([]uint64, error) {
	seen := map[uint64]struct{}{}
	epochs := make([]uint64, 0)

	for key := range client.ListKeys("workspace/tick/aggregate") {
		for _, segment := range strings.Split(key, "/") {
			if len(segment) != 19 {
				continue
			}

			epoch, err := strconv.ParseUint(segment, 10, 64)

			if err != nil {
				continue
			}

			if _, ok := seen[epoch]; ok {
				continue
			}

			seen[epoch] = struct{}{}
			epochs = append(epochs, epoch)
		}
	}

	if len(epochs) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[detect] no stored ticks under workspace/tick/aggregate", nil,
		))
	}

	slices.Sort(epochs)

	return epochs, nil
}

func init() {
	detectCmd.Flags().Int64SliceVar(&detectEpochs, "epoch", nil, "Run epoch to label (repeatable; default: every stored epoch)")
	detectCmd.Flags().StringSliceVar(&detectSymbols, "symbols", nil, "Symbols to label, e.g. BTC/USD,ETH/USD (default: every symbol on the tick)")
	detectCmd.Flags().StringVar(&detectFeePct, "taker-fee-percent", "", "Taker fee in percent per side when the tick has no fee, e.g. 0.8")
	detectCmd.Flags().StringVar(&detectTrace, "trace", "detector_trace", "Directory for the whole-tape decision video detect writes when it labels")

	if err := detectCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}

	rootCmd.AddCommand(detectCmd)
}
