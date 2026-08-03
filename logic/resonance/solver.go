package resonance

import (
	"math"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const resonanceReturnDimensions = 1

/*
AlphaController dynamically adjusts alpha based on real-time prediction error ratios.
It monitors the ratio between Temporal Prediction Error (memory mismatch) and Reconstruction Error
(current sensory mismatch) to detect overfitting/noise-chasing vs underfitting/stiffness:
  - Ratio > 3.0: Overfitting micro-noise -> Decreases alpha to protect long-term memory A.
  - Ratio < 0.5: Underfitting/stiff -> Increases alpha to accelerate learning pace.
  - eRecon Spike > 2.5x EMA: Regime Shift / Flash Crash -> Temporary alpha boost to adapt rapidly.
*/
type AlphaController struct {
	currentAlpha float64
	minAlpha     float64
	maxAlpha     float64
	emaRecon     float64
	emaTemporal  float64
	beta         float64 // EMA smoothing weight
}

/*
NewAlphaController constructs a dynamic pace controller bounded within [minAlpha, maxAlpha].
*/
func NewAlphaController(initialAlpha, minAlpha, maxAlpha float64) *AlphaController {
	return &AlphaController{
		currentAlpha: initialAlpha,
		minAlpha:     minAlpha,
		maxAlpha:     maxAlpha,
		beta:         0.05,
	}
}

/*
Update evaluates current reconstruction and temporal error norms and returns the new alpha.
*/
func (ac *AlphaController) Update(eRecon, eTemporal float64) float64 {
	if ac.emaRecon == 0 {
		ac.emaRecon = eRecon
		ac.emaTemporal = eTemporal
	} else {
		ac.emaRecon = (1-ac.beta)*ac.emaRecon + ac.beta*eRecon
		ac.emaTemporal = (1-ac.beta)*ac.emaTemporal + ac.beta*eTemporal
	}

	ratio := ac.emaTemporal / (ac.emaRecon + 1e-6)

	if eRecon > 2.5*ac.emaRecon {
		// Sudden Regime Shift / Volatility Spike: boost alpha to adapt fast
		ac.currentAlpha *= 1.5
	} else if ratio > 3.0 {
		// Overfitting (chasing tick noise): dampen alpha to protect temporal memory
		ac.currentAlpha *= 0.95
	} else if ratio < 0.5 {
		// Underfitting (stiff/slow): increase learning pace
		ac.currentAlpha *= 1.05
	}

	// Clamp within safe bounds
	if ac.currentAlpha < ac.minAlpha {
		ac.currentAlpha = ac.minAlpha
	}
	if ac.currentAlpha > ac.maxAlpha {
		ac.currentAlpha = ac.maxAlpha
	}

	return ac.currentAlpha
}

/*
Solver wraps the CPU Predictive Coding Resonance Manifold (`learning.ResonanceManifold`).
It accepts physics/field metrics from the upstream manifold solver, computes prediction
errors (Surprise), settles top-down/bottom-up latent states, dynamically adapts alpha,
and extends forward prediction horizons dynamically based on confidence.
*/
type Solver struct {
	recorder        *audit.Recorder
	manifold        *learning.ResonanceManifold
	alphaCtrl       *AlphaController
	alpha           float64
	arch            []int
	featureSchema   []string
	targetDim       int
	pendingInput    []float64
	pendingMid      float64
	pendingAt       time.Time
	targetSamples   uint64
	maxHorizon      int // Maximum forward prediction horizon (e.g. 20 ticks)
	learn           bool
	advanceTemporal bool
	ui              chan []byte
}

/*
NewSolver returns a new predictive coding solver wired to audit recording, dynamic alpha control,
and dynamic forward prediction rollout.
Defaults: initial alpha = 0.03, maxHorizon = 20 ticks.
*/
func NewSolver(ui chan []byte, recorder *audit.Recorder) *Solver {
	initialAlpha := 0.03
	solver := &Solver{
		recorder:        recorder,
		alpha:           initialAlpha,
		alphaCtrl:       NewAlphaController(initialAlpha, 0.005, 0.150),
		targetDim:       resonanceReturnDimensions,
		maxHorizon:      20, // Can extend up to 20 ticks ahead when confidence is high
		learn:           true,
		advanceTemporal: true,
		ui:              ui,
	}

	return solver
}

/*
Update extracts physical/field feature vectors from the Thesis, settles the CPU predictive
coding manifold, dynamically tunes alpha and forward prediction horizon, and enriches the Thesis.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	// 1. Extract feature vector & optional target from Thesis
	features := solver.extractFeatures(thesis)

	if len(features) == 0 {
		return nil
	}

	/*
		The schema grows while the network is still learning nothing, and is
		frozen the moment the task head has a supervised sample worth keeping.

		Live, the set of measurements present drifts continuously: signals
		report at different cadences and leadlag renames its peer whenever the
		cross-section anchor rotates. Resizing the network to follow that drift
		discards every weight it has learned along with the samples behind
		them, and the drift arrives faster than the task head can accumulate
		the two strict-prior samples it needs, so the network stays
		permanently in warmup and never produces a forecast.

		Admitting new features only before the first sample lets the schema
		settle as the signals come up, then holds it steady so learning can
		accumulate. Once frozen, an unseen feature is ignored and a missing one
		reads as zero.
	*/
	if solver.targetSamples == 0 {
		featureSchema := append([]string(nil), solver.featureSchema...)
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

		if solver.manifold == nil || len(featureSchema) != len(solver.featureSchema) {
			if err := solver.initManifold(len(featureSchema)); err != nil {
				return errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"resonance: failed to initialize CPU predictive coding manifold",
					err,
				))
			}

			solver.featureSchema = featureSchema
			solver.pendingInput = nil
			solver.pendingMid = 0
			solver.pendingAt = time.Time{}
		}
	}

	input := make([]float64, len(solver.featureSchema))

	for featureIndex, featureKey := range solver.featureSchema {
		input[featureIndex] = features[featureKey]
	}

	midpoint, targetAt, targetReady := solver.returnTarget(thesis)

	if targetReady && solver.learn {
		if err := solver.learnReturn(midpoint, targetAt); err != nil {
			return err
		}
	}

	// 3. Settle latents and apply Hebbian predictive-coding updates on CPU
	totalSurprise, err := solver.manifold.SettleFromBatchOptions(
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

	// 4. Extract wire layers, top latent state, and total energy
	layers, _, totalEnergy := solver.manifold.WireSnapshot()
	energy := totalEnergy / featureCount
	latent := solver.manifold.LatentState()

	// 5. Evaluate Reconstruction Error vs Temporal
	// Error for Dynamic Alpha Control
	eRecon := surprise
	eTemporal := 0.0

	if len(layers) > 0 {
		topLayer := layers[len(layers)-1]

		if len(topLayer.State) > 0 {
			eTemporal = topLayer.ErrorNorm / math.Sqrt(float64(len(topLayer.State)))
		}
	}

	newAlpha := solver.alphaCtrl.Update(eRecon, eTemporal)

	if newAlpha != solver.alpha {
		solver.alpha = newAlpha
		solver.manifold.SetAlpha(newAlpha)
	}

	// 6. Calculate Confidence and determine Active Dynamic Horizon K
	// High confidence (low eRecon) extends horizon up to maxHorizon
	// (20 ticks). Low confidence (high eRecon) collapses horizon back
	// down to 1 tick.
	confidence := math.Exp(-eRecon)

	activeHorizon := int(math.Max(1.0, math.Floor(
		float64(solver.maxHorizon)*confidence,
	)))

	// 7. Perform Dynamic Recurrent Rollout for k steps
	var forwardCurve []float64

	if solver.targetSamples > 0 {
		forwardCurve = solver.manifold.RolloutTaskPrediction(activeHorizon)
	}

	if targetReady && (solver.pendingAt.IsZero() || !targetAt.Before(solver.pendingAt)) {
		solver.pendingInput = append(solver.pendingInput[:0], input...)
		solver.pendingMid = midpoint
		solver.pendingAt = targetAt
	}

	// 8. Enrich the shared Thesis with predictive coding
	// outcomes, dynamic alpha, and return curve
	thesis.Resonance.Store("surprise", surprise)
	thesis.Resonance.Store("energy", energy)
	thesis.Resonance.Store("latent", latent)
	thesis.Resonance.Store("forwardCurve", forwardCurve)
	thesis.Resonance.Store("activeHorizon", activeHorizon)
	thesis.Resonance.Store("confidence", confidence)
	thesis.Resonance.Store("layers", layers)
	thesis.Resonance.Store("alpha", solver.alpha)

	/*
		Stamp only once the task head has actually produced a forward curve.
		The curve is what the stages downstream read out of resonance, so a
		stamp without one claims a forecast that was never made and lets the
		causal and graph stages run on a reading that does not exist yet.

		targetSamples resets whenever the feature schema changes, so this is
		the ordinary state during warmup and after any re-initialisation.
	*/
	if len(forwardCurve) > 0 {
		thesis.StampSource(types.SourceResonance, types.MarketDerived)
	}

	/*
		The wire still carries every reading regardless, so the dashboard shows
		the network settling during warmup rather than going blank.
	*/
	out := datura.NewMap()
	out["surprise"] = surprise
	out["energy"] = energy
	out["latent"] = latent
	out["forwardCurve"] = forwardCurve
	out["activeHorizon"] = activeHorizon
	out["confidence"] = confidence
	out["alpha"] = solver.alpha

	utils.Publish(solver.ui, datura.NewMap("resonance", out))

	// 9. Record audit snapshot if recorder is attached
	if solver.recorder != nil {
		errnie.Error(audit.Record(solver.recorder, "predictive", map[string]any{
			"stage":         "resonance",
			"surprise":      surprise,
			"energy":        energy,
			"latent":        latent,
			"alpha":         solver.alpha,
			"confidence":    confidence,
			"activeHorizon": activeHorizon,
			"forwardCurve":  forwardCurve,
		}))
	}

	return nil
}

/*
initManifold constructs the CPU predictive coding network.
If an architecture wasn't explicitly supplied, it constructs an adaptive 3-layer
predictive network: [InputDim -> InputDim * 2 -> InputDim].
*/
func (solver *Solver) initManifold(inputDim int) error {
	arch := solver.arch

	if len(arch) < 2 {
		// Default 3-layer bottleneck architecture
		arch = []int{inputDim, inputDim * 2, inputDim}
	} else {
		arch[0] = inputDim // Ensure input layer matches actual feature dimension
	}

	manifold, err := learning.NewResonanceManifold(
		arch,
		solver.targetDim,
		solver.alpha,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: failed to initialize CPU predictive coding manifold",
			err,
		))
	}

	solver.manifold = manifold
	return nil
}

/*
extractFeatures converts physics/field data from Thesis into float64 slices for the manifold.
Reads features, measurements, or metrics produced by the upstream `manifold.Solver`.
*/
func (solver *Solver) extractFeatures(thesis *types.Thesis) map[string]float64 {
	features := make(map[string]float64)

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
			if measurement == nil {
				continue
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
				features[featureKey] = *metric.Normalized
			}
		}

		return true
	})

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
) (float64, time.Time, bool) {
	if thesis == nil || thesis.Tickers == nil {
		return 0, time.Time{}, false
	}

	rawTicker, ok := thesis.Tickers.Load(types.Focus())

	if !ok {
		/*
			Ranging a sync.Map yields no defined order, so the lowest symbol
			keeps the target stable from one tick to the next. A target that
			hopped between symbols would resolve each prediction against a
			different market than the one that produced it.
		*/
		selected := ""

		thesis.Tickers.Range(func(key, value any) bool {
			symbol, isString := key.(string)

			if !isString {
				return true
			}

			if selected == "" || symbol < selected {
				selected, rawTicker = symbol, value
			}

			return true
		})

		if selected == "" {
			return 0, time.Time{}, false
		}
	}

	ticker, ok := rawTicker.(kraken.TickerData)

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
func (solver *Solver) learnReturn(midpoint float64, at time.Time) error {
	if len(solver.pendingInput) == 0 || !at.After(solver.pendingAt) {
		return nil
	}

	/*
		An unusable quote makes this one sample unresolvable, not the pass.
		Returning an error here would abort the analyzer before the planner
		runs, so a single bad tick would stop the desk from deciding at all.
		The sample is dropped and the pairing reset so the next epoch starts
		from a clean prior.
	*/
	if solver.pendingMid <= 0 || midpoint <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: dropped return sample on non-positive target price",
			nil,
		))

		solver.pendingInput = solver.pendingInput[:0]
		solver.pendingAt = time.Time{}

		return nil
	}

	target := math.Log(midpoint / solver.pendingMid)

	if math.IsNaN(target) || math.IsInf(target, 0) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: dropped return sample on non-finite realized return",
			nil,
		))

		solver.pendingInput = solver.pendingInput[:0]
		solver.pendingAt = time.Time{}

		return nil
	}

	if err := solver.manifold.Settle(solver.pendingInput, false); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: prior return state failed to settle",
			err,
		))
	}

	if err := solver.manifold.Learn([]float64{target}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: return target learning failed",
			err,
		))
	}

	solver.targetSamples++

	return nil
}

/*
Reset clears learned temporal and precision state in the predictive coding manifold.
*/
func (solver *Solver) Reset(resetPrecision bool) {
	if solver.manifold != nil {
		solver.manifold.ResetState(resetPrecision)
	}
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.manifold = nil
	return nil
}
