package correlation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
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
	paths    *nmcorrelation.PathStore
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context) *Signal {
	paths := nmcorrelation.NewPathStore()
	signal := &Signal{
		paths: paths,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"pathstore", store.NewKV(),
				nomagique.NewNumber(
					nmcorrelation.NewMember("", paths),
					nmcorrelation.NewPairs("", paths, algo.NewHayashiYoshida()),
					data.NewSelect(
						0,  //  0: last_price
						1,  //  1: observation_count
						2,  //  2: signed_correlation
						3,  //  3: absolute_correlation
						4,  //  4: cohort_signed_correlation
						5,  //  5: cohort_absolute_correlation
						6,  //  6: covariance
						7,  //  7: return_energy:reference
						8,  //  8: return_energy:measured
						9,  //  9: return_energy_rate:reference
						10, // 10: return_energy_rate:measured
						11, // 11: peer_return_energy_rate
						12, // 12: focal_return_energy_rate
						13, // 13: supported_return_count:measured
						14, // 14: supported_return_count:reference
						15, // 15: shared_time
						16, // 16: overlap_density
						17, // 17: overlap_pair_count
						18, // 18: effective_sample_count
						19, // 19: correlation_p_value
						20, // 20: correlation_standard_error_fisher
						21, // 21: cohort_peer_count
						22, // 22: cohort_correlation_dispersion
						23, // 23: cohort_effective_peer_count
						24, // 24: relative_return_energy
						25, // 25: relative_cohort_return_energy
						26, // 26: correlation_baseline
						27, // 27: correlation_divergence
						28, // 28: correlation_zscore
						29, // 29: correlation_velocity
						30, // 30: relative_return_energy_baseline
						31, // 31: relative_return_energy_divergence
						32, // 32: relative_return_energy_zscore
						33, // 33: relative_return_energy_velocity
						34, // 34: historical_path_distance
						35, // 35: historical_path_percentile
					),
				),
				data.NewMessage(
					data.WRITE,
					"pathstore",
					"correlation_state",
					data.NewValue[core.Primitive](),
				),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	price := data.Pull(prior.Read("price")).Metric.Raw
	atNano := float64(prior.At.UnixNano())
	signal.paths.SetCurrent(prior.Label)

	output := make(map[string]float64)
	output["last_price"] = price

	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"pathstore",
			prior.Label,
			data.NewValue(price, atNano),
		).Next(nil),
	) {
		if ptr == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[signal.correlation] pipeline returned nil",
				nil,
			))

			return nil
		}

		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.correlation] pipeline output overflow",
				nil,
			))

			return nil
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	if index != len(outputKeys) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[signal.correlation] incomplete pipeline output",
			nil,
		))

		return nil
	}

	output["last_price"] = price

	return prior.Next(signal.Name(), output)
}
