package derivatives

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
	derivatives "github.com/theapemachine/symm/nomagique/statistic/derivatives"
)

/*
Ticker is the derivative/reference state measuring instrument. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the
measurement where it computes them, and the workload's register owns the
measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	ticker := &Ticker{
		pipeline: nomagique.NewNumber(derivatives.
			NewGate(), derivatives.
			NewBasis(), data.NewFinalizer[float64](),
		),
	}

	ticker.System = runtime.NewSystem(ctx, "derivatives:ticker", ticker)
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

					_, hasLast := candidate.Metrics["last"]
					_, hasIndex := candidate.Metrics["index_price"]
					_, hasMark := candidate.Metrics["mark_price"]
					_, hasOI := candidate.Metrics["open_interest"]
					return hasLast && hasIndex && hasMark && hasOI
				})

				if peer == nil {
					continue inputs
				}

				measurement.Pull(peer, "last", "index_price", "mark_price", "open_interest")
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

/*
Register returns the pre-allocated measurement every futures ticker snapshot
flows through: every metric the instrument can produce is declared, none
valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("derivatives", map[string]data.Metric[float64]{
		"derivative_price": data.NewMetric[float64](
			"derivative_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"reference_price": data.NewMetric[float64](
			"reference_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"spot_price": data.NewMetric[float64](
			"spot_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"open_interest": data.NewMetric[float64](
			"open_interest", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"basis": data.NewMetric[float64](
			"basis", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"basis_baseline": data.NewMetric[float64](
			"basis_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"basis_zscore": data.NewMetric[float64](
			"basis_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"log_basis": data.NewMetric[float64](
			"log_basis", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"derivative_index_log_basis": data.NewMetric[float64](
			"derivative_index_log_basis", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"index_spot_log_basis": data.NewMetric[float64](
			"index_spot_log_basis", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"derivative_spot_log_basis": data.NewMetric[float64](
			"derivative_spot_log_basis", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"basis_closure_error": data.NewMetric[float64](
			"basis_closure_error", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"open_interest_change": data.NewMetric[float64](
			"open_interest_change", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"open_interest_log_change": data.NewMetric[float64](
			"open_interest_log_change", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"open_interest_growth_rate": data.NewMetric[float64](
			"open_interest_growth_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"open_interest_growth_baseline": data.NewMetric[float64](
			"open_interest_growth_baseline", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"basis_change": data.NewMetric[float64](
			"basis_change", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"basis_rate": data.NewMetric[float64](
			"basis_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"basis_velocity": data.NewMetric[float64](
			"basis_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"derivative_log_return": data.NewMetric[float64](
			"derivative_log_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"reference_log_return": data.NewMetric[float64](
			"reference_log_return", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_gap": data.NewMetric[float64](
			"return_gap", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"return_gap_velocity": data.NewMetric[float64](
			"return_gap_velocity", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
