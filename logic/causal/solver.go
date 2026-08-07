package causal

import (
	"math"
	"sync"

	"github.com/alitto/pond/v2"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
Option configures the Causal solver.
*/
type Option func(*Solver)

/*
WithPearlConfig customizes Pearl's causal ladder configuration.
*/
func WithPearlConfig(config algorithm.PearlConfig) Option {
	return func(s *Solver) {
		s.config = config
	}
}

/*
Solver evaluates Judea Pearl's Causal Ladder over live market measurements,
physics fluid metrics, and predictive coding resonance predictions.
*/
type Solver struct {
	recorder *audit.Recorder
	pearls   *sync.Map
	config   algorithm.PearlConfig
	ui       chan []byte
	pool     pond.Pool
	group    pond.TaskGroup
}

/*
NewSolver creates a typed causal solver.
Default layout (4-column row):
  - Col 0: Control 1 (Resonance System Energy)
  - Col 1: Control 2 (Resonance Surprise / Anomaly)
  - Col 2: Treatment (Resonance Task Prediction / Expected Return)
  - Col 3: Target (Realized Price Return)
*/
func NewSolver(ui chan []byte, recorder *audit.Recorder, opts ...Option) *Solver {
	defaultConfig := algorithm.PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		NonlinearCounterfactual: true,
	}

	solver := &Solver{
		recorder: recorder,
		pearls:   &sync.Map{},
		config:   defaultConfig,
		ui:       ui,
		pool:     pond.NewPool(16),
	}

	solver.group = solver.pool.NewGroup()

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Update extracts aligned causal rows from Thesis, evaluates Pearl's causal
ladder, and stores each symbol's output directly on thesis.Causal.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if !thesis.Readiness.Resonance {
		return nil
	}

	thesis.Resonance.Range(func(key, _ any) bool {
		symbol, symbolOK := key.(string)

		if !symbolOK || symbol == "" {
			return true
		}

		solver.group.SubmitErr(func() error {
			return solver.measure(thesis, symbol)
		})

		return true
	})

	if err := solver.group.Wait(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "causal: parallel evaluation failed", err,
		))
	}

	thesis.Stamp(types.SourceCausal)

	solver.publish(thesis)
	return nil
}

func (solver *Solver) measure(
	thesis *types.Thesis,
	symbol string,
) error {
	stored, found := thesis.Resonance.Load(symbol)

	if !found {
		return nil
	}

	resonance, ok := stored.(types.ResonanceReading)

	if !ok || resonance.Forecast == nil {
		return nil
	}

	if err := resonance.Forecast.Validate(); err != nil {
		return nil
	}

	prediction, predictionReady := resonance.Forecast.Step(0)

	if !predictionReady || math.IsNaN(resonance.Energy) ||
		math.IsInf(resonance.Energy, 0) || math.IsNaN(resonance.Surprise) ||
		math.IsInf(resonance.Surprise, 0) {
		return nil
	}

	stored, found = thesis.Measurements.Load(types.SourceSentiment)

	if !found {
		return nil
	}

	measurements, ok := stored.([]*types.Measurement)

	if !ok {
		return errnie.Err(
			errnie.Validation,
			"causal: sentiment measurements have an invalid type",
			nil,
		)
	}

	changeKey := types.MetricKey(types.MetricChange, types.SideNone)
	realizedReturn := 0.0
	found = false
	var latestAt int64

	for _, measurement := range measurements {
		if measurement == nil || measurement.Symbol != symbol {
			continue
		}

		change, present := measurement.Metrics[changeKey]

		if !present || math.IsNaN(change.Raw) || math.IsInf(change.Raw, 0) {
			continue
		}

		at := measurement.At.UnixNano()

		if found && at <= latestAt {
			continue
		}

		realizedReturn = change.Raw
		latestAt = at
		found = true
	}

	if !found {
		return nil
	}

	output, ready, err := solver.getPearl(symbol).Measure(algorithm.PearlInput{
		Key: symbol,
		Row: []float64{
			resonance.Energy,
			resonance.Surprise,
			prediction,
			realizedReturn,
		},
		Contagion:    resonance.Surprise,
		Condition:    resonance.Energy,
		Intervention: prediction,
	})

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"causal: pearl evaluation failed",
			err,
		)
	}

	causalOutput := map[string]any{
		"ready": ready,
	}

	if ready {
		causalOutput = output.Outputs()
		causalOutput["ready"] = true
	}

	thesis.Causal.Store(symbol, causalOutput)
	return nil
}

/*
publish emits one causal wire frame per symbol observed on this tick.
*/
func (solver *Solver) publish(thesis *types.Thesis) {
	if solver.ui == nil || thesis == nil {
		return
	}

	rows := make([]map[string]any, 0)

	thesis.Causal.Range(func(key, value any) bool {
		causalMap, ok := value.(map[string]any)
		symbol, symbolOK := key.(string)

		if !ok || !symbolOK || symbol == "" {
			return true
		}

		if _, present := causalMap["symbol"]; !present {
			causalMap["symbol"] = symbol
		}

		rows = append(rows, causalMap)
		return true
	})

	if len(rows) > 0 {
		select {
		case solver.ui <- datura.NewMap("causal", rows).MarshalAndFree():
		default:
		}
	}
}

/*
getPearl lazily gets or creates a Pearl causal evaluator per symbol.
*/
func (solver *Solver) getPearl(symbol string) *algorithm.Pearl {
	p, ok := solver.pearls.Load(symbol)

	if !ok {
		p = algorithm.NewPearl(solver.config)
		solver.pearls.Store(symbol, p)
	}

	return p.(*algorithm.Pearl)
}

/*
Close cleans up the solver instance.
*/
func (solver *Solver) Close() error {
	solver.pool.StopAndWait()
	return nil
}
