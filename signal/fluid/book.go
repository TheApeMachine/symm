package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
				float64(types.CategoryIndex(types.CategoryLaminar)),
				float64(types.CategoryIndex(types.CategoryTurbulent)),
				float64(types.CategoryIndex(types.CategoryInertial)),
				float64(types.CategoryIndex(types.CategoryViscous)),
			},
		),
	}
}

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

	measurement, err := book.measurementFromReading(reading, eventAt)

	if err != nil || measurement == nil {
		return nil, err
	}

	return []*types.Measurement{measurement}, nil
}

func (book *Book) measurementFromReading(
	reading fluidReading,
	eventAt time.Time,
) (*types.Measurement, error) {
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

	categories := []types.CategoryType{
		types.Laminar,
		types.Turbulent,
		types.Inertial,
		types.Viscous,
	}
	strengths := []float64{
		output.LaminarScore,
		output.TurbulentScore,
		output.InertialScore,
		output.ViscousScore,
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, category := range categories {
		confidence := 0.0

		if index < len(result.Probabilities) {
			confidence = result.Probabilities[index]
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourceFluid,
		Stream:        "book",
		Symbol:        reading.symbol,
		At:            eventAt,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"laminarScore":   output.LaminarScore,
			"turbulentScore": output.TurbulentScore,
			"inertialScore":  output.InertialScore,
			"viscousScore":   output.ViscousScore,
			"strength":       output.Strength,
			"viscosity":      reading.viscosity,
			"reynolds":       reading.reynolds,
			"divergence":     reading.divergence,
			"vorticity":      reading.vorticity,
			"turbulence":     reading.turbulence,
			"sourceBalance":  reading.sourceBalance,
			"memory":         reading.memory,
			"midAddRate":     reading.midAddRate,
			"midExecuteRate": reading.midExecuteRate,
			"laminar":        output.LaminarScore,
			"turbulent":      output.TurbulentScore,
			"inertial":       output.InertialScore,
			"viscous":        output.ViscousScore,
		},
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
