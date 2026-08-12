package causal

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
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
	price    *broker.Price
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
func NewSolver(price *broker.Price, ui chan []byte, recorder *audit.Recorder, opts ...Option) *Solver {
	defaultConfig := algorithm.PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		NonlinearCounterfactual: true,
	}

	solver := &Solver{
		ctx:      context.Background(),
		cancel:   func() {},
		price:    price,
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
	// A failed pass cancels a pond group permanently. Causal evaluation is
	// retried on every enriched Thesis, so it requires a new group per pass.
	group, _ := errgroup.WithContext(solver.ctx)
	updated := false

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		symbolState, stateOK := value.(*types.Symbol)

		if !symbolOK || symbol == "" || !stateOK || symbolState == nil {
			return true
		}

		resonanceValue, found := symbolState.Resonance.Load(symbol)

		if !found {
			return true
		}

		_, valid := resonanceValue.(*learning.ResonanceManifold)

		if !valid {
			return true
		}

		updated = true
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

	if !updated {
		return nil
	}

	solver.publish(thesis)
	return nil
}

func (solver *Solver) measure(
	thesis *types.Thesis,
	symbol string,
) error {
	symbolValue, found := thesis.Symbols.Load(symbol)

	if !found {
		return nil
	}

	symbolState := symbolValue.(*types.Symbol)
	stored, found := symbolState.Resonance.Load(symbol)

	if !found {
		return nil
	}

	coder, ok := stored.(*learning.ResonanceManifold)

	if !ok {
		return errnie.Err(
			errnie.Validation,
			"causal: resonance manifold has an invalid type",
			nil,
		)
	}

	forecast, err := coder.RolloutTaskForecast(1)
	forecastReady := err == nil && len(forecast) > 0 && forecast[0].Ready
	prediction := 0.0

	if forecastReady {
		prediction = forecast[0].Value
	}

	_, surprise, energy := coder.WireSnapshot()

	midpoint := 0.0
	tickerAt := thesis.At

	if solver.price != nil {
		ticker := solver.price.Tick(symbol)

		if ticker != nil {
			tickerAt = ticker.Timestamp

			if ticker.Bid != nil && ticker.Ask != nil {
				bid := ticker.Bid.Float64()
				ask := ticker.Ask.Float64()

				if bid > 0 && ask >= bid {
					midpoint = (bid + ask) / 2
				}
			}

			if midpoint == 0 && ticker.Last != nil && ticker.Last.Sign() > 0 {
				midpoint = ticker.Last.Float64()
			}
		}
	}

	if midpoint == 0 {
		return nil
	}

	row, rows, resolved, err := solver.observe(
		symbol,
		[3]float64{energy, surprise, prediction},
		midpoint,
		tickerAt,
		forecastReady,
	)

	if err != nil {
		return err
	}

	if !resolved {
		return nil
	}

	output, resolved, err := solver.getPearl(symbol).Measure(algorithm.PearlInput{
		Key:          symbol,
		Row:          row,
		Contagion:    surprise,
		Condition:    energy,
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
		"at":             tickerAt,
		"historyRows":    rows,
		"identification": "adjustedAssociation",
		"precision":      precision,
		"samples":        len(rows),
		"treatmentLevel": prediction,
	}

	if resolved {
		causalOutput = output.Outputs()
		causalOutput["historyRows"] = rows
		causalOutput["at"] = tickerAt
		causalOutput["identification"] = "adjustedAssociation"
		causalOutput["precision"] = precision
		causalOutput["samples"] = len(rows)
		causalOutput["treatmentLevel"] = prediction
	}

	symbolState.Causal.Store(symbol, causalOutput)
	return nil
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

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		symbolState, stateOK := value.(*types.Symbol)

		if !symbolOK || symbol == "" || !stateOK || symbolState == nil {
			return true
		}

		stored, found := symbolState.Causal.Load(symbol)
		causalMap, ok := stored.(map[string]any)

		if !found || !ok {
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
