package sentiment

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmsentiment "github.com/theapemachine/symm/nomagique/sentiment"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the cross-sectional price-state instrument. It holds no state and
no logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	return &Ticker{
		System: runtime.NewSystem(ctx, "sentiment:ticker"),
		pipeline: nomagique.NewNumber(
			nmsentiment.NewGate(),
			nmsentiment.NewReturn(),
			nmsentiment.NewFold(),
			data.NewFinalizer[float64](),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	return data.Read[*data.Measurement[float64]](ticker.pipeline.Next(transport.NewOne(unsafe.Pointer(&m)).Next(nil)))
}

/*
Register returns the pre-allocated measurement every sentiment tick flows
through: every metric the instrument can produce is declared, none valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("sentiment", map[string]data.Metric[float64]{
		"last": data.NewMetric[float64](
			"last", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"return": data.NewMetric[float64](
			"return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"absolute_return": data.NewMetric[float64](
			"absolute_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"advance_count": data.NewMetric[float64](
			"advance_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"decline_count": data.NewMetric[float64](
			"decline_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"unchanged_count": data.NewMetric[float64](
			"unchanged_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"valid_member_count": data.NewMetric[float64](
			"valid_member_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_member_count": data.NewMetric[float64](
			"cohort_member_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"excluded_member_count": data.NewMetric[float64](
			"excluded_member_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"advance_fraction": data.NewMetric[float64](
			"advance_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"decline_fraction": data.NewMetric[float64](
			"decline_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"unchanged_fraction": data.NewMetric[float64](
			"unchanged_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"directional_participation": data.NewMetric[float64](
			"directional_participation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"directional_agreement": data.NewMetric[float64](
			"directional_agreement", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"directional_consensus": data.NewMetric[float64](
			"directional_consensus", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"same_direction_peer_count": data.NewMetric[float64](
			"same_direction_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"opposite_direction_peer_count": data.NewMetric[float64](
			"opposite_direction_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"zero_return_peer_count": data.NewMetric[float64](
			"zero_return_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"same_direction_peer_fraction": data.NewMetric[float64](
			"same_direction_peer_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"opposite_direction_peer_fraction": data.NewMetric[float64](
			"opposite_direction_peer_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"zero_return_peer_fraction": data.NewMetric[float64](
			"zero_return_peer_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"breadth": data.NewMetric[float64](
			"breadth", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"breadth_baseline": data.NewMetric[float64](
			"breadth_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"breadth_divergence": data.NewMetric[float64](
			"breadth_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"breadth_zscore": data.NewMetric[float64](
			"breadth_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"breadth_velocity": data.NewMetric[float64](
			"breadth_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_return": data.NewMetric[float64](
			"median_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_return_baseline": data.NewMetric[float64](
			"median_return_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_return_divergence": data.NewMetric[float64](
			"median_return_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_return_zscore": data.NewMetric[float64](
			"median_return_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_return_velocity": data.NewMetric[float64](
			"median_return_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_return": data.NewMetric[float64](
			"median_absolute_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_return_baseline": data.NewMetric[float64](
			"median_absolute_return_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_return_zscore": data.NewMetric[float64](
			"median_absolute_return_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_return_velocity": data.NewMetric[float64](
			"median_absolute_return_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_return_ratio": data.NewMetric[float64](
			"median_absolute_return_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"mean_absolute_return": data.NewMetric[float64](
			"mean_absolute_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"rms_return": data.NewMetric[float64](
			"rms_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_interquartile_range": data.NewMetric[float64](
			"return_interquartile_range", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_dispersion_baseline": data.NewMetric[float64](
			"return_dispersion_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_dispersion_zscore": data.NewMetric[float64](
			"return_dispersion_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_dispersion_velocity": data.NewMetric[float64](
			"return_dispersion_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"return_dispersion_ratio": data.NewMetric[float64](
			"return_dispersion_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_mad": data.NewMetric[float64](
			"return_mad", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"magnitude_mad": data.NewMetric[float64](
			"magnitude_mad", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_absolute_return": data.NewMetric[float64](
			"largest_absolute_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_tie_count": data.NewMetric[float64](
			"largest_move_tie_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_signed_return": data.NewMetric[float64](
			"largest_signed_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_excess": data.NewMetric[float64](
			"largest_move_excess", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_mad_excess": data.NewMetric[float64](
			"largest_move_mad_excess", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_ratio": data.NewMetric[float64](
			"largest_move_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_ratio_baseline": data.NewMetric[float64](
			"largest_move_ratio_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_ratio_zscore": data.NewMetric[float64](
			"largest_move_ratio_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_share": data.NewMetric[float64](
			"largest_move_share", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_share_baseline": data.NewMetric[float64](
			"largest_move_share_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"largest_move_share_zscore": data.NewMetric[float64](
			"largest_move_share_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"peer_median_absolute_return": data.NewMetric[float64](
			"peer_median_absolute_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"peer_magnitude_mad": data.NewMetric[float64](
			"peer_magnitude_mad", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_asof_age_seconds": data.NewMetric[float64](
			"median_asof_age_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"max_asof_age_seconds": data.NewMetric[float64](
			"max_asof_age_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_from_age_seconds": data.NewMetric[float64](
			"median_from_age_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_horizon_seconds": data.NewMetric[float64](
			"cohort_horizon_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"asof_age_seconds": data.NewMetric[float64](
			"asof_age_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"from_age_seconds": data.NewMetric[float64](
			"from_age_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
