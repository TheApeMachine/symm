package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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

			streams := focus.NewSet()

			activate.Boot("engine registering systems trading.model=" + viper.GetString("trading.model"))

			engine.AddSystems(
				ui.NewHub(cmd.Context(), pool),
				public.NewWebSocket(cmd.Context(), pool, streams),
				private.NewWebSocket(
					cmd.Context(),
					pool,
					os.Getenv("KRAKEN_API_KEY"),
					os.Getenv("KRAKEN_API_SECRET"),
				),
				kraken.NewInstrument(cmd.Context(), pool),
				causal.NewSignal(cmd.Context(), pool),
				correlation.NewSignal(cmd.Context(), pool),
				cvd.NewSignal(cmd.Context(), pool),
				depthflow.NewSignal(cmd.Context(), pool),
				exhaust.NewSignal(cmd.Context(), pool),
				fluid.NewSignal(cmd.Context(), pool, streams),
				hawkes.NewSignal(cmd.Context(), pool),
				leadlag.NewSignal(cmd.Context(), pool),
				liquidity.NewSignal(cmd.Context(), pool),
				pumpdump.NewSignal(cmd.Context(), pool),
				sentiment.NewSignal(cmd.Context(), pool),
				toxicity.NewToxicity(cmd.Context(), pool),
				market.NewStory(cmd.Context(), pool, streams),
				trader.NewCrypto(cmd.Context(), pool, streams),
			)

			activate.Boot("engine.Start")

			return errnie.Error(engine.Start())
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
		cfgReader := errnie.Does(func() (fs.File, error) {
			return embedded.Open("cfg/config.yml")
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		defer cfgReader.Close()

		if readErr := viper.ReadConfig(cfgReader); readErr != nil {
			fmt.Printf("embedded config file not readable: %v\n", readErr)
			return
		}
	}

	viper.WatchConfig()

	if strings.TrimSpace(viper.GetString("ui.addr")) == "" {
		viper.Set("ui.addr", "127.0.0.1:8765")
	}
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
