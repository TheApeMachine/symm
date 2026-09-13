package leadlag

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmleadlag "github.com/theapemachine/symm/nomagique/leadlag"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path lead-lag instrument. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over
the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's
lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	return &Ticker{
		System: runtime.NewSystem(ctx, "leadlag:ticker"),
		pipeline: nomagique.NewNumber(
			nmleadlag.NewGate(),
			nmleadlag.NewCross(algo.NewHayashiYoshida()),
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
Register returns the pre-allocated measurement every lead-lag tick flows
through: every metric the instrument can produce is declared, none valued.
The last trade price is the feed's fact the stages consume; the rest are
written where they are computed.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("leadlag", map[string]data.Metric[float64]{
		"last": data.NewMetric[float64](
			"last", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"last_price": data.NewMetric[float64](
			"last_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"observation_count": data.NewMetric[float64](
			"observation_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"contemporaneous_correlation": data.NewMetric[float64](
			"contemporaneous_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"best_lag_correlation": data.NewMetric[float64](
			"best_lag_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"absolute_correlation_gain": data.NewMetric[float64](
			"absolute_correlation_gain", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_fraction": data.NewMetric[float64](
			"lag_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"best_lag_index": data.NewMetric[float64](
			"best_lag_index", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"reference_return_count": data.NewMetric[float64](
			"reference_return_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"measured_return_count": data.NewMetric[float64](
			"measured_return_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"overlap_pair_count": data.NewMetric[float64](
			"overlap_pair_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"effective_sample_count": data.NewMetric[float64](
			"effective_sample_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"search_count": data.NewMetric[float64](
			"search_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"best_lag_seconds": data.NewMetric[float64](
			"best_lag_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_search_resolution_seconds": data.NewMetric[float64](
			"lag_search_resolution_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_search_span": data.NewMetric[float64](
			"lag_search_span", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_peak_prominence": data.NewMetric[float64](
			"lag_peak_prominence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_peak_curvature": data.NewMetric[float64](
			"lag_peak_curvature", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_p_value": data.NewMetric[float64](
			"correlation_p_value", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"search_adjusted_p_value": data.NewMetric[float64](
			"search_adjusted_p_value", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_baseline_seconds": data.NewMetric[float64](
			"lag_baseline_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_divergence_seconds": data.NewMetric[float64](
			"lag_divergence_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_noise_scale_seconds": data.NewMetric[float64](
			"lag_noise_scale_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_zscore": data.NewMetric[float64](
			"lag_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"lag_velocity": data.NewMetric[float64](
			"lag_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_gain_baseline": data.NewMetric[float64](
			"correlation_gain_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_gain_zscore": data.NewMetric[float64](
			"correlation_gain_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_gain_velocity": data.NewMetric[float64](
			"correlation_gain_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"best_lag_correlation_baseline": data.NewMetric[float64](
			"best_lag_correlation_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"best_lag_correlation_zscore": data.NewMetric[float64](
			"best_lag_correlation_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
