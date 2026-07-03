package cmd

import (
	"context"
	"runtime"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func newPool(ctx context.Context) *qpool.Q[any] {
	return qpool.NewQ[any](ctx, 1, runtime.NumCPU(), &qpool.Config{
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
}
