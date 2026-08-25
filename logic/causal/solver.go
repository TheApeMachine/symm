package causal

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	thesis   *types.Thesis
	price    *broker.Price
	recorder *audit.Recorder
	pearls   *sync.Map
	rows     *sync.Map
	config   algorithm.PearlConfig
	causal   *runtime.Channel[types.CausalOutput]
	work     *runtime.Subscription[types.ResonanceArtifact]
}

/*
NewSolver creates a typed causal solver.
Default layout (4-column row):
  - Col 0: Control 1 (Resonance System Energy)
  - Col 1: Control 2 (Resonance Surprise / Anomaly)
  - Col 2: Treatment (Resonance Task Prediction / Expected Return)
  - Col 3: Target (Realized Price Return)
*/
func NewSolver(bus *runtime.Workspace, opts ...Option) *Solver {
	ctx, cancel := context.WithCancel(context.Background())
	defaultConfig := algorithm.PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		NonlinearCounterfactual: true,
	}

	var thesis *types.Thesis
	var price *broker.Price
	if bus != nil {
		if shared, found := bus.Shared("thesis", ""); found {
			if t, ok := shared.(*types.Thesis); ok {
				thesis = t
			}
		}
		if shared, found := bus.Shared("price", ""); found {
			if p, ok := shared.(*broker.Price); ok {
				price = p
			}
		}
	}

	solver := &Solver{
		ctx:      ctx,
		cancel:   cancel,
		thesis:   thesis,
		price:    price,
		pearls:   &sync.Map{},
		rows:     &sync.Map{},
		config:   defaultConfig,
	}

	for _, opt := range opts {
		opt(solver)
	}

	solver.causal = runtime.ChannelOf[types.CausalOutput](
		bus, types.ChannelCausal,
		func(output types.CausalOutput) string { return output.Symbol },
	)
	solver.work = runtime.ChannelOf[types.ResonanceArtifact](
		bus, types.ChannelResonance,
		func(artifact types.ResonanceArtifact) string { return artifact.Symbol },
	).Subscribe(solver.Name(), solver.Step)

	return solver
}

func (solver *Solver) Name() string {
	return "causal"
}

func (solver *Solver) Error() error { return solver.err }

/*
Step evaluates one symbol's resonance stream through Pearl's causal ladder. The
transport workspace preserves order for this symbol while every other symbol's
lane advances concurrently, so one slow causal read never fences the universe.
*/
func (solver *Solver) Step(artifact types.ResonanceArtifact) error {
	if solver == nil || solver.thesis == nil || artifact.Manifold == nil {
		return nil
	}

	_, _, err := solver.measure(solver.thesis, artifact.Symbol, artifact.Manifold)
	solver.err = err

	if err != nil && solver.thesis != nil {
		solver.thesis.Fail(err)

		return err
	}

	// The UI frame for a causal output is published by the workspace observer on
	// ChannelCausal (boot.go); the solver only emits the domain output.
	return nil
}

func (solver *Solver) measure(
	thesis *types.Thesis,
	symbol string,
	coder *learning.ResonanceManifold,
) (types.CausalOutput, bool, error) {
	symbolValue, found := thesis.Symbols.Load(symbol)

	if !found {
		return types.CausalOutput{}, false, nil
	}

	symbolState := symbolValue.(*types.Symbol)

	forecast, err := coder.RolloutTaskForecast(1)

	if err != nil {
		return types.CausalOutput{}, false, errnie.Err(
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
		return types.CausalOutput{}, false, nil
	}

	row, rows, resolved, err := solver.observe(
		symbol,
		[3]float64{energy, surprise, prediction},
		midpoint,
		tickerAt,
		forecastReady,
	)

	if err != nil {
		return types.CausalOutput{}, false, err
	}

	if !resolved {
		return types.CausalOutput{}, false, nil
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
			return types.CausalOutput{}, false, nil
		}

		return types.CausalOutput{}, false, errnie.Err(
			errnie.UnprocessableContent,
			"causal: pearl evaluation failed: "+err.Error(),
			err,
		)
	}

	if !resolved {
		solver.storeUnresolved(symbolState, tickerAt, rows, prediction)
		return types.CausalOutput{}, false, nil
	}

	precision, err := solver.estimatePrecision(rows)

	if err != nil {
		return types.CausalOutput{}, false, errnie.Err(
			errnie.UnprocessableContent,
			"causal: precision estimation failed: "+err.Error(),
			err,
		)
	}

	causalOutput := output.Outputs()
	causalOutput["symbol"] = symbol
	causalOutput["historyRows"] = rows
	causalOutput["at"] = tickerAt
	causalOutput["identification"] = "adjustedAssociation"
	causalOutput["precision"] = precision
	causalOutput["samples"] = len(rows)
	causalOutput["treatmentLevel"] = prediction
	solver.conditionOnManifold(thesis, causalOutput)

	if solver.causal != nil {
		solver.causal.Publish(types.CausalOutput{Symbol: symbolState.Symbol, Rows: causalOutput})
	}

	return types.CausalOutput{Symbol: symbolState.Symbol, Rows: causalOutput}, true, nil
}

func (solver *Solver) conditionOnManifold(thesis *types.Thesis, causalOutput map[string]any) {
	reading, hasReading := thesis.ManifoldSnapshot()
	scores, hasScores := thesis.InterventionSnapshot()

	if !hasReading || !hasScores || len(scores) == 0 {
		return
	}

	best := scores[0]

	for _, score := range scores[1:] {
		if score.Score > best.Score {
			best = score
		}
	}

	gate := reading.KuramotoR * reading.CoherenceMag2 / (1 + math.Abs(reading.Divergence))
	uplift := best.Score * gate
	causalOutput["doExpectation"] = uplift
	causalOutput["intervention"] = uplift
	causalOutput["interventionScore"] = math.Abs(uplift)
	causalOutput["identification"] = "manifoldBVP"
	causalOutput["manifoldAction"] = best.Action
	causalOutput["treatmentLevel"] = uplift
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

	if solver.causal != nil {
		solver.causal.Publish(types.CausalOutput{Symbol: symbolState.Symbol, Rows: map[string]any{
			"symbol":         symbolState.Symbol,
			"at":             at,
			"historyRows":    rows,
			"identification": "unresolved",
			"precision":      precision,
			"samples":        len(rows),
			"treatmentLevel": prediction,
		}})
	}
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
