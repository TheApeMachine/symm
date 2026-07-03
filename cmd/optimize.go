package cmd

import (
	"context"
	"fmt"
	"runtime"

	"github.com/fasthttp/websocket"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/trader"
)

var optimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Replay captured Kraken websocket frames through candidate playbooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		errnie.Apply(&errnie.Config{
			Level: viper.GetViper().GetString("system.log.level"),
		})

		errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))
		startPprof()

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
		defer pool.Close()

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
		privateAccountEndpoint := emulator.Endpoint()

		go func() {
			errnie.Error(emulator.Serve())
		}()

		go func() {
			accountSocket := public.NewReplayer(
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

		<-ctx.Done()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(optimizeCmd)
}
