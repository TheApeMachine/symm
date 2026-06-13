package cmd

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/calibration"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/futures"
	"github.com/theapemachine/symm/kraken/paper"
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
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/prediction"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/trader"
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
			tradingConfig, err := config.LoadTradingConfig()

			if err != nil {
				return errnie.Error(err)
			}

			if tradingConfig.Model == "live" {
				liveConfig := config.LoadLiveReadinessConfig()

				if readinessErr := config.CheckLiveReadiness(
					tradingConfig,
					liveConfig,
					liveReadinessDependencies(tradingConfig),
				); readinessErr != nil {
					return errnie.Error(readinessErr)
				}
			}

			errnie.Apply(&errnie.Config{
				Level: viper.GetViper().GetString("system.log.level"),
			})

			pool := qpool.NewQ[any](cmd.Context(), 1, runtime.NumCPU()*2, &qpool.Config{
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

			engine, err := NewEngine(cmd.Context(), pool)

			if err != nil {
				return err
			}

			systemCtx := engine.Context()

			touchRegistry, touchErr := market.NewTouchRegistry(systemCtx, pool)

			if touchErr != nil {
				return errnie.Error(touchErr)
			}

			market.RegisterTouchRegistry(touchRegistry)

			calibrationRegistry := calibration.NewRegistry()
			calibration.Register(calibrationRegistry)
			calibration.WireLogic()

			systems := []System{
				public.NewWebSocket(systemCtx, pool),
			}

			switch tradingConfig.Model {
			case "paper":
				paperWebSocket := paper.NewWebSocket(systemCtx, pool)

				if paperWebSocket == nil {
					return errors.New("paper trading websocket failed to initialize")
				}

				systems = append(systems, paperWebSocket)
			case "live":
				privateWebSocket := private.NewWebSocket(systemCtx, pool)

				if privateWebSocket == nil {
					return errors.New(
						"live trading requires SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET",
					)
				}

				systems = append(systems, privateWebSocket)
			default:
				return fmt.Errorf("unsupported trading model %q", tradingConfig.Model)
			}

			systems = append(systems,
				touchRegistry,
				causal.NewSystem(systemCtx, pool),
				correlation.NewSystem(systemCtx, pool),
				cvd.NewSystem(systemCtx, pool),
				depthflow.NewSystem(systemCtx, pool),
				exhaust.NewSystem(systemCtx, pool),
				fluid.NewSystem(systemCtx, pool),
				hawkes.NewSystem(systemCtx, pool),
				leadlag.NewSystem(systemCtx, pool),
				manifold.NewSystem(systemCtx, pool),
				liquidity.NewSystem(systemCtx, pool),
				prediction.NewSystem(systemCtx, pool),
				pumpdump.NewSystem(systemCtx, pool),
				sentiment.NewSystem(systemCtx, pool),
				toxicity.NewSystem(systemCtx, pool),
				trader.NewCrypto(systemCtx, pool),
				broker.NewDesk(systemCtx, pool, touchRegistry),
			)

			story, storyErr := market.NewStory(systemCtx, pool, touchRegistry)

			if storyErr != nil {
				return storyErr
			}

			systems = append(systems, story)

			if viper.GetBool("market.futures_enabled") {
				systems = append([]System{futures.NewWebSocket(systemCtx, pool)}, systems...)
			}

			if viper.GetBool("market.l3_enabled") && tradingConfig.Model == "paper" {
				privateWebSocket := private.NewWebSocket(systemCtx, pool)

				if privateWebSocket == nil {
					return errors.New(
						"market.l3_enabled requires SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET",
					)
				}

				systems = append(systems, privateWebSocket)
			}

			if err := engine.AddSystems(systems...); err != nil {
				return err
			}

			errnie.Info("engine.Start", "engine")
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
