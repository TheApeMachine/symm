package causal

import (
	"context"
	"errors"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/learning"
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
	thesis   *types.Thesis
	price    *broker.Price
	recorder *audit.Recorder
	pearls   *sync.Map
	rows     *sync.Map
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
func NewSolver(thesis *types.Thesis, price *broker.Price, ui chan []byte, recorder *audit.Recorder, opts ...Option) *Solver {
	ctx, cancel := context.WithCancel(context.Background())
	defaultConfig := algorithm.PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		NonlinearCounterfactual: true,
	}

	solver := &Solver{
		ctx:      ctx,
		cancel:   cancel,
		thesis:   thesis,
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

	go solver.run()
	return solver
}

func (solver *Solver) Name() string {
	return "causal"
}

/*
run consumes each symbol's resonance stream from the Resonance input MapReduce
and pushes derived causal outputs to the Causal output MapReduce. A failed pass
cancels the pond group; causal evaluation is retried on every enriched thesis,
so each pass gets a fresh group.
*/
func (solver *Solver) run() {
	for {
		select {
		case <-solver.ctx.Done():
			return
		default:
		}

		if !solver.pending() {
			// No symbol has new resonance to evaluate; yield instead of spinning
			// through an empty stage.
			runtime.Gosched()
			continue
		}

		group, _ := errgroup.WithContext(solver.ctx)
		updated := false

		solver.thesis.Symbols.Range(func(key, value any) bool {
			symbol, symbolOK := key.(string)
			symbolState, stateOK := value.(*types.Symbol)

			if !symbolOK || symbol == "" || !stateOK || symbolState == nil {
				return true
			}

			if symbolState.Resonance.Length() == 0 {
				return true
			}

			updated = true
			group.Go(func() error {
				return solver.measure(solver.thesis, symbol)
			})

			return true
		})

		if err := group.Wait(); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"causal: parallel evaluation failed: "+err.Error(),
				err,
			))
		}

		if updated {
			solver.publish(solver.thesis)
		}
	}
}

/*
pending reports whether any symbol has unconsumed resonance on its input
MapReduce, so the run loop can yield without processing when idle.
*/
func (solver *Solver) pending() bool {
	if solver.thesis == nil {
		return false
	}

	hasWork := false

	solver.thesis.Symbols.Range(func(_, value any) bool {
		symbolState, valid := value.(*types.Symbol)

		if !valid || symbolState == nil {
			return true
		}

		if symbolState.Resonance.Length() > 0 {
			hasWork = true

			return false
		}

		return true
	})

	return hasWork
}

/*
popResonanceManifold drains this symbol's resonance artifacts to the causal
stage and hands back the most recent predictive manifold it finds.
*/
func (solver *Solver) popResonanceManifold(symbolState *types.Symbol) (*learning.ResonanceManifold, bool) {
	var manifold *learning.ResonanceManifold

	for stored := range symbolState.MarketResonance(types.SourceCausal) {
		if coder, valid := stored.(*learning.ResonanceManifold); valid && coder != nil {
			manifold = coder
		}
	}

	if manifold == nil {
		return nil, false
	}

	return manifold, true
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

	coder, ok := solver.popResonanceManifold(symbolState)

	if !ok {
		return nil
	}

	forecast, err := coder.RolloutTaskForecast(1)

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"causal: resonance forecast failed: "+err.Error(),
			err,
		)
	}

	forecastReady := len(forecast) > 0 && forecast[0].Ready
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
		if errors.Is(err, io.EOF) {
			solver.storeUnresolved(symbolState, tickerAt, rows, prediction)
			return nil
		}

		return errnie.Err(
			errnie.UnprocessableContent,
			"causal: pearl evaluation failed: "+err.Error(),
			err,
		)
	}

	if !resolved {
		solver.storeUnresolved(symbolState, tickerAt, rows, prediction)
		return nil
	}

	precision, err := solver.estimatePrecision(rows)

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"causal: precision estimation failed: "+err.Error(),
			err,
		)
	}

	causalOutput := output.Outputs()
	causalOutput["historyRows"] = rows
	causalOutput["at"] = tickerAt
	causalOutput["identification"] = "adjustedAssociation"
	causalOutput["precision"] = precision
	causalOutput["samples"] = len(rows)
	causalOutput["treatmentLevel"] = prediction

	symbolState.Causal.Push(causalOutput)
	return nil
}

func (solver *Solver) storeUnresolved(
	symbolState *types.Symbol,
	at time.Time,
	rows [][]float64,
	prediction float64,
) {
	if len(rows) == 0 {
		return
	}

	precision, err := solver.estimatePrecision(rows)

	if err != nil {
		return
	}

	symbolState.Causal.Push(map[string]any{
		"at":             at,
		"historyRows":    rows,
		"identification": "unresolved",
		"precision":      precision,
		"samples":        len(rows),
		"treatmentLevel": prediction,
	})
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

		for causalMap := range symbolState.MarketCausal(types.SourceCausal) {
			if _, present := causalMap["symbol"]; !present {
				causalMap["symbol"] = symbol
			}

			rows = append(rows, causalMap)
		}

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
