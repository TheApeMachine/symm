package resonance

import (
	"math"
	"runtime"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

const resonanceReturnDimensions = 1

/*
Solver wraps the CPU Predictive Coding Resonance Manifold (`learning.ResonanceManifold`).
It accepts physics/field metrics from the upstream manifold solver, computes prediction
errors (Surprise), settles top-down/bottom-up latent states, dynamically adapts alpha,
and extends forward prediction horizons dynamically based on confidence.
*/
type Solver struct {
	recorder        *audit.Recorder
	states          map[string]*symbolState
	arch            []int
	targetDim       int
	maxHorizon      int // Maximum forward prediction horizon (e.g. 20 ticks)
	learn           bool
	advanceTemporal bool
	ui              chan []byte
}

/*
NewSolver returns a new predictive coding solver with dynamic alpha control and
dynamic forward prediction rollout.
Defaults: initial alpha = 0.03, maxHorizon = 20 ticks.
*/
func NewSolver(ui chan []byte, recorder *audit.Recorder) *Solver {
	solver := &Solver{
		recorder:        recorder,
		states:          make(map[string]*symbolState),
		targetDim:       resonanceReturnDimensions,
		maxHorizon:      20, // Can extend up to 20 ticks ahead when confidence is high
		learn:           true,
		advanceTemporal: true,
		ui:              ui,
	}

	return solver
}

func (solver *Solver) state(symbol string) *symbolState {
	state, ok := solver.states[symbol]

	if ok {
		return state
	}

	state = newSymbolState(restAlpha)
	solver.states[symbol] = state

	return state
}

/*
Update extracts physical/field feature vectors from the Thesis, settles the CPU predictive
coding manifold, dynamically tunes alpha and forward prediction horizon, and enriches the Thesis.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	featureSets := solver.extractFeatures(thesis)
	thesis.Resonance.Clear()

	if len(featureSets) == 0 {
		thesis.Readiness.Resonance = true
		return nil
	}

	for symbol := range featureSets {
		solver.state(symbol)
	}

	group := errgroup.Group{}
	group.SetLimit(max(runtime.NumCPU(), 1))

	for symbol, features := range featureSets {
		group.Go(func() error {
			return solver.updateSymbol(thesis, symbol, features)
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	thesis.Readiness.Resonance = true

	if solver.ui == nil {
		return nil
	}

	focused, found := thesis.Resonance.Load(types.Focus())

	if found {
		utils.Publish(solver.ui, datura.NewMap("resonance", []any{focused}))
	}

	return nil
}

func (solver *Solver) updateSymbol(
	thesis *types.Thesis,
	symbol string,
	features map[string]float64,
) error {
	if len(features) == 0 {
		return nil
	}

	state := solver.state(symbol)
	features = solver.standardizeFeatures(state, features)

	if state.targetSamples == 0 {
		featureSchema := append([]string(nil), state.featureSchema...)
		knownFeatures := make(map[string]struct{}, len(featureSchema))

		for _, featureKey := range featureSchema {
			knownFeatures[featureKey] = struct{}{}
		}

		for featureKey := range features {
			if _, known := knownFeatures[featureKey]; known {
				continue
			}

			featureSchema = append(featureSchema, featureKey)
		}

		sort.Strings(featureSchema)

		if state.manifold == nil || len(featureSchema) != len(state.featureSchema) {
			if err := solver.initManifold(state, len(featureSchema)); err != nil {
				return errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"resonance: failed to initialize CPU predictive coding manifold",
					err,
				))
			}

			state.featureSchema = featureSchema
			state.pendingInput = nil
			state.pendingMid = 0
			state.pendingAt = time.Time{}
			state.alphaCtrl = NewAlphaController(state.alpha, minAlpha, maxAlpha)
			state.confidence = newErrorCalibrator()
			state.horizonReach = 1
		}
	}

	input := state.input

	if cap(input) < len(state.featureSchema) {
		input = make([]float64, len(state.featureSchema))
	} else {
		input = input[:len(state.featureSchema)]
	}

	state.input = input

	for featureIndex, featureKey := range state.featureSchema {
		input[featureIndex] = features[featureKey]
	}

	midpoint, targetAt, targetReady := solver.returnTarget(thesis, symbol)

	if targetReady && solver.learn {
		if err := solver.learnReturn(state, midpoint, targetAt); err != nil {
			return err
		}
	}

	totalSurprise, err := state.manifold.SettleFromBatchOptions(
		input,
		nil,
		solver.learn,
		solver.advanceTemporal && !solver.learn,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: CPU predictive coding settle failed",
			err,
		))
	}

	featureCount := float64(len(input))
	surprise := totalSurprise / math.Sqrt(featureCount)
	energy := state.manifold.PredictionEnergy() / featureCount
	temporalError, hasTemporal := state.manifold.TemporalError()
	latentDimension := len(state.featureSchema)

	if len(solver.arch) > 1 {
		latentDimension = solver.arch[len(solver.arch)-1]
	}

	if hasTemporal && latentDimension > 0 {
		temporalError /= math.Sqrt(float64(latentDimension))
	}

	newAlpha := state.alphaCtrl.Update(surprise, temporalError)

	if newAlpha != state.alpha {
		state.alpha = newAlpha
		state.manifold.SetAlpha(newAlpha)
	}

	confidence := state.confidence.Quantile(surprise)
	activeHorizon := solver.horizonFor(state, confidence)
	var forwardCurve []float64
	var forwardRetention []float64

	if state.targetSamples > 0 {
		forwardCurve = state.manifold.RolloutTaskPrediction(activeHorizon)
		forwardRetention = state.manifold.RolloutRetention(activeHorizon)
	}

	expectedReturn := 0.0
	returnReady := len(forwardCurve) > 0

	if returnReady {
		expectedReturn = forwardCurve[0]
	}

	if targetReady && (state.pendingAt.IsZero() || !targetAt.Before(state.pendingAt)) {
		state.input = state.pendingInput[:0]
		state.pendingInput = input
		state.pendingMid = midpoint
		state.pendingAt = targetAt
	}

	var latent []float64

	if solver.ui != nil && symbol == types.Focus() {
		latent = state.manifold.LatentState()
	}

	row := map[string]any{
		"stage":            "resonance",
		"source":           string(types.SourceResonance),
		"symbol":           symbol,
		"targetSymbol":     symbol,
		"at":               thesis.At,
		"surprise":         surprise,
		"energy":           energy,
		"latent":           latent,
		"forwardCurve":     forwardCurve,
		"expectedReturn":   expectedReturn,
		"returnReady":      returnReady,
		"forwardRetention": forwardRetention,
		"activeHorizon":    activeHorizon,
		"confidence":       confidence,
		"alpha":            state.alpha,
		"samples":          state.targetSamples,
	}
	thesis.Resonance.Store(symbol, row)

	return nil
}

/*
horizonRetentionFloor is how much of the first step's latent magnitude must
still survive at a later step for that step to count as forecast.

The floor is relative to the first rollout step rather than to the settled state
it starts from. One application of tanh(A) already removes most of the magnitude
— live, the first step retains under a third — so an absolute floor anywhere
near this value truncates every horizon to a single step no matter what the
dynamics do afterwards, which silently defeats the whole rollout.

Measured against the first step, the quantity means what it should: how fast the
trajectory is decaying relative to where the forecast begins. A third is the
point past which the surviving state contributes less than the relaxation has
removed, so the reading is dominated by the envelope rather than by the market.
*/
const horizonRetentionFloor = 1.0 / 3.0

/*
horizonPrecisionFloor is the task precision below which the forecast horizon
retracts rather than holds.

Task precision is scale-free and centres on one when the head is predicting at
its own typical accuracy. Holding the reach at that point and extending only
above it means the horizon grows on evidence that the head is currently doing
better than usual, and gives way as soon as it is not.
*/
const horizonPrecisionFloor = 1.0

/*
horizonGrowthStep is how many steps the reach may extend on one tick of holding
precision.

Reach is earned one step at a time so a run of good ticks is required before the
forecast claims to see far, while a single tick of degraded precision can hand
back much more than a single tick earned. That asymmetry is deliberate: claiming
too much reach prices trades off a forecast that was never made, whereas
claiming too little only forgoes edge.
*/
const horizonGrowthStep = 1

/*
horizonRetractionFactor is the share of the current reach kept when precision
falls below its floor.

Halving retracts within a handful of ticks from any reach the ceiling allows,
which is fast enough that a regime change does not leave stale reach in place
for the length of a position.
*/
const horizonRetractionFactor = 0.5

/*
horizonFor is how many steps ahead the forward curve is worth reading.

The reach is adaptive state rather than a per-tick recomputation. It extends
while the head's own precision holds at or above its typical accuracy, and
retracts when precision degrades, which is what lets the window grow through a
regime the network has learned and give way when the market changes underneath
it. A quantity recomputed from scratch each tick could only ever reflect that
tick, and so could never express a window earned over many.

Three ceilings bound whatever the reach has grown to, and the tightest wins:

Retention, because the temporal recursion is a contraction. Past the point where
the latent has relaxed toward the origin, further steps report the decay envelope
rather than a forecast, and no amount of precision makes them a forecast.

Confidence, because a network currently predicting worse than its own recent
history has no business claiming its full learned reach.

maxHorizon, as the absolute cap.
*/
func (solver *Solver) horizonFor(state *symbolState, confidence float64) int {
	precision, hasPrecision := state.manifold.TaskPrecision()

	switch {
	case !hasPrecision:
		/*
			No supervised sample has resolved yet, so the head has no basis for
			any reach at all and starts from the shortest one.
		*/
		state.horizonReach = 1
	case precision >= horizonPrecisionFloor:
		state.horizonReach += horizonGrowthStep
	default:
		state.horizonReach = int(math.Floor(
			float64(state.horizonReach) * horizonRetractionFactor,
		))
	}

	state.horizonReach = min(solver.maxHorizon, max(1, state.horizonReach))

	horizon := state.horizonReach

	/*
		Confidence caps the reach without consuming it. A tick the network finds
		hard shortens what it publishes now, but the reach it has earned is
		still there when the next tick is ordinary again.
	*/
	if capped := int(math.Floor(float64(state.horizonReach) * confidence)); capped < horizon {
		horizon = capped
	}

	if horizon < 1 {
		return 1
	}

	retention := state.manifold.RolloutRetention(horizon)

	if len(retention) == 0 || retention[0] <= 0 {
		return 1
	}

	/*
		Every step is measured against the first, which is where the forecast
		begins. The first step therefore always qualifies, and the horizon
		extends for as long as the trajectory has not decayed to a third of it.
	*/
	for step, surviving := range retention {
		if surviving/retention[0] < horizonRetentionFloor {
			if step == 0 {
				return 1
			}

			return step
		}
	}

	return horizon
}

/*
initManifold constructs the CPU predictive coding network.
If an architecture wasn't explicitly supplied, it constructs an adaptive 3-layer
predictive network: [InputDim -> InputDim * 2 -> InputDim].
*/
func (solver *Solver) initManifold(state *symbolState, inputDim int) error {
	arch := append([]int(nil), solver.arch...)

	if len(arch) < 2 {
		// Default 3-layer bottleneck architecture
		arch = []int{inputDim, inputDim * 2, inputDim}
	} else {
		arch[0] = inputDim // Ensure input layer matches actual feature dimension
	}

	manifold, err := learning.NewResonanceManifold(
		arch,
		solver.targetDim,
		state.alpha,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: failed to initialize CPU predictive coding manifold",
			err,
		))
	}

	state.manifold = manifold
	return nil
}

/*
extractFeatures converts physics/field data from Thesis into float64 slices for the manifold.
Reads features, measurements, or metrics produced by the upstream `manifold.Solver`.
*/
func (solver *Solver) extractFeatures(thesis *types.Thesis) map[string]map[string]float64 {
	features := make(map[string]map[string]float64)

	thesis.Measurements.Range(func(key, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if !ok {
			measurement, measurementOK := value.(*types.Measurement)

			if !measurementOK || measurement == nil {
				return true
			}

			rows = []*types.Measurement{measurement}
		}

		for _, measurement := range rows {
			if measurement == nil || measurement.Symbol == "" {
				continue
			}

			bySymbol, ok := features[measurement.Symbol]

			if !ok {
				bySymbol = make(map[string]float64)
				features[measurement.Symbol] = bySymbol
			}

			for metricKey, metric := range measurement.Metrics {
				if metric.Normalized == nil ||
					math.IsNaN(*metric.Normalized) ||
					math.IsInf(*metric.Normalized, 0) {
					continue
				}

				/*
					Peer is deliberately not part of the identity. It names the
					counterpart a relative reading was taken against, and that
					counterpart rotates with the live cross-section anchor, so
					including it would mint a new input dimension every time
					leadership changed and keep the network resizing instead of
					learning.
				*/
				featureKey := string(measurement.Source) + ":" + measurement.Symbol + ":" + metricKey
				bySymbol[featureKey] = *metric.Normalized
			}
		}

		return true
	})

	return features
}

func (solver *Solver) standardizeFeatures(
	state *symbolState,
	features map[string]float64,
) map[string]float64 {
	if len(features) == 0 {
		return nil
	}

	if state.featureScale == nil {
		state.featureScale = make(map[string]*featureNormalizer)
	}

	for featureKey, reading := range features {
		normalizer, ok := state.featureScale[featureKey]

		if !ok {
			normalizer = newFeatureNormalizer()
			state.featureScale[featureKey] = normalizer
		}

		features[featureKey] = normalizer.Standardize(reading)
	}

	return features
}

/*
returnTarget reads the target symbol's current executable midpoint. Ticker
timestamps define market epochs, so repeated signal updates within one ticker
epoch cannot resolve a prediction against itself.

The symbol is whichever one the book actually carries, preferring the dashboard
focus only when it is present. Focus is a UI publish gate, so letting it pick
the target outright would let an operator's panel selection decide whether the
task head learns at all, and would starve it entirely whenever the focused
symbol is not being traded.
*/
func (solver *Solver) returnTarget(
	thesis *types.Thesis,
	symbol string,
) (float64, time.Time, bool) {
	if thesis == nil || thesis.Tickers == nil {
		return 0, time.Time{}, false
	}

	ticker, ok := thesis.LatestTicker(symbol)

	if !ok || ticker.Timestamp.IsZero() {
		return 0, time.Time{}, false
	}

	if ticker.Bid != nil && ticker.Ask != nil {
		bid := ticker.Bid.Float64()
		ask := ticker.Ask.Float64()

		if bid > 0 && ask >= bid {
			return (bid + ask) / 2, ticker.Timestamp, true
		}
	}

	if ticker.Last == nil || ticker.Last.Sign() <= 0 {
		return 0, time.Time{}, false
	}

	return ticker.Last.Float64(), ticker.Timestamp, true
}

/*
learnReturn resolves the prior latent state against the next ticker-epoch
midpoint log return before the current feature vector is settled. This keeps
the task head strictly prior and prevents target leakage.
*/
func (solver *Solver) learnReturn(state *symbolState, midpoint float64, at time.Time) error {
	if len(state.pendingInput) == 0 || !at.After(state.pendingAt) {
		return nil
	}

	/*
		An unusable quote makes this one sample unresolvable, not the pass.
		Returning an error here would abort the analyzer before the planner
		runs, so a single bad tick would stop the desk from deciding at all.
		The sample is dropped and the pairing reset so the next epoch starts
		from a clean prior.
	*/
	if state.pendingMid <= 0 || midpoint <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: dropped return sample on non-positive target price",
			nil,
		))

		state.pendingInput = state.pendingInput[:0]
		state.pendingAt = time.Time{}

		return nil
	}

	target := math.Log(midpoint / state.pendingMid)

	if math.IsNaN(target) || math.IsInf(target, 0) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: dropped return sample on non-finite realized return",
			nil,
		))

		state.pendingInput = state.pendingInput[:0]
		state.pendingAt = time.Time{}

		return nil
	}

	if err := state.manifold.Settle(state.pendingInput, false); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: prior return state failed to settle",
			err,
		))
	}

	if err := state.manifold.Learn([]float64{target}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: return target learning failed",
			err,
		))
	}

	state.targetSamples++

	return nil
}

/*
Reset clears learned temporal and precision state in the predictive coding manifold.
*/
func (solver *Solver) Reset(resetPrecision bool) {
	for _, state := range solver.states {
		if state.manifold != nil {
			state.manifold.ResetState(resetPrecision)
		}
	}
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.states = nil
	return nil
}
