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
	schemaTicks   *sync.Map
	standardizers *sync.Map
	states        *sync.Map
	histories     *sync.Map
	alpha         float64
	ui            chan []byte
}

type sampleHistory struct {
	issued         map[int64]issuedTask
	pending        map[int64][]issuedHorizon
	marks          map[int64]float64
	ticks          []int64
	sequence       int64
	resolved       int
	ledger         *horizonLedger
	lastResolution *taskResolution
}

type issuedTask struct {
	features   []float64
	prediction []float64
}

type issuedHorizon struct {
	horizon  int
	forecast float64
}

type taskResolution struct {
	horizon  int
	forecast float64
	actual   float64
	error    float64
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
		schemaTicks:   &sync.Map{},
		standardizers: &sync.Map{},
		states:        &sync.Map{},
		histories:     &sync.Map{},
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
		config.Resonance.Layers <= 0 {
		return errors.New("resonance: positive layer count required")
	}

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
			markTick := int64(0)

			for rm := range symbol.ResonanceMeasurements() {
				if rm == nil {
					continue
				}

				if rm.Tick > tick {
					tick = rm.Tick
				}

				if rm.Mark > 0 && (rm.Tick > markTick || rm.Tick == markTick && mark == 0) {
					mark = rm.Mark
					markTick = rm.Tick
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
			schemaTick := tick
			frozen := false

			if rawSchemaTick, found := solver.schemaTicks.Load(name); found {
				schemaTick = rawSchemaTick.(int64)
				frozen = tick > schemaTick
			}

			for identity := range state {
				if slices.Contains(schema, identity) {
					continue
				}

				if frozen {
					continue
				}

				schema = append(schema, identity)
				schemaChanged = true
			}

			if schemaChanged {
				sort.Strings(schema)
				solver.schemas.Store(name, schema)
				solver.schemaTicks.Store(name, tick)
				solver.coders.Delete(name)
				solver.histories.Delete(name)
				symbol.Resonance.Delete(types.ResonanceReturnForecastKey)
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
				issued:  make(map[int64]issuedTask),
				pending: make(map[int64][]issuedHorizon),
				marks:   make(map[int64]float64),
				ledger:  newHorizonLedger(),
			})

			history := historyValue.(*sampleHistory)
			newObservation := false

			// 1. Process Matured Historical Targets (Delayed Training)
			if mark > 0 && tick > 0 {
				if len(history.ticks) == 0 || tick > history.ticks[len(history.ticks)-1] {
					history.ticks = append(history.ticks, tick)
					history.marks[tick] = mark
					history.sequence++
					newObservation = true
				}

				for len(history.ticks) > 1 {
					previousTick := history.ticks[0]
					previousMark := history.marks[previousTick]
					issued, found := history.issued[previousTick]
					futureTick := history.ticks[1]
					futureMark, futureFound := history.marks[futureTick]
					target := make([]float64, 1)
					complete := found && previousMark > 0

					if !futureFound || futureMark <= 0 {
						complete = false
					}

					if complete {
						target[0] = math.Log(futureMark / previousMark)

						for _, pending := range history.pending[history.sequence] {
							history.ledger.observe(
								pending.horizon,
								pending.forecast,
								target[0],
							)
							history.lastResolution = &taskResolution{
								horizon:  pending.horizon,
								forecast: pending.forecast,
								actual:   target[0],
								error:    target[0] - pending.forecast,
							}
						}

						delete(history.pending, history.sequence)
					}

					delete(history.issued, previousTick)
					delete(history.marks, previousTick)
					history.ticks = history.ticks[1:]

					if !complete {
						continue
					}

					if err := coder.ObserveTask(
						issued.features,
						issued.prediction,
						target,
					); err != nil {
						return errnie.Error(errnie.Err(
							errnie.Internal,
							fmt.Sprintf("resonance: resolve task failed [%s]", err.Error()),
							err,
						))
					}

					history.resolved++
				}
			}

			// 2. Settle and learn the CURRENT unsupervised state before forecasting.
			if _, err := coder.SettleFromBatchOptions(input, nil, true, false); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: settle failed [%s]", err.Error()),
					err,
				))
			}

			symbol.Resonance.Store(name, coder)
			symbol.Resonance.Delete(types.ResonanceReturnForecastKey)

			// 3. Extract Diagnostics & Rollout Telemetry
			layers, surprise, energy := coder.WireSnapshot()
			taskPrecision, taskPrecisionReady := coder.TaskPrecision()
			taskSkill, taskSkillReady := coder.TaskSkill()
			taskCalibration := "calibrating"
			taskSkillStatus := "calibrating"

			if taskPrecisionReady {
				taskCalibration = "calibrated"
			}

			if taskSkillReady {
				taskSkillStatus = "baseline"

				if taskSkill > 1 {
					taskSkillStatus = "above baseline"
				}

				if taskSkill < 1 {
					taskSkillStatus = "below baseline"
				}
			}
			supportedHorizon := history.ledger.supported(
				config.Planner.MinimumConfidence,
			)
			probeHorizon := supportedHorizon + 1
			forecast, err := coder.RolloutTaskForecast(probeHorizon)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: forecast rollout failed [%s]", err.Error()),
					err,
				))
			}

			if newObservation && len(forecast) > 0 {
				history.issued[tick] = issuedTask{
					features:   coder.LatentState(),
					prediction: []float64{forecast[0].Value},
				}

				for index, output := range forecast {
					if !output.Ready {
						continue
					}

					horizon := index + 1
					targetSequence := history.sequence + int64(horizon)
					history.pending[targetSequence] = append(
						history.pending[targetSequence],
						issuedHorizon{horizon: horizon, forecast: output.Value},
					)
				}
			}

			if len(forecast) > supportedHorizon {
				forecast = forecast[:supportedHorizon]
			}

			forwardCurve := coder.RolloutTaskPrediction(supportedHorizon)

			if len(forwardCurve) > supportedHorizon {
				forwardCurve = forwardCurve[:supportedHorizon]
			}

			forwardRetention := coder.RolloutRetention(supportedHorizon)

			forecastFrame := datura.NewMap(
				"posterior", forecast,
				"forwardRetention", forwardRetention,
				"supportedHorizon", supportedHorizon,
				"probeHorizon", probeHorizon,
			)
			frame := datura.NewMap(
				"source", types.SourceResonance,
				"symbol", name,
				"at", thesis.At,
				"tick", thesis.Tick,
				"taskRelativePrecision", taskPrecision,
				"taskRelativePrecisionReady", taskPrecisionReady,
				"taskCalibration", taskCalibration,
				"taskSkill", taskSkill,
				"taskSkillReady", taskSkillReady,
				"taskSkillStatus", taskSkillStatus,
				"samples", history.resolved,
				"layers", layers,
				"latent", layers[len(layers)-1].State,
				"surprise", surprise,
				"energy", energy,
				"forecast", forecastFrame,
			)

			if supportedHorizon > 0 {
				aggregate, aggregateErr := coder.RolloutTaskAggregateForecast(
					supportedHorizon,
				)

				if aggregateErr != nil {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						fmt.Sprintf("resonance: aggregate forecast failed [%s]", aggregateErr.Error()),
						aggregateErr,
					))
				}

				if len(aggregate) > 0 && aggregate[0].Ready {
					frame["taskScale"] = aggregate[0].Scale
					frame["taskForecast"] = aggregate[0].Value
					forecastFrame["aggregate"] = aggregate[0]
					symbol.Resonance.Store(
						types.ResonanceReturnForecastKey,
						&types.ResonanceReturnForecast{
							Distribution: aggregate[0],
							Horizon:      supportedHorizon,
						},
					)
				}

				forecastFrame["forwardCurve"] = forwardCurve
			}

			if history.lastResolution != nil {
				frame["lastResolvedForecast"] = history.lastResolution.forecast
				frame["lastResolvedHorizon"] = history.lastResolution.horizon
				frame["lastRealizedReturn"] = history.lastResolution.actual
				frame["lastForecastError"] = history.lastResolution.error
			}

			utils.Publish(solver.ui, datura.NewMap("resonance", frame))

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
