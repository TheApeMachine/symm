package resonance

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
Solver feeds normalized market measurements into one resonance manifold per symbol.
*/
type Solver struct {
	ctx      context.Context
	cancel   context.CancelFunc
	recorder *audit.Recorder
	coders   *sync.Map
	samples  *sync.Map
	reach    *sync.Map
	schemas  *sync.Map
	alpha    float64
	ui       chan []byte
}

/*
NewSolver returns a predictive-coding solver using the configured learning pace.
*/
func NewSolver(
	ctx context.Context,
	ui chan []byte,
	recorder *audit.Recorder,
	initialAlpha float64,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	if initialAlpha == 0 && system.Cfg != nil && system.Cfg.Resonance != nil {
		initialAlpha = system.Cfg.Resonance.LearningRate
	}

	return &Solver{
		ctx:      ctx,
		cancel:   cancel,
		recorder: recorder,
		coders:   &sync.Map{},
		samples:  &sync.Map{},
		reach:    &sync.Map{},
		schemas:  &sync.Map{},
		alpha:    initialAlpha,
		ui:       ui,
	}
}

/*
Update settles one predictive-coding manifold for every symbol carrying finite,
normalized measurements and publishes the resulting hierarchy.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if !(solver.alpha > 0) || solver.alpha > 1 || math.IsNaN(solver.alpha) || math.IsInf(solver.alpha, 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: alpha must be finite and in (0, 1]",
			errors.New("invalid resonance alpha"),
		))
	}

	features := make(map[string]map[string]float64)

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol.Stamped(types.SourceResonance) || len(symbol.Measurements) == 0 {
			return true
		}

		name := key.(string)
		features[name] = make(map[string]float64)

		for _, measurement := range symbol.Measurements {
			for key, sample := range measurement.Metrics {
				if sample.Normalized == nil {
					continue
				}

				identity := string(measurement.Source) + ":" + name + ":" + key
				features[name][identity] = *sample.Normalized
			}
		}

		return true
	})

	if len(features) == 0 {
		return nil
	}

	group, _ := errgroup.WithContext(solver.ctx)

	for symbolName, readings := range features {
		group.Go(func() error {
			var masterSchema []string

			if rawSchema, loaded := solver.schemas.Load(symbolName); loaded {
				masterSchema = rawSchema.([]string)
			}

			schemaMap := make(map[string]bool, len(masterSchema))

			for _, id := range masterSchema {
				schemaMap[id] = true
			}

			schemaUpdated := false

			for identity := range readings {
				if !schemaMap[identity] {
					masterSchema = append(masterSchema, identity)
					schemaMap[identity] = true
					schemaUpdated = true
				}
			}

			if schemaUpdated || len(masterSchema) == 0 {
				sort.Strings(masterSchema)
				solver.schemas.Store(symbolName, masterSchema)
			}

			inputDim := len(masterSchema)

			if inputDim == 0 {
				reading := types.ResonanceReading{
					Stage:  "resonance",
					Source: types.SourceResonance,
					Symbol: symbolName,
					At:     thesis.At,
					Alpha:  solver.alpha,
					Verdict: types.ResonanceVerdict{
						Learning: "observing",
					},
				}
				stored, _ := thesis.Symbols.Load(symbolName)
				symbol := stored.(*types.Symbol)
				symbol.Resonance.Store(symbolName, reading)
				thesis.Stamp(symbolName, types.SourceResonance)
				utils.Publish(
					solver.ui,
					datura.NewMap("resonance", reading),
				)
				return nil
			}

			input := make([]float64, inputDim)

			for index, identity := range masterSchema {
				if val, found := readings[identity]; found {
					input[index] = val
				}
			}

			found, ok := solver.coders.Load(symbolName)
			var coder *learning.ResonanceManifold

			if ok && found != nil {
				if existingCoder, valid := found.(*learning.ResonanceManifold); valid {
					layers, _, _ := existingCoder.WireSnapshot()

					if len(layers) > 0 && len(layers[0].State) == inputDim {
						coder = existingCoder
					}
				}
			}

			if coder == nil {
				coder = learning.NewResonanceManifold(
					[]int{inputDim, inputDim * 2, inputDim}, 1, solver.alpha,
				)

				if coder == nil {
					thesis.Stamp(symbolName, types.SourceResonance)
					return errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"resonance: failed to construct predictive coding manifold",
						errors.New("invalid resonance manifold"),
					))
				}

				solver.coders.Store(symbolName, coder)
			}

			if _, err := coder.SettleFromBatch(input, nil); err != nil {
				thesis.Stamp(symbolName, types.SourceResonance)
				return err
			}

			var count uint64
			if val, loaded := solver.samples.Load(symbolName); loaded {
				count = val.(uint64) + 1
			} else {
				count = 1
			}
			solver.samples.Store(symbolName, count)

			layers, surprise, energy := coder.WireSnapshot()
			latent := coder.LatentState()

			var embedding []float64
			if len(latent) >= 2 {
				embedding = latent[:2]
			} else {
				embedding = latent
			}

			forecast, verdict := solver.buildForecast(coder, symbolName)

			skillEvidence := 0.0
			if skill, ok := coder.TaskSkill(); ok {
				skillEvidence = skill
			}

			reading := types.ResonanceReading{
				Stage:         "resonance",
				Source:        types.SourceResonance,
				Symbol:        symbolName,
				At:            thesis.At,
				Surprise:      surprise,
				Energy:        energy,
				Latent:        latent,
				Embedding:     embedding,
				Layers:        layers,
				Forecast:      forecast,
				Verdict:       verdict,
				Alpha:         solver.alpha,
				Samples:       count,
				SkillEvidence: skillEvidence,
			}

			stored, _ := thesis.Symbols.Load(symbolName)
			symbol := stored.(*types.Symbol)
			symbol.Resonance.Store(symbolName, reading)
			thesis.Stamp(symbolName, types.SourceResonance)

			utils.Publish(
				solver.ui,
				datura.NewMap("resonance", reading),
			)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: failed to update CPU predictive coding manifold: "+err.Error(),
			err,
		))
	}

	return nil
}

/*
buildForecast queries nomagique's task readiness and rollout methods.
If nomagique reports that the task head is not ready (TaskSkill or TaskPrecision missing, or RLS unready),
it returns an observing verdict and a nil forecast without fabricating confidence metrics.
*/
func (solver *Solver) buildForecast(
	coder *learning.ResonanceManifold, symbol string,
) (*types.ResonanceForecast, types.ResonanceVerdict) {
	verdict := types.ResonanceVerdict{
		Learning:       "observing",
		Tuning:         "recursive least squares",
		LearningHealth: 0,
		TuningHealth:   1,
	}

	skill, hasSkill := coder.TaskSkill()
	precision, hasPrecision := coder.TaskPrecision()

	if !hasSkill || !hasPrecision {
		return nil, verdict
	}

	currentReach := 1
	if val, loaded := solver.reach.Load(symbol); loaded {
		currentReach = val.(int)
	}

	horizon, newReach := coder.DynamicHorizon(precision, currentReach, 8)
	solver.reach.Store(symbol, newReach)

	rlsOutputs, err := coder.RolloutTaskForecast(horizon)

	if err != nil || len(rlsOutputs) < horizon || !rlsOutputs[0].Ready {
		return nil, verdict
	}

	predictions := coder.RolloutTaskPrediction(horizon)

	if len(predictions) < horizon {
		return nil, verdict
	}

	retention := coder.RolloutRetention(horizon)

	if len(retention) < horizon {
		return nil, verdict
	}

	rls := rlsOutputs[0]
	fc, err := types.NewResonanceForecast(
		predictions[:horizon], retention[:horizon], horizon, skill,
	)

	if err != nil {
		return nil, verdict
	}

	if err := fc.SetPredictiveDistribution(rls.Scale, rls.DegreesOfFreedom, rls.Ready); err != nil {
		return nil, verdict
	}

	verdict.Learning = "predicting"
	verdict.LearningHealth = 1
	verdict.Conviction = skill

	if fc.ExpectedReturn > 0 {
		verdict.Direction = 1
	}

	if fc.ExpectedReturn < 0 {
		verdict.Direction = -1
	}

	return fc, verdict
}

/*
Close stops the solver context.
*/
func (solver *Solver) Close() error {
	if solver.cancel != nil {
		solver.cancel()
	}

	return nil
}
