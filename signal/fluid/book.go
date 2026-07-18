package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Book owns fluid book measurement so order-book dynamics stay separate from
signal orchestration and downstream interpretation.
*/
type Book struct {
	registry  *Registry
	fluidflow *equation.Fluidflow
}

func NewBook(registry *Registry) *Book {
	return &Book{
		registry:  registry,
		fluidflow: equation.NewFluidflow(),
	}
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (book *Book) Measure(row kraken.BookData) ([]*types.Measurement, error) {
	if row.Timestamp.IsZero() {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: book event timestamp required",
			nil,
		))
	}

	if row.Type == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: book frame type required",
			nil,
		))
	}

	state := book.registry.loadSymbol(row.Symbol)

	if state == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		))
	}

	if len(row.Bids) == 0 && len(row.Asks) == 0 {
		// No book levels in this frame (checksum-only refresh, or a thin/halted
		// market with nothing resting on either side). There is no book state to
		// feed or measure, but it is not a malformed frame.
		return nil, nil
	}

	if row.PriceIncrement != nil && row.PriceIncrement.Float64() > 0 {
		state.setInstrumentTickSize(row.PriceIncrement.Float64())
	}

	eventAt := row.Timestamp.UTC()

	if err := state.FeedBook(row, eventAt); errnie.Error(err) != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	reading, ok := state.Reading()

	if !ok {
		return nil, nil
	}

	if reading.volume <= 0 || reading.price <= 0 || reading.spreadBPS <= 0 {
		return nil, nil
	}

	measurements, err := book.measurementsFromReading(reading, eventAt)

	if err != nil || len(measurements) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return measurements, nil
}

/*
measurementsFromReading turns one validated fluid reading into numerical
measurements so solver state is published without selecting a category.
*/
func (book *Book) measurementsFromReading(
	reading fluidReading,
	eventAt time.Time,
) ([]*types.Measurement, error) {
	output, err := book.fluidflow.Measure(book.fluidflowInput(reading))

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	maturity := float64(reading.gridSteps) / float64(reading.gridSteps+1)
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	historyFrom := eventAt
	earliestStamp := reading.dynamics.earliestStamp()

	if !earliestStamp.IsZero() {
		historyFrom = earliestStamp
	}

	subject := types.SubjectType("order_book")

	specs := []struct {
		metric types.MetricType
		unit   types.MeasurementUnit
		raw    float64
		from   time.Time
	}{
		{types.MetricLaminarScore, types.UnitDimensionless, output.LaminarScore, historyFrom},
		{types.MetricTurbulentScore, types.UnitDimensionless, output.TurbulentScore, historyFrom},
		{types.MetricInertialScore, types.UnitDimensionless, output.InertialScore, historyFrom},
		{types.MetricViscousScore, types.UnitDimensionless, output.ViscousScore, historyFrom},
		{types.MetricViscosity, types.UnitDimensionless, reading.viscosity, eventAt},
		{types.MetricReynolds, types.UnitDimensionless, reading.reynolds, eventAt},
		{types.MetricDivergenceV2, types.UnitInverseSecond, reading.divergence, eventAt},
		{types.MetricVelocityCurvatureV2, types.UnitInverseQuoteCurrencySecond, reading.velocityCurvature, eventAt},
		{types.MetricTurbulence, types.UnitQuoteCurrencyPerSecond, reading.turbulence, eventAt},
		{types.MetricSourceBalance, types.UnitBaseCurrency, reading.sourceBalance, eventAt},
		{types.MetricMemory, types.UnitDimensionless, reading.memory, eventAt},
		{types.MetricMidAddRate, types.UnitBaseCurrencyPerSecond, reading.midAddRate, eventAt},
		{types.MetricMidExecuteRate, types.UnitBaseCurrencyPerSecond, reading.midExecuteRate, eventAt},
	}

	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		from := spec.from
		through := eventAt

		if through.Before(from) {
			from = through
		}

		measurements = append(measurements, &types.Measurement{
			Source:       types.SourceFluid,
			Metric:       spec.metric,
			Subject:      subject,
			Stream:       types.Fluid,
			Symbol:       reading.symbol,
			Side:         types.SideNone,
			At:           through,
			ObservedFrom: from,
			Unit:         spec.unit,
			Raw:          spec.raw,
			Maturity:     maturity,
			Validity:     validity,
			Scale: types.ScaleReference{
				Kind:    types.ScaleObservationWindow,
				From:    from,
				Through: through,
			},
		})
	}

	return measurements, nil
}

/*
fluidflowInput maps empirical book dynamics and their per-symbol baselines into
dimensionless fluid-state evidence.
*/
func (book *Book) fluidflowInput(reading fluidReading) equation.FluidflowInput {
	divergence := math.Abs(reading.divergence)
	velocityCurvature := math.Abs(reading.velocityCurvature)
	turbulence := math.Abs(reading.turbulence)
	laminarCeiling := book.baseline(0.5, reading.dynamics.reynoldsHistory, reading.reynolds)
	turbulentFloor := book.baseline(0.75, reading.dynamics.reynoldsHistory, reading.reynolds)
	divergenceEdge := book.baseline(0.5, reading.dynamics.divergenceHistory, divergence)
	viscosityBaseline := book.baseline(
		0.5, reading.dynamics.viscosityHistory, reading.viscosity,
	)
	vorticityBaseline := book.baseline(
		0.5, reading.dynamics.velocityCurvatureHistory, velocityCurvature,
	)
	turbulenceBaseline := book.baseline(
		0.5, reading.dynamics.turbulenceHistory, turbulence,
	)

	return equation.FluidflowInput{
		Reynolds:           book.finitePositive(reading.reynolds),
		Divergence:         book.finitePositive(divergence),
		Viscosity:          book.finitePositive(reading.viscosity),
		LaminarCeiling:     book.finitePositive(laminarCeiling),
		TurbulentFloor:     book.finitePositive(turbulentFloor),
		DivergenceEdge:     book.finitePositive(divergenceEdge),
		ViscosityBaseline:  book.finitePositive(viscosityBaseline),
		Vorticity:          book.finitePositive(velocityCurvature),
		VorticityBaseline:  book.finitePositive(vorticityBaseline),
		Turbulence:         book.finitePositive(turbulence),
		TurbulenceBaseline: book.finitePositive(turbulenceBaseline),
	}
}

/*
baseline selects an empirical quantile when history exists and otherwise uses
the current sample as the only observed scale.
*/
func (book *Book) baseline(percentile float64, values []float64, sample float64) float64 {
	if len(values) > 0 {
		return sampleQuantile(percentile, values)
	}

	return sample
}

/*
finite removes non-finite solver values so invalid floating-point state is not
published as market evidence.
*/
func (book *Book) finite(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

/*
finitePositive retains finite positive values for equation inputs whose domain
excludes zero and negative values.
*/
func (book *Book) finitePositive(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return book.finite(value)
}
