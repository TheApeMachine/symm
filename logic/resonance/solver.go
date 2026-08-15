package resonance

import (
	"context"
	"errors"
	"fmt"
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
	moves          *adaptive.Accumulator
	moveStat       adaptive.AccumulatorOutput
	lastTickMark   float64
	sequencedMark  float64
}

type issuedTask struct {
	features   []float64
	prediction []float64
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
		standardizers: &sync.Map{},
		states:        &sync.Map{},
		histories:     &sync.Map{},
		alpha:         initialAlpha,
		ui:            ui,
	}
}

func (solver *Solver) Name() string {
	return "resonance"
}

/*
Update settles one predictive-coding manifold for every symbol carrying finite,
normalized measurements and publishes the resulting hierarchy.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if solver.alpha <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: positive learning pace required",
			nil,
		))
	}

	if err := errnie.Require(map[string]any{
		"alpha":     solver.alpha,
		"thesis":    thesis,
		"config":    system.Cfg,
		"resonance": system.Cfg.Resonance,
		"planner":   system.Cfg.Planner,
		"layers":    system.Cfg.Resonance.Layers,
	}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.PreconditionFailed,
			"resonance: thesis required",
			nil,
		))
	}

	config := system.Cfg.Snapshot()
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
			tick := thesis.Tick

			for rm := range symbol.ResonanceMeasurements() {
				if rm == nil {
					continue
				}

				if rm.Mark > 0 {
					mark = rm.Mark
				}

				for identity, value := range rm.Readings {
					readings[identity] = value
				}
			}

			if len(readings) == 0 && mark <= 0 {
				return nil
			}

			if tick <= 0 {
				return errors.New("resonance: positive analysis tick required")
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

			if !updated && (mark <= 0 || len(state) == 0) {
				return nil
			}

			var schema []string

			if rawSchema, loaded := solver.schemas.Load(name); loaded {
				schema = rawSchema.([]string)
			}

			historyValue, _ := solver.histories.LoadOrStore(name, &sampleHistory{
				issued:  make(map[int64]issuedTask),
				pending: make(map[int64][]issuedHorizon),
				marks:   make(map[int64]float64),
				ledger:  newHorizonLedger(),
				moves:   adaptive.NewAccumulator(),
			})
			history := historyValue.(*sampleHistory)
			var coder *learning.ResonanceManifold

			if rawCoder, loaded := solver.coders.Load(name); loaded {
				coder = rawCoder.(*learning.ResonanceManifold)
			}

			newObservation := false

			if mark > 0 && tick > 0 {
				if len(history.ticks) == 0 || tick > history.ticks[len(history.ticks)-1] {
					previousMark := history.lastTickMark

					if err := history.observeTickMove(mark); err != nil {
						return err
					}

					if previousMark <= 0 || mark != previousMark {
						history.ticks = append(history.ticks, tick)
						history.marks[tick] = mark
						history.sequence++
						history.sequencedMark = mark
						newObservation = true
					}
				}

				if newObservation && coder != nil {
					if err := history.resolve(coder, mark); err != nil {
						return err
					}
				}

				history.pruneTicks()
			}

			schemaChanged := false

			for identity := range state {
				if slices.Contains(schema, identity) {
					continue
				}

				if coder != nil && history.inFlight() {
					continue
				}

				schema = append(schema, identity)
				schemaChanged = true
			}

			if schemaChanged {
				sort.Strings(schema)
				solver.schemas.Store(name, schema)
				solver.coders.Delete(name)
				coder = nil
				history.issued = make(map[int64]issuedTask)
				history.pending = make(map[int64][]issuedHorizon)
				history.ledger = newHorizonLedger()
				symbol.Resonance.Delete(types.ResonanceReturnForecastKey)
			}

			input := make([]float64, len(schema))

			for index, identity := range schema {
				input[index] = state[identity]
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

			// 2. Settle and learn the CURRENT unsupervised state before forecasting.
			if _, err := coder.SettleFromBatchOptions(input, nil, true, false); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: settle failed [%s]", err.Error()),
					err,
				))
			}

			symbol.Resonance.Store(name, coder)

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

			if newObservation {
				if err := history.issue(
					coder,
					tick,
					mark,
					probeHorizon,
					forecast,
				); err != nil {
					return err
				}
			}

			if len(forecast) > supportedHorizon {
				forecast = forecast[:supportedHorizon]
			}

			forwardRetention := coder.RolloutRetention(supportedHorizon)

			leans := make([]float64, len(forecast))

			for index, step := range forecast {
				leans[index] = step.Value
			}

			forecastFrame := datura.NewMap(
				"posterior", forecast,
				"forwardCurve", leans,
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

			publishedForecast := false

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

				if len(aggregate) > 0 {
					call := directionCall(
						aggregate[0],
						config.Planner.MinimumConfidence,
					)
					frame["taskScale"] = aggregate[0].Scale
					frame["taskForecast"] = call
					forecastFrame["aggregate"] = aggregate[0]
					forecastFrame["call"] = call
					symbol.Resonance.Store(
						types.ResonanceReturnForecastKey,
						&types.ResonanceReturnForecast{
							Distribution: aggregate[0],
							Horizon:      supportedHorizon,
							Call:         call,
						},
					)
					publishedForecast = true

					if call != 0 {
						frame["taskDirection"] = call
					}
				}
			}

			if !publishedForecast {
				symbol.Resonance.Delete(types.ResonanceReturnForecastKey)
			}

			if history.lastResolution != nil {
				frame["lastResolvedForecast"] = signedDirection(
					history.lastResolution.forecast,
				)
				frame["lastResolvedHorizon"] = history.lastResolution.horizon
				frame["lastRealizedReturn"] = signedDirection(
					history.lastResolution.actual,
				)
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
