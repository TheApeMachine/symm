package cmd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal/profiling"
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

			profileServer, profileErr := profiling.MaybeStart(cmd.Context())

			if profileErr != nil {
				return profileErr
			}

			if profileServer != nil {
				defer func() {
					if closeErr := profileServer.Close(); closeErr != nil {
						errnie.Error(closeErr)
					}
				}()
			}

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

			trader := trader.NewCrypto(cmd.Context(), pool)
			defer trader.Close()

			return nil
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
