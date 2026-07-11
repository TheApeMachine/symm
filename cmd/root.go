package cmd

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
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
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			if err := validateTradingModel(); err != nil {
				return err
			}

			reportLiveReadiness()

			errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))
			startPprof()

			tree := dmt.NewTree(viper.GetString("cognitive.persist_dir"))

			pool := qpool.NewQ[any](ctx, runtime.NumCPU(), runtime.NumCPU(), nil)

			public := websocket.New(
				ctx,
				pool,
				"ws.kraken.com/v2",
				"https://api.kraken.com",
				false,
				true,
			)

			private := websocket.New(
				ctx,
				pool,
				"ws-auth.kraken.com/v2",
				"https://api.kraken.com",
				true,
				false,
			)

			level3 := websocket.New(
				ctx,
				pool,
				"ws-l3.kraken.com/v2",
				"https://api.kraken.com",
				true,
				false,
			)

			api := websocket.NewAPI(public, private, level3)
			defer api.Close()

			channel := make(chan []byte, viper.GetInt("system.websocket.channel.buffer"))

			price := broker.NewPrice(api, channel)
			balance := broker.NewBalance(api, channel)
			uiHub, err := ui.NewHub(ctx, price, balance, channel)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create UI hub",
					err,
				))
			}

			defer uiHub.Close()

			crypto, err := trader.NewCrypto(
				ctx,
				pool,
				tree,
				api,
				price,
				balance,
				channel,
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create crypto",
					err,
				))
			}

			defer crypto.Close()
			crypto.Run()

			return uiHub.Serve()
		},
	}
)

func Execute() {
	err := rootCmd.Execute()

	if err != nil {
		os.Exit(1)
	}
}

func validateTradingModel() error {
	model := strings.ToLower(strings.TrimSpace(
		viper.GetString("trading.model"),
	))

	if model == "paper" {
		return nil
	}

	return errnie.Err(
		errnie.NotAcceptable,
		"live trading remediation lock: trading.model must be paper",
		nil,
	)
}

/*
liveReadiness evaluates every live.* confirmation and positive risk limit in
cfg/config.yml against the loaded configuration, returning one description
per unmet requirement. An empty result means every configured gate is
satisfied; it does not mean live trading is unlocked, since that also
requires trading.model to leave paper (see validateTradingModel).
*/
func liveReadiness() []string {
	reasons := make([]string, 0)

	requireConfirmed := func(key, label string) {
		if !viper.GetBool(key) {
			reasons = append(reasons, label+" ("+key+") not confirmed")
		}
	}

	requireConfirmed("live.api_key_permissions_confirmed", "API key permissions")
	requireConfirmed("live.clock_synchronized", "clock synchronization")
	requireConfirmed("live.exchange_connectivity_confirmed", "exchange connectivity")
	requireConfirmed("live.paper_live_parity_passed", "paper/live parity")
	requireConfirmed("live.native_protective_stops_supported", "native protective stops")

	if strings.TrimSpace(viper.GetString("live.confirm")) == "" {
		reasons = append(reasons, "live.confirm operator acknowledgement is empty")
	}

	if viper.GetFloat64("live.max_order_notional") <= 0 {
		reasons = append(reasons, "live.max_order_notional must be a configured positive limit")
	}

	if viper.GetFloat64("live.max_daily_loss") <= 0 {
		reasons = append(reasons, "live.max_daily_loss must be a configured positive limit")
	}

	return reasons
}

/*
reportLiveReadiness logs the outcome of liveReadiness at startup. Live
trading is never unlocked by this report; validateTradingModel is the sole
gate. This only makes the live.* confirmations and limits executable and
visible instead of inert documentation.
*/
func reportLiveReadiness() {
	reasons := liveReadiness()

	if len(reasons) == 0 {
		errnie.Info("live readiness: all configured live.* gates satisfied (trading remains paper-locked)")
		return
	}

	errnie.Info(fmt.Sprintf(
		"live readiness: %d gate(s) unmet, live trading stays locked: %s",
		len(reasons), strings.Join(reasons, "; "),
	))
}

func startPprof() {
	if !viper.GetBool("system.pprof.enabled") && os.Getenv("SYMM_PPROF") == "" {
		return
	}

	addr := viper.GetString("system.pprof.addr")

	if addr == "" {
		addr = "127.0.0.1:6060"
	}

	go func() {
		errnie.Error(http.ListenAndServe(addr, nil))
	}()
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
			fmt.Fprintf(os.Stderr, "embedded config file not readable: %v\n", err)
			os.Exit(1)
		}

		defer cfgReader.Close()

		if readErr := viper.ReadConfig(cfgReader); readErr != nil {
			fmt.Fprintf(os.Stderr, "embedded config file not readable: %v\n", readErr)
			os.Exit(1)
		}
	}

	viper.WatchConfig()
}

const rootLong = `
Shake your money maker like somebody's 'bout to pay ya
Don't worry about them haters, keep your nose up in the ayer
You know I got it, if you wanna come get it
Stand next to this money like - ey ey ey

Shake, shake, shake your money maker
Like you were shaking it for some paper

...
`
