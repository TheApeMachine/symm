package resonance

import (
	"context"
	"fmt"
	"maps"
	"math"
	"strings"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/learning"
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
	dynamics      *sync.Map
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
	stableCall     float64
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
		dynamics:      &sync.Map{},
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
			readings := manifoldContextReadings(thesis, name)
			mark := 0.0
			tick := thesis.Tick

			for rm := range symbol.ResonanceMeasurements() {
				if rm == nil {
					continue
				}

				if rm.Mark > 0 {
					mark = rm.Mark
				}

				maps.Copy(readings, rm.Readings)
			}

			if len(readings) == 0 && mark <= 0 {
				return nil
			}

			if tick <= 0 {
				return nil
			}

			schemaValue, loaded := solver.schemas.Load(name)

			if !loaded {
				schemaValue, _ = solver.schemas.LoadOrStore(name, taskSchema(name))
			}

			schema := schemaValue.(*featureSchema)

			stateValue, _ := solver.states.LoadOrStore(name, make(map[string]float64))
			state := stateValue.(map[string]float64)
			updated := false

			for identity, reading := range readings {
				if _, allowed := schema.known[identity]; !allowed {
					continue
				}
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

			input := make([]float64, len(schema.identities))

			for index, identity := range schema.identities {
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
			dynamicsOutput, err := solver.measurePredictiveDynamics(
				name,
				thesis,
				layers,
				readings,
				surprise,
				energy,
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("resonance: predictive dynamics failed [%s]", err.Error()),
					err,
				))
			}

			symbol.Resonance.Store(learning.PredictiveDynamicsKey, dynamicsOutput)
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
			if taskSkillReady && taskSkill >= 2.0 {
				expansion := int(math.Floor(taskSkill)) - 1

				if expansion > 3 {
					expansion = 3
				}

				supportedHorizon += expansion
			}
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
				"dynamics", predictiveDynamicsWire(dynamicsOutput),
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
					// Regulator optimization confidence governs control-space exploration. It
					// must not turn an otherwise admissible market posterior into a 95%
					// direction-switch veto.
					switchThreshold := config.Planner.MinimumConfidence

					stabilized := stabilizeDirection(
						history.stableCall,
						directionPosterior(aggregate[0]),
						config.Planner.MinimumConfidence,
						switchThreshold,
					)
					history.stableCall = stabilized.stable
					frame["taskScale"] = aggregate[0].Scale
					frame["taskForecast"] = stabilized.call
					frame["taskCandidate"] = stabilized.candidate
					frame["taskStable"] = stabilized.stable
					frame["taskHeld"] = stabilized.held
					forecastFrame["aggregate"] = aggregate[0]
					forecastFrame["candidateCall"] = stabilized.candidate
					forecastFrame["call"] = stabilized.call
					forecastFrame["stableCall"] = stabilized.stable
					forecastFrame["held"] = stabilized.held
					forecastFrame["switchConfidence"] = stabilized.confidence
					forecastFrame["switchThreshold"] = stabilized.switchThreshold
					symbol.Resonance.Store(
						types.ResonanceReturnForecastKey,
						&types.ResonanceReturnForecast{
							Distribution:     aggregate[0],
							Horizon:          supportedHorizon,
							CandidateCall:    stabilized.candidate,
							Call:             stabilized.call,
							StableCall:       stabilized.stable,
							Held:             stabilized.held,
							SwitchConfidence: stabilized.confidence,
							SwitchThreshold:  stabilized.switchThreshold,
						},
					)
					publishedForecast = true

					if stabilized.call != 0 {
						frame["taskDirection"] = stabilized.call
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

func (solver *Solver) measurePredictiveDynamics(
	name string,
	thesis *types.Thesis,
	layers []learning.ResonanceLayerWire,
	readings map[string]float64,
	surprise float64,
	energy float64,
) (nomagique.Frame, error) {
	if solver.dynamics == nil {
		solver.dynamics = &sync.Map{}
	}

	observedAt := float64(thesis.At.Unix()) +
		float64(thesis.At.Nanosecond())/float64(1_000_000_000)

	if thesis.At.IsZero() {
		observedAt = float64(thesis.Tick)
	}

	input := nomagique.Frame{}
	input.Put(learning.SymbolDynamicsTime, observedAt)
	input.Put(learning.SymbolDynamicsPosition, predictiveLatentPosition(layers))
	input.Put(
		learning.SymbolDynamicsActivity,
		predictiveActivity(readings, surprise),
	)
	input.Put(
		learning.SymbolDynamicsExternalPower,
		predictiveExternalPower(readings, energy),
	)

	if phase, found := predictivePhase(readings); found {
		input.Put(learning.SymbolDynamicsPhase, phase)
	}

	streamValue, _ := solver.dynamics.LoadOrStore(
		name,
		nomagique.NewStream(learning.PredictiveDynamics, nomagique.Frame{}),
	)
	stream, ok := streamValue.(*nomagique.Stream)

	if !ok || stream == nil {
		return nomagique.Frame{}, fmt.Errorf(
			"resonance: predictive dynamics stream unavailable for %s",
			name,
		)
	}

	return stream.Step(input)
}

func predictiveLatentPosition(layers []learning.ResonanceLayerWire) float64 {
	if len(layers) == 0 {
		return 0
	}

	coordinates := layers[len(layers)-1].State

	if len(coordinates) == 0 {
		return 0
	}

	position := 0.0

	for _, coordinate := range coordinates {
		position += coordinate
	}

	return position / float64(len(coordinates))
}

func predictiveActivity(
	readings map[string]float64,
	surprise float64,
) float64 {
	activity := math.Abs(surprise)

	for identity, reading := range readings {
		normalized := strings.ToLower(identity)

		if strings.Contains(normalized, "spectral_radius") ||
			strings.Contains(normalized, "arrival_rate") ||
			strings.Contains(normalized, "urgency") ||
			strings.Contains(normalized, "branching") {
			activity += math.Abs(reading)
		}
	}

	return activity
}

func predictiveExternalPower(
	readings map[string]float64,
	energy float64,
) float64 {
	power := 0.0
	contributors := 0

	for identity, reading := range readings {
		normalized := strings.ToLower(identity)

		if strings.Contains(normalized, "net_fraction") ||
			strings.Contains(normalized, "signed") ||
			strings.Contains(normalized, "guidance_speed") {
			power += reading
			contributors++
		}
	}

	if contributors == 0 {
		return 0
	}

	return power * energy / float64(contributors)
}

func predictivePhase(readings map[string]float64) (float64, bool) {
	for identity, reading := range readings {
		if strings.Contains(strings.ToLower(identity), "phase") {
			return reading, true
		}
	}

	return 0, false
}

func predictiveDynamicsWire(frame nomagique.Frame) map[string]float64 {
	fields := []struct {
		name   string
		symbol nomagique.Symbol
	}{
		{name: "ready", symbol: learning.SymbolDynamicsReady},
		{name: "deltaTime", symbol: learning.SymbolDynamicsDeltaTime},
		{name: "position", symbol: learning.SymbolDynamicsPosition},
		{name: "velocity", symbol: learning.SymbolDynamicsVelocity},
		{name: "acceleration", symbol: learning.SymbolDynamicsAcceleration},
		{name: "memory", symbol: learning.SymbolDynamicsMemory},
		{name: "memoryScale", symbol: learning.SymbolDynamicsMemoryScale},
		{name: "storedEnergy", symbol: learning.SymbolDynamicsStoredEnergy},
		{name: "suppliedPower", symbol: learning.SymbolDynamicsSuppliedPower},
		{name: "dissipation", symbol: learning.SymbolDynamicsDissipation},
		{name: "passivityResidue", symbol: learning.SymbolDynamicsPassivityResidue},
		{name: "continuousVariance", symbol: learning.SymbolDynamicsContinuousVariance},
		{name: "jumpAmplitude", symbol: learning.SymbolDynamicsJumpAmplitude},
		{name: "jumpVariance", symbol: learning.SymbolDynamicsJumpVariance},
		{name: "sampleCount", symbol: learning.SymbolDynamicsSampleCount},
		{name: "rotorScalar", symbol: learning.SymbolDynamicsRotorScalar},
		{name: "rotorBivector", symbol: learning.SymbolDynamicsRotorBivector},
		{name: "equivarianceNorm", symbol: learning.SymbolDynamicsEquivarianceNorm},
	}
	wire := make(map[string]float64, len(fields))

	for _, field := range fields {
		value, found := frame.Get(field.symbol)

		if found {
			wire[field.name] = value
		}
	}

	return wire
}
