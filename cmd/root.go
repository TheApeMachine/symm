package cmd

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "net/http/pprof"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/types"
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
			errnie.Apply(&errnie.Config{
				Level: viper.GetViper().GetString("system.log.level"),
			})

			startPprof()

			errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))

			pool := qpool.NewQ[any](cmd.Context(), 1, runtime.NumCPU(), &qpool.Config{
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

			publicRest := public.NewRest(cmd.Context(), tree)
			defer publicRest.Close()

			assetPairCount, assetPairErr := publicRest.LoadAssetPairs(cmd.Context())
			if assetPairErr != nil {
				return errnie.Error(assetPairErr)
			}
			errnie.Info(fmt.Sprintf("kraken/public: loaded %d AssetPairs fee schedules", assetPairCount))

			publicSocket := public.NewWebSocket(cmd.Context(), pool, tree)
			defer publicSocket.Close()

			go publicSocket.Run(public.WebSocketURL)

			if tradingModelLive() {
				privateRest := private.NewRest(
					cmd.Context(), public.EndpointWebSocketsToken, tree,
				)
				defer privateRest.Close()
				types.BindTokenRest(privateRest)

				privateSocket := private.NewWebSocket(cmd.Context(), pool, tree)
				defer privateSocket.Close()

				if err := privateSocket.Connect(string(public.WebSocketAuthURL), 1); err != nil {
					return err
				}

				go privateSocket.Run()
			} else {
				paperRest, paperRestErr := paper.NewRest(cmd.Context())

				if paperRestErr != nil {
					return paperRestErr
				}

				defer paperRest.Close()
				types.BindTokenRest(paperRest)

				paperSocket := paper.NewWebSocket(cmd.Context(), pool, tree)
				defer paperSocket.Close()

				go paperSocket.Run()
			}

			cryptoTrader, cryptoErr := trader.NewCrypto(cmd.Context(), pool, tree)

			if cryptoErr != nil {
				return errnie.Error(cryptoErr)
			}

			defer cryptoTrader.Close()

			uiHub, hubErr := ui.NewHub(cmd.Context(), pool, tree)

			if hubErr != nil {
				return errnie.Error(hubErr)
			}

			defer uiHub.Close()

			go func() {
				if err := cryptoTrader.Run(); err != nil {
					panic(err)
				}
			}()

			return errnie.Error(uiHub.Run())
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

func tradingModelLive() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SYMM_LIVE"))) {
	case "1", "true", "yes", "live":
		return true
	}

	return strings.EqualFold(strings.TrimSpace(viper.GetString("trading.model")), "live")
}

func startPprof() {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SYMM_PPROF"))) {
	case "1", "true", "yes", "on":
	case "":
		if !viper.GetBool("system.pprof.enabled") {
			return
		}
	default:
		return
	}

	addr := strings.TrimSpace(viper.GetString("system.pprof.addr"))
	if addr == "" {
		addr = "127.0.0.1:6060"
	}

	go func() {
		errnie.Info("pprof listening on http://" + addr + "/debug/pprof/")
		if err := http.ListenAndServe(addr, nil); err != nil {
			fmt.Fprintf(os.Stderr, "symm: pprof server: %v\n", err)
		}
	}()
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
