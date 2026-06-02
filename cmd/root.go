package cmd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
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
	"github.com/theapemachine/symm/toxicity"
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

var (
	cfgFile string

	rootCmd = &cobra.Command{
		Use:   "symm",
		Short: "S.Y.M.M. is not financial advice.",
		Long:  rootLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			pool := qpool.NewQ(cmd.Context(), 1, 4, nil)

			engine, err := NewEngine(cmd.Context(), pool)

			if err != nil {
				return err
			}

			streams := focus.NewSet()

			story, storyErr := market.NewStory(cmd.Context(), pool, streams)

			if storyErr != nil {
				return storyErr
			}

			hub, hubErr := ui.NewHub(cmd.Context(), pool)

			if hubErr != nil {
				return hubErr
			}

			crypto, cryptoErr := trader.NewCrypto(cmd.Context(), pool, streams)

			if cryptoErr != nil {
				return cryptoErr
			}

			activate.Boot("engine registering systems trading.model=" + viper.GetString("trading.model"))

			if err := configureLevel3(
				cmd.Context(),
				os.Getenv("SYMM_KRAKEN_API_KEY"),
				os.Getenv("SYMM_KRAKEN_API_SECRET"),
			); err != nil {
				return err
			}

			if err := engine.AddSystems(
				hub,
				public.NewWebSocket(cmd.Context(), pool, streams),
				private.NewWebSocket(
					cmd.Context(),
					pool,
					os.Getenv("SYMM_KRAKEN_API_KEY"),
					os.Getenv("SYMM_KRAKEN_API_SECRET"),
				),
				kraken.NewInstrument(cmd.Context(), pool),
				kraken.NewLevel3WebSocket(cmd.Context(), pool),
				causal.NewSignal(cmd.Context(), pool),
				correlation.NewSignal(cmd.Context(), pool),
				cvd.NewSignal(cmd.Context(), pool),
				depthflow.NewSignal(cmd.Context(), pool),
				exhaust.NewSignal(cmd.Context(), pool),
				fluid.NewSignal(cmd.Context(), pool),
				hawkes.NewSignal(cmd.Context(), pool),
				leadlag.NewSignal(cmd.Context(), pool),
				liquidity.NewSignal(cmd.Context(), pool),
				pumpdump.NewSignal(cmd.Context(), pool),
				sentiment.NewSignal(cmd.Context(), pool),
				toxicity.NewToxicity(cmd.Context(), pool),
				story,
				crypto,
			); err != nil {
				return err
			}

			activate.Boot("engine.Start")

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
