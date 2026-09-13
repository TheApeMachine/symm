package derivatives

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmderivatives "github.com/theapemachine/symm/nomagique/derivatives"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
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
	return &Ticker{
		System: runtime.NewSystem(ctx, "derivatives:ticker"),
		pipeline: nomagique.NewNumber(
			nmderivatives.NewGate(),
			nmderivatives.NewBasis(),
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
Register returns the pre-allocated measurement every futures ticker snapshot
flows through: every metric the instrument can produce is declared, none
valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("derivatives", map[string]data.Metric[float64]{
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
}
