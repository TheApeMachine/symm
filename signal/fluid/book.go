package fluid

import (
	"encoding/json"
	"io"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Book struct {
	registry *Registry
	tree     *dmt.Tree
	algo     io.ReadWriteCloser
}

func NewBook(registry *Registry, tree *dmt.Tree) *Book {
	book := &Book{
		registry: registry,
		tree:     tree,
	}

	book.algo = nomagique.Number(
		equation.NewFluidflow(equation.FluidflowConfig()),
		probability.NewClassifier(datura.Acquire(
			"fluid", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"laminarScore",
				"turbulentScore",
				"inertialScore",
				"viscousScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryLaminar)),
				float64(logic.CategoryIndex(logic.CategoryTurbulent)),
				float64(logic.CategoryIndex(logic.CategoryInertial)),
				float64(logic.CategoryIndex(logic.CategoryViscous)),
			},
		})),
	)

	return book
}

func (book *Book) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	eventAt, err := eventTime(frame, -1)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	symbol, _ := frame.Scope()
	state := book.registry.loadSymbol(symbol)

	if state == nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: symbol state required",
			nil,
		)))
	}

	update := bookUpdate(frame, -1, symbol, eventAt)

	if len(update.Bids) == 0 && len(update.Asks) == 0 {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"fluid: book bids or asks required",
			nil,
		)))
	}

	if err := book.setInstrumentTick(symbol); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if !state.HasBook() && len(update.Bids) > 0 && len(update.Asks) > 0 {
		update.Type = "snapshot"
	}

	if err := state.FeedBook(update, eventAt); errnie.Error(err) != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	reading, ok := state.Reading()

	if !ok {
		return nil
	}

	return book.measurementFromReading(reading, eventAt)
}

func (book *Book) measurementFromReading(reading fluidReading, eventAt time.Time) *datura.Artifact {
	measurement := datura.Acquire("fluid", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(reading.symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceFluid)))
	measurement.SetTimestamp(eventAt.UnixNano())

	measurement.WithPayload(datura.Map[any]{
		"price":          reading.price,
		"last":           reading.price,
		"spreadBPS":      reading.spreadBPS,
		"volume":         reading.volume,
		"change_pct":     reading.changePct,
		"re":             reading.reynolds,
		"div":            reading.divergence,
		"vort":           reading.vorticity,
		"turb":           reading.turbulence,
		"visc":           reading.viscosity,
		"src_bal":        reading.sourceBalance,
		"memory":         reading.memory,
		"midAddRate":     reading.midAddRate,
		"midExecuteRate": reading.midExecuteRate,
		"features":       book.fluidflowFeatures(reading),
		"inputs":         equation.FluidflowInputKeys,
		"root":           "features",
		"timestamp":      eventAt.UnixNano(),
		"output": datura.Map[any]{
			"viscosity":      reading.viscosity,
			"reynolds":       reading.reynolds,
			"divergence":     reading.divergence,
			"vorticity":      reading.vorticity,
			"turbulence":     reading.turbulence,
			"sourceBalance":  reading.sourceBalance,
			"memory":         reading.memory,
			"midAddRate":     reading.midAddRate,
			"midExecuteRate": reading.midExecuteRate,
		},
	}.Marshal())

	if err := nomagique.RoundTripArtifact(measurement, book.algo); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		).With(measurement.Log()...))

		return nil
	}

	measurement.MergeOutputs(map[string]any{
		"viscosity":      reading.viscosity,
		"reynolds":       reading.reynolds,
		"divergence":     reading.divergence,
		"vorticity":      reading.vorticity,
		"turbulence":     reading.turbulence,
		"sourceBalance":  reading.sourceBalance,
		"memory":         reading.memory,
		"midAddRate":     reading.midAddRate,
		"midExecuteRate": reading.midExecuteRate,
		"laminar":        datura.Peek[float64](measurement, "output", "laminarScore"),
		"turbulent":      datura.Peek[float64](measurement, "output", "turbulentScore"),
		"inertial":       datura.Peek[float64](measurement, "output", "inertialScore"),
		"viscous":        datura.Peek[float64](measurement, "output", "viscousScore"),
	})

	return measurement
}

func (book *Book) fluidflowFeatures(reading fluidReading) []float64 {
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

	return []float64{
		book.finitePositive(reading.reynolds),
		book.finitePositive(divergence),
		book.finitePositive(reading.viscosity),
		book.finitePositive(reading.midAddRate),
		book.finitePositive(reading.midExecuteRate),
		book.finitePositive(laminarCeiling),
		book.finitePositive(turbulentFloor),
		turbulentReady,
		book.finitePositive(divergenceEdge),
		book.finitePositive(reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)),
		book.finitePositive(vorticity),
		book.finitePositive(turbulence),
		book.finitePositive(reading.memory),
		book.finitePositive(reading.price),
		book.finitePositive(reading.spreadBPS),
		book.finite(reading.changePct),
		book.finitePositive(reading.volume),
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

func (book *Book) setInstrumentTick(symbol string) error {
	if book.tree == nil {
		return nil
	}

	raw, ok := book.tree.Get([]byte("instrument/" + symbol + "/"))

	if !ok {
		return nil
	}

	artifact := datura.Acquire("fluid", datura.APPJSON)
	defer artifact.Release()

	if _, err := artifact.Unpack(raw); err != nil {
		return err
	}

	var meta struct {
		TickSize float64 `json:"tick_size"`
	}

	if err := json.Unmarshal(artifact.DecryptPayload(), &meta); err != nil {
		return err
	}

	book.registry.SetInstrumentTickSize(symbol, meta.TickSize)

	return nil
}
