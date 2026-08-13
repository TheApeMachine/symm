package resonance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
	"gonum.org/v1/gonum/stat/distuv"
)

/*
Solver feeds adaptively standardized market measurements into one resonance
manifold per symbol.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	recorder      *audit.Recorder
	coders        *sync.Map
	schemas       *sync.Map
	standardizers *sync.Map
	states        *sync.Map
	histories     *sync.Map
	currentReach  *sync.Map
	alpha         float64
	ui            chan []byte
}

type sampleHistory struct {
	inputs           map[int64][]float64
	marks            map[int64]float64
	ticks            []int64
	resolved         int
	supportedHorizon int
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
		standardizers: &sync.Map{},
		states:        &sync.Map{},
		histories:     &sync.Map{},
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

	config := system.Cfg.Snapshot()

	if config == nil || config.Resonance == nil || config.Planner == nil ||
		config.Resonance.Layers <= 0 || config.Resonance.MaxHorizon <= 0 {
		return errors.New("resonance: positive horizon layer count required")
	}

	maxHorizon := config.Resonance.MaxHorizon

	group, _ := errgroup.WithContext(solver.ctx)

	thesis.Symbols.Range(func(key, value any) bool {
		name := key.(string)
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		group.Go(func() error {
			readings := make(map[string]float64)
			mark := 0.0
			tick := int64(0)

			for rm := range symbol.ResonanceMeasurements() {
				if rm == nil {
					continue
				}

				if rm.Tick > tick || (rm.Tick == tick && mark == 0) {
					if rm.Mark > 0 {
						mark = rm.Mark
						tick = rm.Tick
					}
				}

				for identity, value := range rm.Readings {
					readings[identity] = value
				}
			}

			if len(readings) == 0 {
				return nil
			}

			stateValue, _ := solver.states.LoadOrStore(name, make(map[string]float64))
			state := stateValue.(map[string]float64)
			updated := false

			for identity, reading := range readings {
				rawStandardizer, found := solver.standardizers.Load(identity)

				if !found {
					rawStandardizer = adaptive.NewStandardizer()
					solver.standardizers.Store(identity, rawStandardizer)
				}

				standardized, err := rawStandardizer.(*adaptive.Standardizer).Measure(reading)

				if err != nil {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						fmt.Sprintf("resonance: standardize %s: %s", identity, err.Error()),
						err,
					))
				}

				if !standardized.Ready {
					continue
				}

				state[identity] = standardized.Value
				updated = true
			}

			if !updated {
				return nil
			}

			var schema []string

			if rawSchema, loaded := solver.schemas.Load(name); loaded {
				schema = rawSchema.([]string)
			}

			schemaChanged := false
			trained := false

			if rawHistory, found := solver.histories.Load(name); found {
				trained = rawHistory.(*sampleHistory).resolved > 0
			}

			for identity := range state {
				if slices.Contains(schema, identity) {
					continue
				}

				if trained {
					continue
				}

				schema = append(schema, identity)
				schemaChanged = true
			}

			if schemaChanged {
				sort.Strings(schema)
				solver.schemas.Store(name, schema)
				solver.coders.Delete(name)
				solver.histories.Delete(name)
				solver.currentReach.Delete(name)
			}

			input := make([]float64, len(schema))

			for index, identity := range schema {
				input[index] = state[identity]
			}

			var coder *learning.ResonanceManifold
			rawCoder, loaded := solver.coders.Load(name)

			if loaded {
				coder = rawCoder.(*learning.ResonanceManifold)
			}

			if coder == nil {
				architecture := make([]int, config.Resonance.Layers)

				for layer := range architecture {
					architecture[layer] = len(input)

					if layer > 0 && layer+1 < len(architecture) {
						architecture[layer] = len(input) * 2
					}
				}

				coder = learning.NewResonanceManifold(
					architecture, 1, solver.alpha,
				)

				if coder == nil {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"resonance: failed to create manifold",
						nil,
					))
				}

				solver.coders.Store(name, coder)
			}

			historyValue, _ := solver.histories.LoadOrStore(name, &sampleHistory{
				inputs: make(map[int64][]float64),
				marks:  make(map[int64]float64),
			})

			history := historyValue.(*sampleHistory)
			resolvedBefore := history.resolved

			// 1. Process Matured Historical Targets (Delayed Training)
			if mark > 0 && tick > 0 {
				if len(history.ticks) == 0 || tick > history.ticks[len(history.ticks)-1] {
					history.ticks = append(history.ticks, tick)
					history.marks[tick] = mark
				}

				for len(history.ticks) > 1 {
					previousTick := history.ticks[0]
					previousMark := history.marks[previousTick]
					previousInput, found := history.inputs[previousTick]
					futureTick := history.ticks[1]
					futureMark, futureFound := history.marks[futureTick]
					target := make([]float64, 1)
					complete := found && previousMark > 0

					if !futureFound || futureMark <= 0 {
						complete = false
					}

					if complete {
						target[0] = math.Log(futureMark / previousMark)
					}

					delete(history.inputs, previousTick)
					delete(history.marks, previousTick)
					history.ticks = history.ticks[1:]

					if !complete {
						continue
					}

					if _, err := coder.SettleFromBatchOptions(
						previousInput, target, true, false,
					); err != nil {
						return errnie.Error(errnie.Err(
							errnie.Internal,
							fmt.Sprintf("resonance: settle failed [%s]", err.Error()),
							err,
						))
					}

					history.resolved++
				}
			}

			// 2. Perform Generative Settle on CURRENT Tick Input for Inference
			if _, err := coder.SettleFromBatchOptions(input, nil, false, true); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: settle failed [%s]", err.Error()),
					err,
				))
			}

			if _, found := history.marks[tick]; found {
				history.inputs[tick] = slices.Clone(input)
			}

			symbol.Resonance.Store(name, coder)

			// 3. Extract Diagnostics & Rollout Telemetry
			layers, surprise, energy := coder.WireSnapshot()
			taskPrecision, taskPrecisionReady := coder.TaskPrecision()
			taskSkill, taskSkillReady := coder.TaskSkill()
			currentReach := 1

			if storedReach, found := solver.currentReach.Load(name); found {
				currentReach = storedReach.(int)
			}

			supportedHorizon := history.supportedHorizon
			nextReach := currentReach

			if supportedHorizon == 0 {
				supportedHorizon = 1
			}

			forecast, err := coder.RolloutTaskForecast(maxHorizon)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: forecast rollout failed [%s]", err.Error()),
					err,
				))
			}

			if history.resolved > resolvedBefore {
				index := min(supportedHorizon, len(forecast)) - 1
				confidence := 0.0

				if index >= 0 && forecast[index].Ready {
					distribution := distuv.StudentsT{
						Mu:    forecast[index].Value,
						Sigma: forecast[index].Scale,
						Nu:    forecast[index].DegreesOfFreedom,
					}
					confidence = 1 - distribution.CDF(0)

					if forecast[index].Value < 0 {
						confidence = distribution.CDF(0)
					}
				}

				if index >= 0 && forecast[index].Ready &&
					confidence >= config.Planner.MinimumConfidence {
					supportedHorizon = min(maxHorizon, supportedHorizon+1)
				} else {
					supportedHorizon = max(1, supportedHorizon-1)
				}

				nextReach = supportedHorizon
				history.supportedHorizon = supportedHorizon
				solver.currentReach.Store(name, supportedHorizon)
			}

			if len(forecast) > supportedHorizon {
				forecast = forecast[:supportedHorizon]
			}

			forwardCurve := coder.RolloutTaskPrediction(maxHorizon)

			if len(forwardCurve) > supportedHorizon {
				forwardCurve = forwardCurve[:supportedHorizon]
			}

			forwardRetention := coder.RolloutRetention(supportedHorizon)

			// Safe guards against empty predictions during cold start
			var taskScale float64

			if len(forecast) > 0 {
				taskScale = forecast[0].Scale
			}

			var taskForecast float64

			if len(forwardCurve) > 0 {
				taskForecast = forwardCurve[0]
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
				"taskScale", taskScale,
				"taskForecast", taskForecast,
				"samples", history.resolved,
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

			return nil
		})

		return true
	})

	if err := group.Wait(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: solver update failed [%s]", err.Error()),
			err,
		))
	}

	return nil
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
