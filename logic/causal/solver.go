package causal

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
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
	ctx      context.Context
	cancel   context.CancelFunc
	recorder *audit.Recorder
	pearls   *sync.Map
	rows     *sync.Map
	rowsMu   sync.Mutex
	config   algorithm.PearlConfig
	ui       chan []byte
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
		ctx:      context.Background(),
		cancel:   func() {},
		recorder: recorder,
		pearls:   &sync.Map{},
		rows:     &sync.Map{},
		config:   defaultConfig,
		ui:       ui,
	}

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
	if thesis.Readiness.Resonance {
		// A failed pass cancels a pond group permanently. Causal evaluation is
		// retried on every enriched Thesis, so it requires a new group per pass.
		group, _ := errgroup.WithContext(solver.ctx)

		thesis.Resonance.Range(func(key, _ any) bool {
			symbol, symbolOK := key.(string)

			if !symbolOK || symbol == "" {
				return true
			}

			group.Go(func() error {
				return solver.measure(thesis, symbol)
			})

			return true
		})

		if err := group.Wait(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"causal: parallel evaluation failed: "+err.Error(),
				err,
			))
		}

		thesis.Stamp(types.SourceCausal)
		solver.publish(thesis)
	}

	thesis.Fanout(types.SourceCausal)
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

	if !ok {
		return errnie.Err(
			errnie.Validation,
			"causal: resonance reading has an invalid type",
			nil,
		)
	}

	if resonance.Forecast == nil {
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

	row := []float64{
		resonance.Energy,
		resonance.Surprise,
		prediction,
		realizedReturn,
	}
	rows := solver.observe(symbol, row)

	output, resolved, err := solver.getPearl(symbol).Measure(algorithm.PearlInput{
		Key:          symbol,
		Row:          row,
		Contagion:    resonance.Surprise,
		Condition:    resonance.Energy,
		Intervention: prediction,
	})

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"causal: pearl evaluation failed: "+err.Error(),
			err,
		)
	}

	precision, err := solver.estimatePrecision(rows)

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"causal: precision estimation failed: "+err.Error(),
			err,
		)
	}

	causalOutput := map[string]any{
		"historyRows":    rows,
		"precision":      precision,
		"samples":        len(rows),
		"treatmentLevel": prediction,
	}

	if resolved {
		causalOutput = output.Outputs()
		causalOutput["historyRows"] = rows
		causalOutput["precision"] = precision
		causalOutput["samples"] = len(rows)
		causalOutput["treatmentLevel"] = prediction
	}

	thesis.Causal.Store(symbol, causalOutput)
	return nil
}

/*
observe retains the same aligned causal rows the Pearl model observed.

The capacity is the number of first- and second-moment parameters implied by
the row width, matching the model's own data-backed window without introducing
an independent history limit.
*/
func (solver *Solver) observe(symbol string, row []float64) [][]float64 {
	solver.rowsMu.Lock()
	defer solver.rowsMu.Unlock()

	stored, _ := solver.rows.LoadOrStore(symbol, [][]float64{})
	rows := stored.([][]float64)
	rows = append(rows, row)
	rowWidth := len(row)
	capacity := 1 + rowWidth + rowWidth*(rowWidth+1)/2

	if len(rows) > capacity {
		rows = rows[len(rows)-capacity:]
	}

	solver.rows.Store(symbol, rows)
	return append([][]float64(nil), rows...)
}

/*
estimatePrecision reports the weakest finite-sample precision required to
identify a treatment effect: both the treatment and target must vary. It uses
the same predictive-scale precision measure as nomagique's online
standardizer, so precision rises continuously instead of crossing a row-count
gate.
*/
func (solver *Solver) estimatePrecision(rows [][]float64) (float64, error) {
	treatment := adaptive.NewStandardizer()
	target := adaptive.NewStandardizer()

	for _, row := range rows {
		if solver.config.Treatment < 0 || solver.config.Treatment >= len(row) ||
			solver.config.Target < 0 || solver.config.Target >= len(row) {
			return 0, errnie.Err(
				errnie.Validation,
				"causal: treatment and target must fit retained row width",
				nil,
			)
		}

		if _, err := treatment.Measure(row[solver.config.Treatment]); err != nil {
			return 0, err
		}

		if _, err := target.Measure(row[solver.config.Target]); err != nil {
			return 0, err
		}
	}

	return math.Min(treatment.Precision(), target.Precision()), nil
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
	solver.cancel()
	return nil
}
