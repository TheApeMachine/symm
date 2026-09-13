package correlation

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path correlation instrument. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the
measurement where it computes them, and the workload's register owns the
measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
}

func NewTicker(ctx context.Context) *Ticker {
	return &Ticker{
		System: runtime.NewSystem(ctx, "correlation:ticker"),
		pipeline: nomagique.NewNumber(
			nmcorrelation.NewGate(),
			nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			nmcorrelation.NewFold(),
			nmcorrelation.NewHistory(),
			nmcorrelation.NewRelative(),
			nmcorrelation.NewCorrelationVelocity(),
			nmcorrelation.NewEnergyVelocity(),
			data.NewFinalizer[float64](),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	return data.Read[*data.Measurement[float64]](ticker.pipeline.Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))
}

/*
Register returns the pre-allocated measurement every correlation tick flows
through: every metric the instrument can produce is declared, none valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("correlation", map[string]data.Metric[float64]{
		"last_price": data.NewMetric[float64](
			"last_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"observation_count": data.NewMetric[float64](
			"observation_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_correlation": data.NewMetric[float64](
			"signed_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"absolute_correlation": data.NewMetric[float64](
			"absolute_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_signed_correlation": data.NewMetric[float64](
			"cohort_signed_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_absolute_correlation": data.NewMetric[float64](
			"cohort_absolute_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"covariance": data.NewMetric[float64](
			"covariance", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy:reference": data.NewMetric[float64](
			"return_energy:reference", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy:measured": data.NewMetric[float64](
			"return_energy:measured", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy_rate:reference": data.NewMetric[float64](
			"return_energy_rate:reference", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy_rate:measured": data.NewMetric[float64](
			"return_energy_rate:measured", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"overlap_density": data.NewMetric[float64](
			"overlap_density", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"peer_return_energy_rate": data.NewMetric[float64](
			"peer_return_energy_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"supported_return_count:measured": data.NewMetric[float64](
			"supported_return_count:measured", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"supported_return_count:reference": data.NewMetric[float64](
			"supported_return_count:reference", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"overlap_pair_count": data.NewMetric[float64](
			"overlap_pair_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"shared_time": data.NewMetric[float64](
			"shared_time", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_p_value": data.NewMetric[float64](
			"correlation_p_value", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_standard_error_fisher": data.NewMetric[float64](
			"correlation_standard_error_fisher", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_peer_count": data.NewMetric[float64](
			"cohort_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_effective_peer_count": data.NewMetric[float64](
			"cohort_effective_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_correlation_dispersion": data.NewMetric[float64](
			"cohort_correlation_dispersion", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy": data.NewMetric[float64](
			"relative_return_energy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_baseline": data.NewMetric[float64](
			"correlation_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_divergence": data.NewMetric[float64](
			"correlation_divergence", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_zscore": data.NewMetric[float64](
			"correlation_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_velocity": data.NewMetric[float64](
			"correlation_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_baseline": data.NewMetric[float64](
			"relative_return_energy_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_divergence": data.NewMetric[float64](
			"relative_return_energy_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_zscore": data.NewMetric[float64](
			"relative_return_energy_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_velocity": data.NewMetric[float64](
			"relative_return_energy_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
