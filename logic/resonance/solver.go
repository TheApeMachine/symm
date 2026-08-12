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
)

const defaultHorizonConfidence = 0.85

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
	inputs   map[int64][]float64
	marks    map[int64]float64
	resolved int
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

	if system.Cfg == nil || system.Cfg.Resonance == nil ||
		system.Cfg.Resonance.Layers <= 0 {
		return errors.New("resonance: positive horizon layer count required")
	}

	maxHorizon := system.Cfg.Resonance.Layers

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

			for identity := range state {
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
				coder = learning.NewResonanceManifold(
					[]int{len(input), len(input) * 2, len(input)}, maxHorizon, solver.alpha,
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

			// 1. Process Matured Historical Targets (Delayed Training)
			if mark > 0 && tick > 0 {
				history.marks[tick] = mark
				dueTicks := make([]int64, 0)

				for previousTick := range history.inputs {
					if previousTick+int64(maxHorizon) <= tick {
						dueTicks = append(dueTicks, previousTick)
					}
				}

				slices.Sort(dueTicks)

				for _, previousTick := range dueTicks {
					previousMark := history.marks[previousTick]
					target := make([]float64, maxHorizon)
					complete := previousMark > 0

					for horizon := 1; horizon <= maxHorizon; horizon++ {
						futureMark, found := history.marks[previousTick+int64(horizon)]

						if !found || futureMark <= 0 {
							complete = false
							break
						}

						target[horizon-1] = math.Log(futureMark / previousMark)
					}

					previousInput := history.inputs[previousTick]
					delete(history.inputs, previousTick)

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

				for previousTick := range history.marks {
					if previousTick+int64(maxHorizon) < tick {
						delete(history.marks, previousTick)
					}
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

			if mark > 0 && tick > 0 {
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

			supportedHorizon, nextReach := coder.DynamicHorizon(
				defaultHorizonConfidence,
				currentReach,
				maxHorizon,
			)
			solver.currentReach.Store(name, nextReach)

			forecast, err := coder.RolloutTaskForecast(supportedHorizon)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: forecast rollout failed [%s]", err.Error()),
					err,
				))
			}

			forwardCurve := coder.RolloutTaskPrediction(supportedHorizon)
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
