package resonance

import (
	"math"
	"runtime"
	"slices"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

const (
	resonanceReturnDimensions = 1
	restAlpha                 = 0.03
)

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
	maxHorizon      int
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
	return &Solver{
		recorder:        recorder,
		states:          make(map[string]*symbolState),
		targetDim:       resonanceReturnDimensions,
		maxHorizon:      20,
		learn:           true,
		advanceTemporal: true,
		ui:              ui,
	}
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
			state.alphaCtrl = learning.NewPaceController(learning.PaceConfig{
				InitialAlpha: state.alpha,
			})
			state.confidence = probability.NewCalibrator()
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
	informative := false

	for featureIndex, featureKey := range state.featureSchema {
		input[featureIndex] = features[featureKey]

		if input[featureIndex] != 0 {
			informative = true
		}
	}

	/*
		A standardizer answers zero until it has the moments to score against,
		so a vector that is entirely zero is the absence of a reading rather
		than a market that measured zero on all counts.

		Settling on it drives the latent state to the origin, which makes the
		rollout retention zero and publishes the forecast as invalid; learning
		from it spends a resolved return sample teaching the return head that
		no input predicts a real move. Both are wrong about a stage that is
		simply still warming, so the reading says so and waits.
	*/
	if !informative {
		thesis.Resonance.Store(symbol, types.ResonanceReading{
			Stage:        "resonance",
			Source:       types.SourceResonance,
			Symbol:       symbol,
			TargetSymbol: symbol,
			At:           thesis.At,
			Alpha:        state.alpha,
			Samples:      state.targetSamples,
			ForecastValidity: types.MeasurementValidity{
				State:     types.ValidityProvisional,
				Readiness: types.ReadinessModel,
				Reason:    "resonance features have no standardizable scale yet",
			},
		})

		return nil
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
	activeHorizon, newReach := state.manifold.DynamicHorizon(
		confidence, state.horizonReach, solver.maxHorizon,
	)
	state.horizonReach = newReach

	var forecast *types.ResonanceForecast
	forecastValidity := types.MeasurementValidity{
		State:     types.ValidityProvisional,
		Readiness: types.ReadinessModel,
		Reason:    "resonance return head has no resolved sample",
	}

	if state.targetSamples > 0 {
		var err error
		forecast, err = types.NewResonanceForecast(
			state.manifold.RolloutTaskPrediction(activeHorizon),
			state.manifold.RolloutRetention(activeHorizon),
			activeHorizon,
			confidence,
		)

		if err != nil {
			forecastValidity.State = types.ValidityInvalid
			forecastValidity.Reason = err.Error()
		} else {
			forecastValidity.State = types.ValidityValid
			forecastValidity.Readiness = types.ReadinessForecast
			forecastValidity.Reason = ""
		}
	}

	if targetReady && (state.pendingAt.IsZero() || !targetAt.Before(state.pendingAt)) {
		state.pendingInput = slices.Clone(input)
		state.pendingMid = midpoint
		state.pendingAt = targetAt
	}

	var latent []float64

	if solver.ui != nil && symbol == types.Focus() {
		latent = state.manifold.LatentState()
	}

	row := types.ResonanceReading{
		Stage:            "resonance",
		Source:           types.SourceResonance,
		Symbol:           symbol,
		TargetSymbol:     symbol,
		At:               thesis.At,
		Surprise:         surprise,
		Energy:           energy,
		Latent:           latent,
		Forecast:         forecast,
		ForecastValidity: forecastValidity,
		Alpha:            state.alpha,
		Samples:          state.targetSamples,
	}
	thesis.Resonance.Store(symbol, row)

	return nil
}

func (solver *Solver) initManifold(state *symbolState, inputDim int) error {
	arch := append([]int(nil), solver.arch...)

	if len(arch) < 2 {
		arch = []int{inputDim, inputDim * 2, inputDim}
	} else {
		arch[0] = inputDim
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
		state.featureScale = make(map[string]*adaptive.Standardizer)
	}

	for featureKey, reading := range features {
		normalizer, ok := state.featureScale[featureKey]

		if !ok {
			normalizer = adaptive.NewStandardizer()
			state.featureScale[featureKey] = normalizer
		}

		output, err := normalizer.Measure(reading)

		if err != nil {
			features[featureKey] = 0
			continue
		}

		features[featureKey] = output.Value
	}

	return features
}

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

func (solver *Solver) learnReturn(state *symbolState, midpoint float64, at time.Time) error {
	if len(state.pendingInput) == 0 || !at.After(state.pendingAt) {
		return nil
	}

	if state.pendingMid <= 0 || midpoint <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: dropped return sample on non-positive target price",
			nil,
		))

		state.pendingInput = nil
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

		state.pendingInput = nil
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
