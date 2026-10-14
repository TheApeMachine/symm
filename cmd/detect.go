package cmd

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/replay"
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
		socket := network.NewReplayer(cmd.Context())
		public := network.NewReplayer(cmd.Context())
		private := network.NewReplayer(cmd.Context())
		normalizer := spot.NewNormalizer()

		if err := broker.SeedNormalizer(normalizer); err != nil {
			return err
		}

		storeTee := hindsight.NewStoreTee(
			cmd.Context(), "detector.store",
		)
		storeTee.Transition(nmruntime.READY)
		storeTee.Run()

		price := broker.NewPrice(
			cmd.Context(),
			broker.NewBook(cmd.Context(), normalizer),
			private,
			broker.NewInstrument(cmd.Context(), public),
			normalizer,
		)

		if detectFeePct != "" {
			fee, err := decimal.NewFromString(detectFeePct)
			if err != nil || fee == nil || fee.Sign() <= 0 {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"--taker-fee-percent must be a positive decimal percent per side (e.g. 0.8)",
					err,
				))
			}

			price.SetDefaultFee(kraken.TradeVolumeFee{Fee: fee})
			for _, sym := range detectSymbols {
				price.SetFee(sym, kraken.TradeVolumeFee{Fee: fee})
			}
		}

		budget := decimal.NewFromInt64(int64(system.Cfg.Market.Balance))
		if budget.Sign() <= 0 {
			budget = decimal.NewFromInt64(200)
		}
		price.SetReferenceCash(budget)

		detector := strategy.NewDetector(cmd.Context(), storeTee, price)

		workspace := nmruntime.NewWorkspace(
			cmd.Context(),
			2,
			"workspace",
			[][]nmruntime.Node{
				{
					replay.NewStage(cmd.Context(), "correlation", socket),
					replay.NewStage(cmd.Context(), "cvd", socket),
					replay.NewStage(cmd.Context(), "depthflow", socket),
					replay.NewStage(cmd.Context(), "hawkes", socket),
					replay.NewStage(cmd.Context(), "leadlag", socket),
					replay.NewStage(cmd.Context(), "liquidity", socket),
					replay.NewStage(cmd.Context(), "morphology", socket),
					replay.NewStage(cmd.Context(), "pumpdump", socket),
					replay.NewStage(cmd.Context(), "sentiment", socket),
					replay.NewStage(cmd.Context(), "toxicity", socket),
				},
				{
					detector,
				},
			},
		)

		for {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			default:
			}

			buf, err := socket.Read("trades", "spot:trade")
			if err != nil {
				break
			}

			trade := &data.Measurement{}
			if err := trade.UnmarshalBinary(buf); err != nil {
				continue
			}

			workspace.Step(trade)
		}

		detector.FlushAll()

		for storeTee.Pending() > 0 {
			time.Sleep(10 * time.Millisecond)
		}

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
