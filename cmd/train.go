package cmd

import (
	"context"
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cli/progress"
	"github.com/theapemachine/symm/kraken"
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
	Short: "Train the action tree and publish its prefixes",
	Long: `train reads each stored tick and the excursions detect wrote for it.
Sequitur turns the lit region tokens into a path. Each path that agrees on
one action is written under policy/current/<path>/enter.bin, exit.bin, or
noop.bin, which is the key training matches later. The previous current tree
is moved to policy/archive/ first.`,
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
				"[train] unable to open store client",
				nil,
			))
		}

		defer client.Close()

		feePct, err := strconv.ParseFloat(trainFeePct, 64)

		if err != nil || feePct <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[train] taker-fee-percent must be a positive percent per side",
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

		epochs, err := trainRunEpochs(client)

		if err != nil {
			return errnie.Error(err)
		}

		if len(client.ListKeys("policy/current/")) > 0 {
			if err := client.MovePrefix("policy/current/", "policy/archive/"); err != nil {
				return errnie.Error(err)
			}
		}

		trainer := strategy.NewTrainer(ctx, client, price, trainSymbols...)

		workTotal := trainer.WorkTotal(epochs...)

		if workTotal < 1 {
			return errnie.Err(errnie.NotFound, "[train] no stored detections", nil)
		}

		bar := progress.New("train excursions", workTotal)
		defer bar.Finish()
		trainer.OnFrame = bar.Step
		trainer.SetWorkTotal = bar.SetTotal

		return trainer.Generate(epochs...)
	},
}

func trainRunEpochs(client *store.Client) ([]int64, error) {
	if len(trainEpochs) == 0 {
		return nil, nil
	}

	epochs := make([]int64, 0, len(trainEpochs))

	for _, epoch := range trainEpochs {
		if epoch <= 0 {
			return nil, errnie.Err(errnie.Validation, "[train] epoch must be positive", nil)
		}

		epochs = append(epochs, epoch)
	}

	return epochs, nil
}

func init() {
	trainCmd.Flags().Int64SliceVar(&trainEpochs, "epoch", nil, "Epoch to train on (repeatable; default: every stored tick epoch)")
	trainCmd.Flags().StringSliceVar(&trainSymbols, "symbols", nil, "Symbols to train on, e.g. BTC/USD,ETH/USD (default: every symbol the excursions name)")
	trainCmd.Flags().StringVar(&trainFeePct, "taker-fee-percent", "", "Taker fee in percent per side when an excursion cannot be priced from the book, e.g. 0.8")

	if err := trainCmd.MarkFlagRequired("taker-fee-percent"); err != nil {
		errnie.Error(err)
	}

	rootCmd.AddCommand(trainCmd)
}
