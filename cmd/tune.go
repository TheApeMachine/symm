package cmd

import (
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/optimizer"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/toxicity"
)

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Search perspective trees against a replay capture",
	Long:  tuneLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		pool := errnie.Does(func() (*qpool.Q, error) {
			return qpool.NewQ(cmd.Context(), 1, 4, nil), nil
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		engine := errnie.Does(func() (*Engine, error) {
			return NewEngine(cmd.Context(), pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		engine.AddSystems(
			causal.NewSignal(cmd.Context(), pool),
			correlation.NewSignal(cmd.Context(), pool),
			cvd.NewSignal(cmd.Context(), pool),
			depthflow.NewSignal(cmd.Context(), pool),
			exhaust.NewSignal(cmd.Context(), pool),
			fluid.NewSignal(cmd.Context(), pool, focus.NewSet()),
			hawkes.NewSignal(cmd.Context(), pool),
			leadlag.NewSignal(cmd.Context(), pool),
			liquidity.NewSignal(cmd.Context(), pool),
			pumpdump.NewSignal(cmd.Context(), pool),
			sentiment.NewSignal(cmd.Context(), pool),
			toxicity.NewToxicity(cmd.Context(), pool),
			optimizer.NewTuner(cmd.Context(), pool),
			optimizer.NewTrader(cmd.Context(), pool),
		)

		return errnie.Error(engine.Start())
	},
}

func init() {
	rootCmd.AddCommand(tuneCmd)
}

const tuneLong = `
Run the optimizer against a replay capture.

Use cmd/cfg/tune.yml (make tune passes --config). Measurements go to optimizer.Tuner;
actions go to optimizer.Trader.
`
