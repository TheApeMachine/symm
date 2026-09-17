package leadlag

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
	leadlag "github.com/theapemachine/symm/nomagique/statistic/leadlag"
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
	ticker := &Ticker{
		pipeline: nomagique.NewNumber(leadlag.
			NewGate(), leadlag.
			NewCross(algo.NewHayashiYoshida()), data.NewFinalizer[float64](),
		),
	}

	ticker.System = runtime.NewSystem(ctx, "leadlag:ticker", ticker)
	return ticker
}

/*
Next supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
	inputs:
		for arriving := range in {
			measurement := *(**data.Measurement[float64])(arriving)

			if ticker.Status() != runtime.READY {
				errnie.Warn(ticker.Name() + ": Next called before READY; dropping event")
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if measurement == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if len(measurement.Peers) > 0 {
				peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
					if candidate.Label == "" {
						return false
					}

					return quotedPrice(candidate) > 0
				})

				if peer == nil {
					continue inputs
				}

				price := quotedPrice(peer)
				measurement.Pull(peer)
				measurement.Metrics["last"] = measurement.Metrics["last"].Write(price)
				measurement.Metrics["last_price"] = measurement.Metrics["last_price"].Write(price)
			}

			res := sequence.Read[*data.Measurement[float64]](ticker.pipeline.Next(sequence.NewOne(unsafe.Pointer(&measurement)).Next(nil)))

			if res == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if res != nil && !yield(unsafe.Pointer(&res)) {
				return
			}
			continue inputs

		}
	}
}

func quotedPrice(measurement *data.Measurement[float64]) float64 {
	for _, key := range []string{"last_price", "last", "price"} {
		if metric, ok := measurement.Metrics[key]; ok && metric.Raw > 0 {
			return metric.Raw
		}
	}

	return 0
}

/*
Register returns the pre-allocated measurement every lead-lag tick flows
through: every metric the instrument can produce is declared, none valued.
The last trade price is the feed's fact the stages consume; the rest are
written where they are computed.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("leadlag", map[string]data.Metric[float64]{
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
	m.Metadata["peer-interest"] = "*"
	return m
}
