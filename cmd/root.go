package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "net/http/pprof"

	"github.com/fasthttp/websocket"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	symmlive "github.com/theapemachine/symm/live"
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

			errnie.Apply(&errnie.Config{
				Level: viper.GetViper().GetString("system.log.level"),
			})

			errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))

			pool := qpool.NewQ[any](ctx, 1, runtime.NumCPU(), &qpool.Config{
				SchedulingTimeout: viper.GetDuration("system.qpool.scheduling_timeout"),
				Regulators: []qpool.Regulator{
					qpool.NewRegulator(qpool.NewCircuitBreaker(
						viper.GetInt("system.qpool.regulators.circuit_breaker.max_failures"),
						viper.GetDuration("system.qpool.regulators.circuit_breaker.reset_timeout"),
						viper.GetInt("system.qpool.regulators.circuit_breaker.max_half_open"),
					)),
					qpool.NewRegulator(qpool.NewRateLimiter(
						viper.GetInt("system.qpool.regulators.rate_limiter.max_requests"),
						viper.GetDuration("system.qpool.regulators.rate_limiter.interval"),
					)),
					qpool.NewRegulator(qpool.NewBackPressureRegulator(
						viper.GetInt("system.qpool.regulators.back_pressure.max_queue_size"),
						viper.GetDuration("system.qpool.regulators.back_pressure.interval"),
						viper.GetDuration("system.qpool.regulators.back_pressure.timeout"),
					)),
					qpool.NewRegulator(qpool.NewResourceGovernorRegulator(
						viper.GetFloat64("system.qpool.regulators.resource_governor.max_cpu_percent"),
						viper.GetFloat64("system.qpool.regulators.resource_governor.max_memory_percent"),
						viper.GetDuration("system.qpool.regulators.resource_governor.interval"),
					)),
				},
			})

			tree := dmt.NewTree(viper.GetString("cognitive.persist_dir"))

			go func() {
				publicSocket := public.NewWebSocket(
					ctx,
					pool,
					tree,
					websocket.DefaultDialer,
					[]string{"ticker"},
					[]string{"kraken:public"},
				)

				defer publicSocket.Close()
				publicSocket.Run(public.WebSocketURL)
			}()

			tradingModel := viper.GetViper().GetString("trading.model")
			privateAccountEndpoint := public.WebSocketAuthURL

			if tradingModel == "paper" {
				emulator, err := public.NewEmulator(ctx, pool, tree)

				if err != nil {
					cancel()
					return errnie.Error(errnie.Err(
						errnie.IO,
						"emulator: failed to create private websocket emulator",
						err,
					))
				}

				defer emulator.Close()
				privateAccountEndpoint = emulator.Endpoint()

				go func() {
					errnie.Error(emulator.Serve())
				}()
			}

			if tradingModel == "paper" || tradingModel == "live" {
				if err := symmlive.ValidateReadiness(); err != nil {
					cancel()
					return errnie.Error(err)
				}

				token := public.NewRest(
					ctx,
					tree,
					string(public.EndpointWebSocketsToken),
				)

				defer token.Close()
				public.BindTokenRest(token)
			}

			go func() {
				accountSocket := public.NewWebSocket(
					ctx,
					pool,
					tree,
					websocket.DefaultDialer,
					[]string{"balances", "executions", "orders"},
					[]string{"kraken:private"},
				)

				defer accountSocket.Close()
				accountSocket.Run(privateAccountEndpoint)
			}()

			go func() {
				level3Socket := public.NewWebSocket(
					ctx,
					pool,
					tree,
					websocket.DefaultDialer,
					[]string{"level3"},
					[]string{"kraken:private"},
				)

				defer level3Socket.Close()
				level3Socket.Run(public.WebSocketL3URL)
			}()

			go func() {
				cryptoTrader, err := trader.NewCrypto(ctx, pool, tree)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"trader: failed to create crypto",
						err,
					))

					cancel()
					return
				}

				defer cryptoTrader.Close()
				errnie.Error(cryptoTrader.Run())
			}()

			uiHub, err := ui.NewHub(ctx, pool, tree)

			if err != nil {
				cancel()
				return errnie.Error(errnie.Err(
					errnie.IO,
					"ui: failed to create hub",
					err,
				))
			}

			defer uiHub.Close()
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
