package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	kraken "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market"
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
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
)

/*
Embed a mini filesystem into the binary to hold the default config file.
This will be written to the home directory of the user running the service,
which allows a developer to easily override the config file.
*/
//go:embed cfg/config.yml
var embedded embed.FS

// defaultCapturePath is where `make run --record` writes measurements and where
// `make tune` reads them back — the single capture file the two commands share.
const defaultCapturePath = "runs/capture.jsonl"

var (
	cfgFile   string
	recordRun bool

	rootCmd = &cobra.Command{
		Use:   "symm",
		Short: "S.Y.M.M. is not financial advice.",
		Long:  rootLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --record guarantees the run collects data for the optimizer, without
			// depending on trading.record.file being set in the config.
			if recordRun {
				viper.Set("trading.record.file", defaultCapturePath)
				errnie.Info("recording run measurements to "+defaultCapturePath+" (feeds `make tune`)", "engine")
			}

			pool := qpool.NewQ[any](cmd.Context(), 1, 4, nil)
			engine, err := NewEngine(cmd.Context(), pool)

			if err != nil {
				return err
			}

			systemCtx := engine.Context()
			streams := focus.NewSet()
			quotes := broker.EnsureQuoteCache(systemCtx, pool)
			stress := broker.EnsureStressCache(systemCtx, pool)
			crypto := trader.NewCryptoWithCaches(systemCtx, pool, streams, quotes, stress)
			story := newStoryWithBookCapture(systemCtx, pool, quotes, crypto)

			errnie.Info(
				"engine registering systems trading.model="+viper.GetString("trading.model"),
				"engine",
			)

			apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
			apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

			if err := engine.AddSystems(
				ui.NewHub(systemCtx, pool),
				public.NewWebSocket(systemCtx, pool, streams),
			); err != nil {
				return err
			}

			for _, runtime := range private.ExecutionSystems(
				systemCtx, pool, apiKey, apiSecret, quotes,
			) {
				if err := engine.AddSystems(runtime); err != nil {
					return err
				}
			}

			if err := engine.AddSystems(
				kraken.NewInstrument(systemCtx, pool),
				causal.NewSignal(systemCtx, pool),
				correlation.NewSignal(systemCtx, pool),
				cvd.NewSignal(systemCtx, pool),
				depthflow.NewSignal(systemCtx, pool),
				exhaust.NewSignal(systemCtx, pool),
				fluid.NewSignal(systemCtx, pool),
				hawkes.NewSignal(systemCtx, pool),
				leadlag.NewSignal(systemCtx, pool),
				liquidity.NewSignal(systemCtx, pool),
				pumpdump.NewSignal(systemCtx, pool),
				sentiment.NewSignal(systemCtx, pool),
				toxicity.NewToxicity(systemCtx, pool),
				story,
				crypto,
			); err != nil {
				return err
			}

			errnie.Info("engine.Start", "engine")
			return engine.Start()
		},
	}
)

func Execute() {
	err := rootCmd.Execute()

	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"path to config file (default: try cmd/cfg/config.yml, ./config.yml, $HOME/.symm/config.yml, then embedded default)",
	)

	rootCmd.PersistentFlags().BoolVar(
		&recordRun,
		"record",
		false,
		"record run measurements to "+defaultCapturePath+" so `make tune` can optimize on them",
	)
}

func initConfig() {
	viper.SetConfigType("yml")

	tryRead := func(path string) error {
		viper.SetConfigFile(path)
		return viper.ReadInConfig()
	}

	loaded := false

	if rootCmd.PersistentFlags().Changed("config") && strings.TrimSpace(cfgFile) != "" {
		if err := tryRead(cfgFile); err == nil {
			loaded = true
		} else {
			fmt.Fprintf(os.Stderr, "symm: config file %q: %v\n", cfgFile, err)
			os.Exit(1)
		}
	}

	if !loaded {
		paths := []string{
			"cmd/cfg/config.yml",
			"config.yml",
		}

		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, ".symm", "config.yml"))
		}

		for _, p := range paths {
			if err := tryRead(p); err == nil {
				loaded = true
				break
			}
		}
	}

	if !loaded {
		cfgReader, err := embedded.Open("cfg/config.yml")

		if err != nil {
			fmt.Printf("embedded config file not readable: %v\n", err)
			return
		}

		defer cfgReader.Close()

		if readErr := viper.ReadConfig(cfgReader); readErr != nil {
			fmt.Printf("embedded config file not readable: %v\n", readErr)
			return
		}
	}

	viper.WatchConfig()
}

func newStoryWithBookCapture(
	ctx context.Context,
	pool *qpool.Q[any],
	quotes *broker.QuoteCache,
	crypto *trader.Crypto,
) *market.Story {
	story := market.NewStory(ctx, pool)
	story.SetBookEnricher(broker.MeasurementBookEnricher(ctx, pool))
	story.SetQuoteReady(func(symbol string) bool {
		_, ok := quotes.Snapshot(symbol)

		return ok
	})
	story.SetPositionHeld(crypto.SymbolHeld)

	return story
}

const rootLong = `
Shake your money maker like somebody's 'bout to pay ya
I see you on my radar, don't you act like you're afraid of shit
You know I got it, If you wanna come get it
Stand next to this money like - ey ey ey
Shake your money maker like somebody's 'bout to pay ya
Don't worry about them haters, keep your nose up in the air
You know I got it, If you wanna come get it
Stand next to this money like - ey ey ey

Shake, shake, shake your money maker
Like you were shaking it for some paper

...
`
