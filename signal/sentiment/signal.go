package sentiment

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var outputKeys = []string{
	"cohort_member_count",
	"valid_member_count",
	"excluded_member_count",
	"cohort_horizon_seconds",
	"return",
	"absolute_return",
	"asof_age_seconds",
	"from_age_seconds",
	"advance_count",
	"decline_count",
	"unchanged_count",
	"advance_fraction",
	"decline_fraction",
	"unchanged_fraction",
	"directional_participation",
	"breadth",
	"directional_agreement",
	"directional_consensus",
	"median_return",
	"median_absolute_return",
	"mean_absolute_return",
	"rms_return",
	"return_mad",
	"magnitude_mad",
	"return_interquartile_range",
	"largest_move_tie_count",
	"largest_absolute_return",
	"largest_signed_return",
	"largest_move_share",
	"peer_median_absolute_return",
	"peer_magnitude_mad",
	"largest_move_excess",
	"largest_move_ratio",
	"largest_move_mad_excess",
	"same_direction_peer_count",
	"opposite_direction_peer_count",
	"zero_return_peer_count",
	"same_direction_peer_fraction",
	"opposite_direction_peer_fraction",
	"zero_return_peer_fraction",
	"breadth_baseline",
	"breadth_divergence",
	"breadth_zscore",
	"median_return_baseline",
	"median_return_divergence",
	"median_return_zscore",
	"median_return_velocity",
	"breadth_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	pipeline core.Primitive
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		pipeline: nomagique.NewNumber(
			crosssection.NewCohort(),
		),
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		return nil
	}

	price := priceEntry.Metric.Raw
	if price <= 0 {
		return nil
	}

	atNano := float64(prior.At.UnixNano())
	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"sentiment",
			prior.Label,
			data.NewValue(price, atNano),
		).Next(nil),
	) {
		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.sentiment] overflow",
				nil,
			))
			return nil
		}

		if ptr == nil {
			continue
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	return prior.Next(signal.Name(), output)
}
