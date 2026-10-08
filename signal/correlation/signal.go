package correlation

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

var outputKeys = []string{
	"last_price",
	"observation_count",
	"signed_correlation",
	"absolute_correlation",
	"cohort_signed_correlation",
	"cohort_absolute_correlation",
	"covariance",
	"return_energy:reference",
	"return_energy:measured",
	"return_energy_rate:reference",
	"return_energy_rate:measured",
	"peer_return_energy_rate",
	"focal_return_energy_rate",
	"supported_return_count:measured",
	"supported_return_count:reference",
	"shared_time",
	"overlap_density",
	"overlap_pair_count",
	"effective_sample_count",
	"correlation_p_value",
	"correlation_standard_error_fisher",
	"cohort_peer_count",
	"cohort_correlation_dispersion",
	"cohort_effective_peer_count",
	"relative_return_energy",
	"relative_cohort_return_energy",
	"correlation_baseline",
	"correlation_divergence",
	"correlation_zscore",
	"correlation_velocity",
	"relative_return_energy_baseline",
	"relative_return_energy_divergence",
	"relative_return_energy_zscore",
	"relative_return_energy_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	stages core.Primitive
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		stages: transport.NewStages(
			nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			nmcorrelation.NewFold(),
			nmcorrelation.NewRelative(),
			nmcorrelation.NewHistory(),
			nmcorrelation.NewCorrelationVelocity(),
			nmcorrelation.NewEnergyVelocity(),
		),
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	entry := data.Pull(prior.Read("price"))

	if entry == nil || entry.Metric == nil {
		signal.Error(errnie.Err(errnie.Validation, "[signal.correlation] missing price metric", nil))
		return nil
	}

	price := entry.Metric.Raw
	prior.Put("last_price", price)

	measurement := prior

	for ptr := range signal.stages.Next(transport.NewOne(unsafe.Pointer(&measurement)).Next(nil)) {
		measurement = *(**data.Measurement)(ptr)
	}

	if err := signal.stages.Error(); err != nil {
		signal.Error(err)
		return nil
	}

	output := make(map[string]float64, len(outputKeys))

	for _, key := range outputKeys {
		output[key] = measurement.Value(key)
	}

	output["last_price"] = price
	return prior.Next(signal.Name(), output)
}
