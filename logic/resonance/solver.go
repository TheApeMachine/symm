package resonance

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

const (
	resonanceReturnDimensions = 1
)

/*
Solver wraps the CPU Predictive Coding Resonance Manifold (`learning.ResonanceManifold`).
It accepts physics/field metrics from the upstream manifold solver, computes prediction
errors (Surprise), settles top-down/bottom-up latent states, learns forward returns
with recursive least squares, and extends horizons based on resolved forecast skill.
*/
type Solver struct {
	ctx              context.Context
	cancel           context.CancelFunc
	recorder         *audit.Recorder
	states           map[string]*symbolState
	arch             []int
	targetDim        int
	initialAlpha     float64
	configurationErr error
	learn            bool
	advanceTemporal  bool
	ui               chan []byte
}

/*
NewSolver returns a predictive coding solver with a configured base alpha and
dynamic forward prediction rollout.
The forecast horizon grows only as resolved return samples accumulate and
contracts to the confidence-supported path length.
*/
func NewSolver(
	ctx context.Context,
	ui chan []byte,
	recorder *audit.Recorder,
	initialAlpha float64,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	solver := &Solver{
		ctx:             ctx,
		cancel:          cancel,
		recorder:        recorder,
		states:          make(map[string]*symbolState),
		targetDim:       resonanceReturnDimensions,
		initialAlpha:    initialAlpha,
		learn:           true,
		advanceTemporal: true,
		ui:              ui,
	}

	if !(initialAlpha > 0) || initialAlpha > 1 || math.IsNaN(initialAlpha) || math.IsInf(initialAlpha, 0) {
		solver.configurationErr = errors.New(
			"resonance: learning rate must be finite and in (0, 1]",
		)
	}

	return solver
}

func (solver *Solver) state(symbol string) *symbolState {
	state, ok := solver.states[symbol]

	if ok {
		return state
	}

	state = newSymbolState(solver.initialAlpha)
	solver.states[symbol] = state

	return state
}

/*
Update extracts physical/field feature vectors from the Thesis, settles the CPU
predictive-coding manifold, updates the adaptive return learner and forward
prediction horizon, and enriches the Thesis.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if solver.configurationErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: invalid solver configuration",
			solver.configurationErr,
		))
	}

	featureSets := solver.extractFeatures(thesis)

	if len(featureSets) > 0 {
		group, _ := errgroup.WithContext(context.Background())

		for symbol := range featureSets {
			solver.state(symbol)
		}

		for symbol, features := range featureSets {
			group.Go(func() error {
				return solver.updateSymbol(thesis, symbol, features)
			})
		}

		if err := group.Wait(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"resonance: failed to update CPU predictive coding manifold: "+err.Error(),
				err,
			))
		}

		thesis.Readiness.Stamp(types.SourceResonance)

		// Every carrier the solver settled this round is published, not just the
		// focused one. The latent manifold is a cross-section — a cloud of symbols
		// positioned by where their predictive states sit relative to one another —
		// and one point is not a cross-section. Only the focused row carries the
		// full latent vector and the layer stack; every other row carries its
		// embedding and its scalars, which is what the cloud actually plots.
		rows := make([]any, 0, 64)

		thesis.Resonance.Range(func(_, value any) bool {
			rows = append(rows, value)
			return true
		})

		if len(rows) != 0 {
			utils.Publish(
				solver.ui,
				datura.NewMap("resonance", rows),
			)
		}
	}

	thesis.Fanout()
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

	if state.manifold != nil && !state.hasFeatures(features) {
		return nil
	}

	features, informative, err := solver.standardizeFeatures(state, features)

	if err != nil {
		return err
	}

	if state.manifold == nil {
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
					"resonance: failed to initialize CPU predictive coding manifold: "+err.Error(),
					err,
				))
			}

			state.featureSchema = featureSchema
			state.extractor = vector.NewFeatureExtractor(vector.FeatureExtractorConfig{
				FeatureScopeConfig: vector.FeatureScopeConfig{
					Root:   ".",
					Inputs: slices.Clone(featureSchema),
				},
			})
			state.pendingInput = nil
			state.pendingMid = 0
			state.pendingAt = time.Time{}
			state.skill = probability.NewBernoulli()
			state.skillEvidence = chanceForecastSkill
			state.horizonReach = 1
		}
	}

	fields := make([]vector.NamedValue, 0, len(features))

	for featureKey, reading := range features {
		fields = append(fields, vector.NamedValue{
			Name:  featureKey,
			Value: reading,
		})
	}

	featureVector, err := state.extractor.Measure(vector.FeatureInput{
		Row: vector.NewFeatureRow(fields...),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: failed to extract ordered feature vector: "+err.Error(),
			err,
		))
	}

	input := featureVector.Features

	/*
		A standardizer reports whether prior spread makes its current score a
		measurement. During warmup it answers zero without readiness, while a
		ready observation at the learned mean is also legitimately zero.

		Settling on an unready vector drives the latent state to the origin, which makes the
		rollout retention zero and publishes the forecast as invalid; learning
		from it spends a resolved return sample teaching the return head that
		no input predicts a real move. Both are wrong about a stage that has
		simply not seen a feature move yet, so the reading says so and waits.

		This resolves as soon as any feature varies, because the standardizer
		scores against its own precision rather than waiting out a sample count:
		an early reading is small because the scale behind it is uncertain, and
		it grows into a full z-score as the moments settle. Once ready, its value
		may still be zero without making the observation absent.
	*/
	if !informative {
		thesis.Resonance.Store(symbol, types.ResonanceReading{
			Stage:         "resonance",
			Source:        types.SourceResonance,
			Symbol:        symbol,
			TargetSymbol:  symbol,
			At:            thesis.At,
			Verdict:       resonanceVerdict(nil),
			Alpha:         state.alpha,
			Samples:       state.targetSamples,
			SkillEvidence: state.skillEvidence,
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
			"resonance: CPU predictive coding settle failed: "+err.Error(),
			err,
		))
	}

	featureCount := float64(len(input))
	surprise := totalSurprise / math.Sqrt(featureCount)
	energy := state.manifold.PredictionEnergy() / featureCount

	firstStep, err := state.manifold.RolloutTaskForecast(1)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: first-step return forecast failed: "+err.Error(),
			err,
		))
	}

	if len(firstStep) != solver.targetDim {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: first-step return forecast dimension mismatch",
			nil,
		))
	}

	confidence, err := state.forecastConfidence(firstStep[0])

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: current forecast confidence failed: "+err.Error(),
			err,
		))
	}

	activeHorizon := solver.horizon(state, confidence)

	rollout, err := state.manifold.RolloutTaskForecast(activeHorizon)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: return forecast rollout failed: "+err.Error(),
			err,
		))
	}

	if len(rollout) != activeHorizon*solver.targetDim {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: return forecast rollout dimension mismatch",
			nil,
		))
	}

	curve := make([]float64, activeHorizon)

	for step := range activeHorizon {
		curve[step] = rollout[step*solver.targetDim].Value
	}

	retention := state.manifold.RolloutRetention(activeHorizon)
	var forecast *types.ResonanceForecast

	if len(retention) == activeHorizon && retention[0] > 0 {
		forecast, err = types.NewResonanceForecast(
			curve, retention, activeHorizon, confidence,
		)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"resonance: invalid forward forecast: "+err.Error(),
				err,
			))
		}

		if err := forecast.SetPredictiveDistribution(
			rollout[0].Scale,
			rollout[0].DegreesOfFreedom,
			rollout[0].Ready,
		); err != nil {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"resonance: invalid predictive distribution: "+err.Error(),
				err,
			))
		}
	}

	verdict := resonanceVerdict(forecast)

	if targetReady && (state.pendingAt.IsZero() || !targetAt.Before(state.pendingAt)) {
		state.pendingInput = slices.Clone(input)
		state.pendingMid = midpoint
		state.pendingAt = targetAt
	}

	/*
		Every carrier carries the two leading components of its settled state so
		the cross-section can place it against the others. The full vector and the
		layer stack stay with the focused symbol: those are what the hierarchy
		panel reads, and they are the expensive part of the frame — the whole
		universe's worth of them would be published every round to draw a scatter
		that only needs two numbers per symbol.
	*/
	var latent []float64
	var layers []learning.ResonanceLayerWire
	var embedding []float64

	if solver.ui != nil {
		settled := state.manifold.LatentState()

		if len(settled) >= 2 {
			embedding = []float64{settled[0], settled[1]}
		}

		if symbol == types.Focus() {
			latent = settled
			layers, _, _ = state.manifold.WireSnapshot()
		}
	}

	row := types.ResonanceReading{
		Stage:         "resonance",
		Source:        types.SourceResonance,
		Symbol:        symbol,
		TargetSymbol:  symbol,
		At:            thesis.At,
		Surprise:      surprise,
		Energy:        energy,
		Latent:        latent,
		Embedding:     embedding,
		Layers:        layers,
		Forecast:      forecast,
		Verdict:       verdict,
		Alpha:         state.alpha,
		Samples:       state.targetSamples,
		SkillEvidence: state.skillEvidence,
	}

	thesis.Resonance.Store(symbol, row)
	return nil
}

/*
horizon returns and remembers the confidence-supported forecast reach. Resolved
return samples are the finite bound: the model cannot support more steps than
targets it has learned, and no fixed product horizon is imposed.

DynamicHorizon floors its confidence cap. At one remembered step that would
make every confidence below certainty publish one step forever, even while the
task head is precise enough to grow. Using the ceiling of the same proportional
support lets high confidence earn the next whole tick. The published horizon is
still what is remembered, so weak confidence contracts the next rollout rather
than merely hiding an independently growing reach.
*/
func (solver *Solver) horizon(state *symbolState, confidence float64) int {
	maximum := max(1, int(state.targetSamples))
	horizon, reach := state.manifold.DynamicHorizon(
		confidence, state.horizonReach, maximum,
	)
	confidenceReach := max(1, int(math.Ceil(float64(reach)*confidence)))

	if confidenceReach > horizon {
		supportedConfidence := float64(confidenceReach) / float64(reach)
		horizon, _ = state.manifold.DynamicHorizon(
			supportedConfidence, state.horizonReach, maximum,
		)
	}

	state.horizonReach = horizon
	return horizon
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
			"resonance: failed to initialize CPU predictive coding manifold: "+err.Error(),
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
) (map[string]float64, bool, error) {
	if len(features) == 0 {
		return nil, false, nil
	}

	if state.featureScale == nil {
		state.featureScale = make(map[string]*adaptive.Standardizer)
	}

	informative := false

	for featureKey, reading := range features {
		normalizer, ok := state.featureScale[featureKey]

		if !ok {
			normalizer = adaptive.NewStandardizer()
			state.featureScale[featureKey] = normalizer
		}

		output, err := normalizer.Measure(reading)

		if err != nil {
			return nil, false, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"resonance: failed to standardize feature: "+err.Error(),
				err,
			).With("feature", featureKey))
		}

		features[featureKey] = output.Value

		if output.Ready {
			informative = true
		}
	}

	return features, informative, nil
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
			"resonance: prior return state failed to settle: "+err.Error(),
			err,
		))
	}

	prediction := state.manifold.TaskPrediction()

	if len(prediction) != solver.targetDim {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: prior return forecast dimension mismatch",
			nil,
		))
	}

	if err := state.measureForecastSkill(prediction[0], target); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: forecast skill update failed: "+err.Error(),
			err,
		))
	}

	if err := state.manifold.Learn([]float64{target}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: return target learning failed: "+err.Error(),
			err,
		))
	}

	state.targetSamples++

	return nil
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.states = nil
	return nil
}
