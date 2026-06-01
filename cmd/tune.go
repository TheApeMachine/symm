package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	krakenreplay "github.com/theapemachine/symm/kraken/replay"
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
	Short: "Run the optimizer against a replay capture",
	Long:  tuneLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := applyTuneOverrides(); err != nil {
			return err
		}

		path, err := tuneReplayPath()

		if err != nil {
			return err
		}

		applyReplaySymbols()

		sessionCtx, sessionCancel := context.WithCancel(cmd.Context())
		defer sessionCancel()

		pool := errnie.Does(func() (*qpool.Q, error) {
			return qpool.NewQ(sessionCtx, 1, 4, nil), nil
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		engine := errnie.Does(func() (*Engine, error) {
			return NewEngine(sessionCtx, pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		file, err := os.Open(path)

		if err != nil {
			return errnie.Error(err)
		}

		defer file.Close()

		replaySocket, err := krakenreplay.NewWebSocket(sessionCtx, pool, file)

		if err != nil {
			return errnie.Error(err)
		}

		tuner := optimizer.NewTuner(sessionCtx, pool)
		trader := optimizer.NewTrader(sessionCtx, pool)
		tuner.BindTrader(trader)

		engine.AddSystems(
			replaySocket,
			causal.NewSignal(sessionCtx, pool),
			correlation.NewSignal(sessionCtx, pool),
			cvd.NewSignal(sessionCtx, pool),
			depthflow.NewSignal(sessionCtx, pool),
			exhaust.NewSignal(sessionCtx, pool),
			fluid.NewSignal(sessionCtx, pool, focus.NewSet()),
			hawkes.NewSignal(sessionCtx, pool),
			leadlag.NewSignal(sessionCtx, pool),
			liquidity.NewSignal(sessionCtx, pool),
			pumpdump.NewSignal(sessionCtx, pool),
			sentiment.NewSignal(sessionCtx, pool),
			toxicity.NewToxicity(sessionCtx, pool),
			tuner,
			trader,
		)

		runErr := engine.Start()
		summary := tuner.Summary()

		fmt.Fprintf(os.Stderr, "symm tune: %s\n", summary)

		if runErr != nil && runErr != context.Canceled {
			return errnie.Error(runErr)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuneCmd)
}

const tuneLong = `
Run the optimizer against a replay capture.

Use the default config (cmd/cfg/config.yml). Measurements go to optimizer.Tuner;
actions go to optimizer.Trader.
`

func applyTuneOverrides() error {
	viper.Set("trading.model", "replay")
	viper.Set("trading.replay.pace", time.Duration(0))
	viper.Set("trading.replay.loop", false)

	return nil
}

func tuneReplayPath() (string, error) {
	path := strings.TrimSpace(viper.GetString("trading.replay.file"))

	if path == "" {
		return "", fmt.Errorf("tune: trading.replay.file is required")
	}

	return path, nil
}

func applyReplaySymbols() {
	if len(viper.GetStringSlice("market.symbols")) > 0 {
		return
	}

	defaults := viper.GetStringSlice("market.default_symbols")

	if len(defaults) > 0 {
		viper.Set("market.symbols", defaults)
	}
}
