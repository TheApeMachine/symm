package resonance

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Solver feeds normalized market measurements into one resonance manifold per symbol.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	recorder      *audit.Recorder
	coders        *sync.Map
	schemas       *sync.Map
	previousInput *sync.Map
	previousMark  *sync.Map
	previousTick  *sync.Map
	currentReach  *sync.Map
	alpha         float64
	ui            chan []byte
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
		ctx:           ctx,
		cancel:        cancel,
		recorder:      recorder,
		coders:        &sync.Map{},
		schemas:       &sync.Map{},
		previousInput: &sync.Map{},
		previousMark:  &sync.Map{},
		previousTick:  &sync.Map{},
		currentReach:  &sync.Map{},
		alpha:         initialAlpha,
		ui:            ui,
	}
}

/*
Update settles one predictive-coding manifold for every symbol carrying finite,
normalized measurements and publishes the resulting hierarchy.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if solver.alpha <= 0 {
		return errors.New("resonance: positive learning pace required")
	}

	var updateErr error

	thesis.Symbols.Range(func(key, value any) bool {
		name := key.(string)
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		readings := make(map[string]float64)
		mark := 0.0
		tick := int64(0)

		for measurement := range symbol.MarketMeasurements("resonance") {
			if measurement.Symbol != name {
				continue
			}

			if measurement.Tick > tick || measurement.Tick == tick && mark == 0 {
				if bid, hasBid := measurement.Metadata["bid"]; hasBid {
					if ask, hasAsk := measurement.Metadata["ask"]; hasAsk && bid > 0 && ask > 0 {
						mark = (bid + ask) / 2
						tick = measurement.Tick
					}
				}

				if mark == 0 {
					if value, found := measurement.Metadata["last_price"]; found && value > 0 {
						mark = value
						tick = measurement.Tick
					}
				}

				if mark == 0 {
					if value, found := measurement.Metadata["trade_price"]; found && value > 0 {
						mark = value
						tick = measurement.Tick
					}
				}
			}

			for key, sample := range measurement.Metrics {
				if sample.Normalized == nil {
					continue
				}

				identity := string(measurement.Source) + ":" + name + ":" + key
				readings[identity] = *sample.Normalized
			}
		}

		if len(readings) == 0 {
			return true
		}

		var schema []string
		if rawSchema, loaded := solver.schemas.Load(name); loaded {
			schema = rawSchema.([]string)
		}

		schemaChanged := false
		for identity := range readings {
			if slices.Contains(schema, identity) {
				continue
			}

			schema = append(schema, identity)
			schemaChanged = true
		}

		if schemaChanged {
			sort.Strings(schema)
			solver.schemas.Store(name, schema)
			solver.coders.Delete(name)
			solver.previousInput.Delete(name)
			solver.previousMark.Delete(name)
			solver.previousTick.Delete(name)
			solver.currentReach.Delete(name)
		}

		input := make([]float64, len(schema))
		for index, identity := range schema {
			input[index] = readings[identity]
		}

		var coder *learning.ResonanceManifold
		rawCoder, loaded := solver.coders.Load(name)

		if loaded {
			coder = rawCoder.(*learning.ResonanceManifold)
		}

		if coder == nil {
			coder = learning.NewResonanceManifold(
				[]int{len(input), len(input) * 2, len(input)}, 1, solver.alpha,
			)

			if coder == nil {
				updateErr = errors.New("resonance: failed to construct manifold")
				return false
			}

			solver.coders.Store(name, coder)
		}

		if previousInput, found := solver.previousInput.Load(name); found && mark > 0 {
			previousTick, _ := solver.previousTick.Load(name)

			if tick == previousTick.(int64)+1 {
				previousMark, _ := solver.previousMark.Load(name)
				target := math.Log(mark / previousMark.(float64))

				if _, err := coder.SettleFromBatchOptions(
					previousInput.([]float64), []float64{target}, true, false,
				); err != nil {
					updateErr = err
					return false
				}
			}
		}

		if _, err := coder.SettleFromBatchOptions(input, nil, false, true); err != nil {
			updateErr = err
			return false
		}

		if mark > 0 && tick > 0 {
			solver.previousInput.Store(name, slices.Clone(input))
			solver.previousMark.Store(name, mark)
			solver.previousTick.Store(name, tick)
		}

		symbol.Resonance.Store(name, coder)
		layers, surprise, energy := coder.WireSnapshot()
		taskPrecision, taskPrecisionReady := coder.TaskPrecision()
		taskSkill, taskSkillReady := coder.TaskSkill()
		currentReach := 1

		if storedReach, found := solver.currentReach.Load(name); found {
			currentReach = storedReach.(int)
		}

		supportedHorizon, nextReach := coder.DynamicHorizon(
			taskPrecision,
			currentReach,
			system.Cfg.Resonance.Layers,
		)
		solver.currentReach.Store(name, nextReach)
		forecast, err := coder.RolloutTaskForecast(supportedHorizon)

		if err != nil {
			updateErr = err
			return false
		}

		forwardCurve := coder.RolloutTaskPrediction(supportedHorizon)
		forwardRetention := coder.RolloutRetention(supportedHorizon)

		if len(forwardCurve) > supportedHorizon {
			forwardCurve = forwardCurve[:supportedHorizon]
		}

		if len(forwardRetention) > supportedHorizon {
			forwardRetention = forwardRetention[:supportedHorizon]
		}

		utils.Publish(solver.ui, datura.NewMap("resonance", datura.NewMap(
			"source", types.SourceResonance,
			"symbol", name,
			"at", thesis.At,
			"tick", thesis.Tick,
			"taskPrecision", taskPrecision,
			"taskPrecisionReady", taskPrecisionReady,
			"taskSkill", taskSkill,
			"taskSkillReady", taskSkillReady,
			"taskScale", forecast[0].Scale,
			"taskForecast", forwardCurve[0],
			"layers", layers,
			"latent", layers[len(layers)-1].State,
			"surprise", surprise,
			"energy", energy,
			"forecast", datura.NewMap(
				"posterior", forecast,
				"forwardCurve", forwardCurve,
				"forwardRetention", forwardRetention,
				"supportedHorizon", supportedHorizon,
				"currentReach", currentReach,
				"nextReach", nextReach,
			),
		)))

		return true
	})

	return updateErr
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
