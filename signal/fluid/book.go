package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Book struct {
	registry   *Registry
	fluidflow  *equation.Fluidflow
	classifier *probability.ScoreClassifier
}

func NewBook(registry *Registry) *Book {
	return &Book{
		registry:  registry,
		fluidflow: equation.NewFluidflow(),
		classifier: probability.NewScoreClassifier(
			[]string{"laminarScore", "turbulentScore", "inertialScore", "viscousScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryLaminar)),
				float64(logic.CategoryIndex(logic.CategoryTurbulent)),
				float64(logic.CategoryIndex(logic.CategoryInertial)),
				float64(logic.CategoryIndex(logic.CategoryViscous)),
			},
		),
	}
}

func (book *Book) Measure(row kraken.BookData) (*logic.Measurement, error) {
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
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: book bids or asks required",
			nil,
		))
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

	return book.measurementFromReading(reading, eventAt)
}

func (book *Book) measurementFromReading(
	reading fluidReading,
	eventAt time.Time,
) (*logic.Measurement, error) {
	output, err := book.fluidflow.Measure(book.fluidflowInput(reading))

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if output.Strength <= 0 {
		return nil, nil
	}

	result, err := book.classifier.Classify(map[string]float64{
		"laminarScore":   output.LaminarScore,
		"turbulentScore": output.TurbulentScore,
		"inertialScore":  output.InertialScore,
		"viscousScore":   output.ViscousScore,
		"strength":       output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourceFluid, reading.symbol, eventAt)
	measurement.AddMetric("laminarScore", output.LaminarScore)
	measurement.AddMetric("turbulentScore", output.TurbulentScore)
	measurement.AddMetric("inertialScore", output.InertialScore)
	measurement.AddMetric("viscousScore", output.ViscousScore)
	measurement.AddMetric("strength", output.Strength)
	measurement.AddMetric("viscosity", reading.viscosity)
	measurement.AddMetric("reynolds", reading.reynolds)
	measurement.AddMetric("divergence", reading.divergence)
	measurement.AddMetric("vorticity", reading.vorticity)
	measurement.AddMetric("turbulence", reading.turbulence)
	measurement.AddMetric("sourceBalance", reading.sourceBalance)
	measurement.AddMetric("memory", reading.memory)
	measurement.AddMetric("midAddRate", reading.midAddRate)
	measurement.AddMetric("midExecuteRate", reading.midExecuteRate)
	measurement.AddMetric("laminar", output.LaminarScore)
	measurement.AddMetric("turbulent", output.TurbulentScore)
	measurement.AddMetric("inertial", output.InertialScore)
	measurement.AddMetric("viscous", output.ViscousScore)

	if err := measurement.ApplyClassifier(
		result.Value,
		result.Confidence,
		result.EntryBaseline,
		result.ExitBaseline,
		result.Strength,
		result.Distribution,
	); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if err := measurement.Ready(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return measurement, nil
}

func (book *Book) fluidflowInput(reading fluidReading) equation.FluidflowInput {
	divergence := math.Abs(reading.divergence)
	vorticity := math.Abs(reading.vorticity)
	turbulence := math.Abs(reading.turbulence)
	laminarCeiling := book.baseline(0.5, reading.dynamics.reynoldsHistory, reading.reynolds)
	turbulentFloor := book.baseline(0.75, reading.dynamics.reynoldsHistory, reading.reynolds)
	divergenceEdge := book.baseline(0.5, reading.dynamics.divergenceHistory, divergence)
	turbulentReady := 0.0

	if turbulentFloor > 0 {
		turbulentReady = 1
	}

	return equation.FluidflowInput{
		Reynolds:       book.finitePositive(reading.reynolds),
		Divergence:     book.finitePositive(divergence),
		Viscosity:      book.finitePositive(reading.viscosity),
		MidAddRate:     book.finitePositive(reading.midAddRate),
		MidExecuteRate: book.finitePositive(reading.midExecuteRate),
		LaminarCeiling: book.finitePositive(laminarCeiling),
		TurbulentFloor: book.finitePositive(turbulentFloor),
		TurbulentReady: turbulentReady > 0,
		DivergenceEdge: book.finitePositive(divergenceEdge),
		IcebergScore:   book.finitePositive(reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)),
		Vorticity:      book.finitePositive(vorticity),
		Turbulence:     book.finitePositive(turbulence),
		Memory:         book.finitePositive(reading.memory),
		Price:          book.finitePositive(reading.price),
		SpreadBPS:      book.finitePositive(reading.spreadBPS),
		ChangePct:      book.finite(reading.changePct),
		Volume:         book.finitePositive(reading.volume),
	}
}

func (book *Book) baseline(percentile float64, values []float64, sample float64) float64 {
	if len(values) > 0 {
		return sampleQuantile(percentile, values)
	}

	return sample
}

func (book *Book) finite(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

func (book *Book) finitePositive(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return book.finite(value)
}
