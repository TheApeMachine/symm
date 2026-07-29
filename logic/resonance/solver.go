package resonance

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

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
	targetDim       int
	maxHorizon      int // Maximum forward prediction horizon (e.g. 20 ticks)
	learn           bool
	advanceTemporal bool
}

/*
NewSolver returns a new predictive coding solver wired to audit recording, dynamic alpha control,
and dynamic forward prediction rollout.
Defaults: initial alpha = 0.03, maxHorizon = 20 ticks.
*/
func NewSolver(recorder *audit.Recorder) *Solver {
	initialAlpha := 0.03
	solver := &Solver{
		recorder:        recorder,
		alpha:           initialAlpha,
		alphaCtrl:       NewAlphaController(initialAlpha, 0.005, 0.150),
		maxHorizon:      20, // Can extend up to 20 ticks ahead when confidence is high
		learn:           true,
		advanceTemporal: true,
	}

	return solver
}

/*
Update extracts physical/field feature vectors from the Thesis, settles the CPU predictive
coding manifold, dynamically tunes alpha and forward prediction horizon, and enriches the Thesis.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil {
		return nil
	}

	// 1. Extract feature vector & optional target from Thesis
	input, target := solver.extractFeatures(thesis)
	if len(input) == 0 {
		return nil
	}

	// 2. Lazily initialize the CPU ResonanceManifold if not yet created
	if solver.manifold == nil {
		if err := solver.initManifold(len(input), len(target)); err != nil {
			return err
		}
	}

	// 3. Settle latents and apply Hebbian predictive-coding updates on CPU
	surprise, err := solver.manifold.SettleFromBatchOptions(
		input,
		target,
		solver.learn,
		solver.advanceTemporal,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"resonance: CPU predictive coding settle failed",
			err,
		))
	}

	// 4. Extract wire layers, top latent state, and total energy
	layers, _, energy := solver.manifold.WireSnapshot()
	latent := solver.manifold.LatentState()

	// 5. Evaluate Reconstruction Error vs Temporal Error for Dynamic Alpha Control
	eRecon := solver.manifold.ReconstructionError()
	eTemporal := 0.0
	if len(layers) > 0 {
		eTemporal = layers[len(layers)-1].ErrorNorm
	}

	newAlpha := solver.alphaCtrl.Update(eRecon, eTemporal)
	if newAlpha != solver.alpha {
		solver.alpha = newAlpha
		solver.manifold.SetAlpha(newAlpha)
	}

	// 6. Calculate Confidence and determine Active Dynamic Horizon K
	// High confidence (low eRecon) extends horizon up to maxHorizon (20 ticks).
	// Low confidence (high eRecon) collapses horizon back down to 1 tick.
	confidence := math.Max(0.0, math.Min(1.0, 1.0-(eRecon/2.0)))
	activeHorizon := int(math.Max(1.0, math.Floor(float64(solver.maxHorizon)*confidence)))

	// 7. Perform Dynamic Recurrent Rollout for k steps
	forwardCurve := solver.manifold.RolloutTaskPrediction(activeHorizon)

	// 8. Enrich the shared Thesis with predictive coding outcomes, dynamic alpha, and return curve
	solver.enrichThesis(thesis, surprise, energy, latent, forwardCurve, activeHorizon, confidence, layers, solver.alpha)

	// 9. Record audit snapshot if recorder is attached
	if solver.recorder != nil {
		solver.recorder.Write(map[string]any{
			"surprise":      surprise,
			"energy":        energy,
			"latent":        latent,
			"alpha":         solver.alpha,
			"confidence":    confidence,
			"activeHorizon": activeHorizon,
			"forwardCurve":  forwardCurve,
		})
	}

	return nil
}

/*
enrichThesis attaches predictive coding outputs, dynamic forward return curve,
active horizon length, and confidence score back to the shared Thesis.
*/
func (solver *Solver) enrichThesis(
	thesis *types.Thesis,
	surprise float64,
	energy float64,
	latent []float64,
	forwardCurve []float64,
	activeHorizon int,
	confidence float64,
	layers []learning.ResonanceLayerWire,
	alpha float64,
) {
	thesis.Resonance.Store(
		"resonance",
		map[string]any{
			"surprise":      surprise,
			"energy":        energy,
			"latent":        latent,
			"forwardCurve":  forwardCurve,  // []float64 trajectory of predictions [t+1, t+2, ..., t+k]
			"activeHorizon": activeHorizon, // Active extended horizon length k
			"confidence":    confidence,    // Confidence score [0.0, 1.0]
			"layers":        layers,
			"alpha":         alpha,
		},
	)
}

/*
initManifold constructs the CPU predictive coding network.
If an architecture wasn't explicitly supplied, it constructs an adaptive 3-layer
predictive network: [InputDim -> InputDim * 2 -> InputDim].
*/
func (solver *Solver) initManifold(inputDim, targetDim int) error {
	arch := solver.arch
	if len(arch) < 2 {
		// Default 3-layer bottleneck architecture
		arch = []int{inputDim, inputDim * 2, inputDim}
	} else {
		arch[0] = inputDim // Ensure input layer matches actual feature dimension
	}

	tDim := solver.targetDim
	if targetDim > 0 {
		tDim = targetDim
	}

	manifold, err := learning.NewResonanceManifold(arch, tDim, solver.alpha)

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
func (solver *Solver) extractFeatures(thesis *types.Thesis) (input []float64, target []float64) {
	// 1. Try direct vector interface or slice methods if present
	if v, ok := any(thesis).(interface{ Vector() []float64 }); ok {
		input = v.Vector()
	} else if f, ok := any(thesis).(interface{ Features() []float64 }); ok {
		input = f.Features()
	}

	// 2. Fallback: Extract from Thesis measurements/metrics map
	thesis.Measurements.Range(func(key, value any) bool {
		measurement, ok := value.(*types.Measurement)

		if !ok {
			return true // Skip if not a Measurement
		}

		for _, metric := range measurement.Metrics {
			input = append(input, metric.Raw)
		}

		return true
	})

	// 3. Extract supervised target if present
	if t, ok := any(thesis).(interface{ Target() []float64 }); ok {
		target = t.Target()
	}

	return input, target
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
