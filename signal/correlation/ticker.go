package correlation

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
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path correlation instrument. It holds no
state and no logic of its own: its entire behavior is distributed across
nomagique metric pipelines registered as cells with the coordinate grid.
Each metric defines its interests in raw market data, receives its coordinate
and bi-directional communication pipe from the grid, and publishes observations.
*/
type Ticker struct {
	*runtime.System
	metrics  map[string]*nomagique.Number
	pipeline core.Primitive
}

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate]) *Ticker {
	ticker := &Ticker{
		metrics: map[string]*nomagique.Number{
			"last_price": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{{"ticker", "data", "price"}},
				),
				nmcorrelation.NewGate(),
			),
			"observation_count": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{{"ticker", "data", "price"}},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"signed_correlation": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"absolute_correlation": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"cohort_signed_correlation": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"cohort_absolute_correlation": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"covariance": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"return_energy:reference": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"return_energy:measured": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"return_energy_rate:reference": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"return_energy_rate:measured": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"overlap_density": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"peer_return_energy_rate": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"supported_return_count:measured": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"supported_return_count:reference": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"overlap_pair_count": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"shared_time": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"correlation_p_value": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"correlation_standard_error_fisher": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			),
			"cohort_peer_count": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"cohort_effective_peer_count": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"cohort_correlation_dispersion": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"relative_return_energy": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
			),
			"correlation_baseline": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewHistory(),
			),
			"correlation_divergence": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewHistory(),
			),
			"correlation_zscore": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewHistory(),
			),
			"correlation_velocity": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewCorrelationVelocity(),
			),
			"relative_return_energy_baseline": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewRelative(),
			),
			"relative_return_energy_divergence": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewRelative(),
			),
			"relative_return_energy_zscore": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewRelative(),
			),
			"relative_return_energy_velocity": nomagique.NewNumber(
				transport.NewConn[*geometry.Coordinate](
					grid,
					[][]string{
						{"ticker", "data", "price"},
						{"ticker", "data", "timestamp"},
					},
				),
				nmcorrelation.NewGate(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				nmcorrelation.NewFold(),
				nmcorrelation.NewEnergyVelocity(),
			),
		},
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

	ticker.System = runtime.NewSystem(ctx, "correlation:ticker", ticker)
	ticker.Transition(runtime.READY)
	return ticker
}

/*
Next supplies arriving market data to the correlation pipeline.
Computed metric observations are published across their assigned
bi-directional communication pipes to the grid.
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

					metric, ok := candidate.Metrics["last_price"]

					return ok && metric.Raw > 0
				})

				if peer == nil {
					continue inputs
				}

				measurement.Pull(peer)
				measurement.Metrics["last_price"] = measurement.Metrics["last_price"].Write(peer.Metrics["last_price"].Raw)
			}

			res := sequence.Read[*data.Measurement[float64]](ticker.pipeline.Next(sequence.NewOne(unsafe.Pointer(&measurement)).Next(nil)))

			if res == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}

				continue inputs
			}

			for _, metric := range ticker.metrics {
				for range metric.Next(sequence.NewOne(unsafe.Pointer(&res)).Next(nil)) {
				}
			}

			if !yield(unsafe.Pointer(&res)) {
				return
			}

			continue inputs
		}
	}
}

/*
Metrics returns the addressable metric pipelines registered with the grid.
*/
func (ticker *Ticker) Metrics() map[string]*nomagique.Number {
	return ticker.metrics
}

/*
Register returns the pre-allocated measurement every correlation tick flows
through: every metric the instrument can produce is declared, none valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("correlation", map[string]data.Metric[float64]{
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
	m.ID = -1
	m.Metadata["peer-interest"] = "*"
	return m
}
