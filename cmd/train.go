package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
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

		client := store.NewClient(ctx, "symm")

		if client == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[train] unable to open store client",
				nil,
			))
		}

		if len(trainEpochs) == 0 {
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
					trainEpochs = append(trainEpochs, epoch)
				}
			}

			if len(trainEpochs) == 0 {
				return errnie.Error(errnie.Err(
					errnie.NotFound,
					"[train] no completed epochs found in store",
					nil,
				))
			}
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

		storeTee := hindsight.NewStoreTee(ctx, "train")
		storeTee.Transition(nmruntime.READY)
		storeTee.Run()

		// Archive existing active model in policy/current/ before refining/updating
		currentKeys := client.ListKeys("policy/current/")
		if len(currentKeys) > 0 {
			if err := client.MovePrefix("policy/current/", "policy/archive/"); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"[train] warning: failed to archive current policy before training",
					err,
				))
			}
		}

		// Outputs of the epochs about to be regenerated are removed first; a
		// failed delete would mix stale and fresh policy, so it stops training.
		stale := make([]string, 0)
		symbols := slices.Clone(trainSymbols)

		if len(symbols) == 0 {
			for page := range client.List("policy/", "/") {
				for _, cp := range page.CommonPrefixes {
					if cp.Prefix == nil {
						continue
					}

					symbols = append(symbols, strings.Trim(strings.TrimPrefix(*cp.Prefix, "policy/"), "/"))
				}
			}
		}

		for _, epoch := range trainEpochs {
			stale = append(stale,
				fmt.Sprintf("policy/current/%019d/", epoch),
				fmt.Sprintf("policy/current/%d/", epoch),
			)

			for _, symbol := range symbols {
				stale = append(stale,
					fmt.Sprintf("policy/%s/%019d/", symbol, epoch),
					fmt.Sprintf("policy/%s/%d/", symbol, epoch),
				)
			}
		}

		for _, prefix := range stale {
			if err := client.DeletePrefix(prefix); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[train] cannot clear "+prefix+" before regenerating it", err,
				))
			}
		}

		trainer := strategy.NewTrainer(ctx, client, storeTee, price)

		if err := trainer.Generate(trainEpochs...); err != nil {
			return err
		}

		for storeTee.Pending() > 0 {
			time.Sleep(10 * time.Millisecond)
		}

		return nil
	},
}

func init() {
	trainCmd.Flags().Int64SliceVar(&trainEpochs, "epoch", nil, "Run epoch to train on (repeatable; default: all completed epochs)")
	trainCmd.Flags().StringSliceVar(&trainSymbols, "symbols", nil, "Symbols to evaluate, e.g. BTC/USD,ETH/USD")
	trainCmd.Flags().StringVar(&trainFeePct, "taker-fee-percent", "0.8", "Taker fee in percent per side that defines friction")
	rootCmd.AddCommand(trainCmd)
}
