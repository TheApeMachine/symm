package learning

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
PredictiveCoderConfig configures universal hierarchical predictive coding.
*/
type PredictiveCoderConfig struct {
	// InputDim is the dimension of the sensory input vector.
	InputDim int
	// TargetDim is the number of supervised RLS readout heads (defaults to 1).
	TargetDim int
	// DictionaryDim is the size of the overcomplete sparse layer (e.g. 4x InputDim).
	DictionaryDim int

	// LatentDim is the size of the temporal state layer.
	LatentDim int
	// CustomArch optionally specifies explicit layer sizes (e.g. []int{4, 32, 8}).
	CustomArch []int
	// Target defines the target function (e.g. DirectionalTarget, BinaryTarget, DeltaTarget).
	Target TargetTransform
	// MaxHorizon sets the maximum forward prediction horizon.
	MaxHorizon int
	// Pace sets the adaptive learning rate (defaults to 0.03).
	Pace float64
	// Learn enables continuous online learning (defaults to true).
	Learn bool
}

/*
PredictiveInput carries generic sensory and reference data into the coder.
*/
type PredictiveInput struct {
	// Features is the primary sensory input vector.
	Features []float64
	// Reference is the optional ground-truth signal to predict changes/directions of.
	Reference float64
	// HasReference indicates if Reference is present on this step.
	HasReference bool
	// Step is the sequence index or time step.
	Step int64
	// Time is the continuous timestamp (optional, falls back to Step).
	Time float64
	// Drive is an optional external drive/activity scalar for Hamiltonian dynamics.
	Drive float64
	// Power is an optional supplied power scalar for passivity accounting.
	Power float64
}

/*
PredictiveOutput contains universal representation, direction, rollout, and dynamics metrics.
*/
type PredictiveOutput struct {
	// Target & Directional Output
	Score            float64   // Continuous logit / discriminant score
	Direction        float64   // Discrete decision: +1 (Up), -1 (Down), 0 (Neutral)
	Confidence       float64   // Scaled confidence in [0, 1]
	DegreesOfFreedom float64   // Posterior uncertainty degrees of freedom
	ForwardCurve     []float64 // Projected rollout curve across horizons
	ForwardRetention []float64 // Temporal contraction envelope
	SupportedHorizon int       // Skill-adjusted optimal forecast horizon
	Skill            float64   // Pre-quential skill against baseline (>1 means beating baseline)
	Precision        float64   // Scale-free precision of the readout head
	Calibrated       bool      // Whether the head has enough resolved samples to calibrate

	// Self-Supervised Manifold Diagnostics
	Energy              float64   // Total variational free energy
	Surprise            float64   // Instantaneous novelty / reconstruction error
	ReconstructionError float64   // Sensory reconstruction error norm
	InferenceSteps      int       // Settling iterations to convergence
	Readout             []float64 // Multi-layer latents + innovation residuals [z_1..z_L, e_0..e_{L-1}]

	// Physical Latent Dynamics
	Dynamics types.Frame // Kinematics (Velocity, Acceleration, StoredEnergy, Jumps, Rotor)

	// Resolution Telemetry
	LastResolution *ResolutionOutcome
	ResolvedSteps  int
}

/*
PredictiveCoder provides an end-to-end, domain-agnostic predictive coding reducer.
*/
type PredictiveCoder struct {
	config   PredictiveCoderConfig
	manifold *ResonanceManifold
	ledger   *TemporalLedger
	dynamics *types.Stream

	currentHorizon int
}

/*
NewPredictiveCoder instantiates the complete predictive coder in one line.
*/
func NewPredictiveCoder(config PredictiveCoderConfig) *PredictiveCoder {
	pace := config.Pace
	if pace <= 0 {
		pace = 0.03
	}

	maxHorizon := config.MaxHorizon
	if maxHorizon <= 0 {
		maxHorizon = 8
	}

	target := config.Target
	if target == nil {
		target = DirectionalTarget(0) // Default: Up (+1) / Down (-1)
	}

	arch := config.CustomArch
	if len(arch) == 0 {
		inDim := max(1, config.InputDim)
		dictDim := config.DictionaryDim
		if dictDim <= 0 {
			dictDim = inDim * 4 // Default 4x overcomplete dictionary
		}
		latDim := config.LatentDim
		if latDim <= 0 {
			latDim = max(2, inDim)
		}
		arch = []int{inDim, dictDim, latDim}
	}

	targetDim := config.TargetDim
	if targetDim <= 0 {
		targetDim = 1
	}

	var manifold *ResonanceManifold

	if targetDim <= 1 {
		// Single-target direction head: one supervised RLS row per forward
		// horizon, so the forward curve is a genuine multi-horizon forecast.
		manifold = NewResonanceManifoldWithHorizon(arch, targetDim, maxHorizon, pace)
	} else {
		// Multi-target heads supervise one row per target dimension directly.
		manifold = NewResonanceManifold(arch, targetDim, pace)
	}

	ledger := NewTemporalLedger(maxHorizon, target)
	dynamics := types.NewStream(PredictiveDynamics, types.Frame{})

	return &PredictiveCoder{
		config:         config,
		manifold:       manifold,
		ledger:         ledger,
		dynamics:       dynamics,
		currentHorizon: 1,
	}
}

/*
Step advances the entire predictive coding loop in a single call.
*/
func (pc *PredictiveCoder) Step(input PredictiveInput) (PredictiveOutput, error) {
	if len(input.Features) != pc.manifold.arch[0] {
		return PredictiveOutput{}, fmt.Errorf(
			"predictive coder: expected %d features, got %d",
			pc.manifold.arch[0], len(input.Features),
		)
	}

	// 1. Resolve past pending predictions against incoming reference signal
	var lastResolution *ResolutionOutcome
	if input.HasReference {
		res, err := pc.ledger.Resolve(pc.manifold, input.Step, input.Reference)
		if err != nil {
			return PredictiveOutput{}, err
		}
		lastResolution = res
	}

	// 2. Settle current sensory features into the manifold
	if err := pc.manifold.Settle(input.Features, false); err != nil {
		return PredictiveOutput{}, fmt.Errorf("predictive coder: settle failed: %w", err)
	}

	if pc.config.Learn {
		if err := pc.manifold.Learn(nil); err != nil {
			return PredictiveOutput{}, fmt.Errorf("predictive coder: learn failed: %w", err)
		}
	}

	// 3. Harvest the multi-layer readout, then select the supported horizon
	// from per-horizon prequential skill: the longest contiguous prefix of
	// horizons whose heads are ready and beat the zero-prediction baseline.
	// The head predicts from its prior before any row has resolved evidence,
	// so the default reach is a single tick.
	readout := pc.manifold.ReadoutVector()
	pc.currentHorizon = pc.supportedHorizon()

	supportedHorizon := pc.currentHorizon

	// 4. Generate the per-horizon forward curve and the contraction envelope.
	// Element k of the curve is the cumulative directional prediction over the
	// next k+1 ticks, so the call for the supported horizon is its last element.
	rollouts, err := pc.manifold.RolloutTaskForecast(pc.config.MaxHorizon)
	if err != nil {
		return PredictiveOutput{}, fmt.Errorf("predictive coder: task forecast failed: %w", err)
	}

	if len(rollouts) == 0 {
		return PredictiveOutput{}, fmt.Errorf("predictive coder: task head produced no forecast")
	}

	head := rollouts[supportedHorizon-1]
	score := head.Value
	df := head.DegreesOfFreedom
	confidence := 0.0

	if head.Scale > 0 {
		// Signal-to-noise ratio mapped to [0, 1] confidence
		snr := math.Abs(score) / head.Scale
		confidence = snr / (1.0 + snr)
	}

	direction := 0.0
	if score > 0 {
		direction = 1.0
	} else if score < 0 {
		direction = -1.0
	}

	predictions := make([]float64, len(rollouts))
	for horizonIndex, rollout := range rollouts {
		predictions[horizonIndex] = rollout.Value
	}

	forwardCurve := append([]float64(nil), predictions[:supportedHorizon]...)
	retention := pc.manifold.RolloutRetention(supportedHorizon)

	skill, _ := pc.manifold.TaskSkill()
	precision, _ := pc.manifold.TaskPrecision()
	_, calibrated := pc.manifold.TaskSkillAt(supportedHorizon)

	// 5. Update Hamiltonian physical dynamics
	pos := 0.0
	if len(readout) > 0 {
		for _, v := range readout {
			pos += v
		}
		pos /= float64(len(readout))
	}

	dynTime := input.Time
	if dynTime <= 0 {
		dynTime = float64(input.Step)
	}

	dynInput := types.Frame{}
	dynInput.Put(SymbolDynamicsTime, dynTime)
	dynInput.Put(SymbolDynamicsPosition, pos)
	dynInput.Put(SymbolDynamicsActivity, input.Drive)
	dynInput.Put(SymbolDynamicsExternalPower, input.Power)

	dynOutput := pc.dynamics.Step(dynInput)

	if dynOutput.Err != nil {
		return PredictiveOutput{}, fmt.Errorf("predictive coder: dynamics failed: %w", dynOutput.Err)
	}

	// 6. Issue the per-horizon forecasts into the ledger for future verification.
	// Every horizon is issued so each row's own cumulative target can resolve.
	if input.HasReference {
		pc.ledger.Issue(input.Step, input.Reference, readout, predictions, supportedHorizon)
	}

	return PredictiveOutput{
		Score:               score,
		Direction:           direction,
		Confidence:          confidence,
		DegreesOfFreedom:    df,
		ForwardCurve:        forwardCurve,
		ForwardRetention:    retention,
		SupportedHorizon:    supportedHorizon,
		Skill:               skill,
		Precision:           precision,
		Calibrated:          calibrated,
		Energy:              pc.manifold.Energy(),
		Surprise:            pc.manifold.ReconstructionError(),
		ReconstructionError: pc.manifold.ReconstructionError(),
		InferenceSteps:      pc.manifold.lastInferenceSteps,
		Readout:             readout,
		Dynamics:            dynOutput,
		LastResolution:      lastResolution,
		ResolvedSteps:       pc.ledger.ResolvedCount(),
	}, nil
}

/*
supportedHorizon picks the longest contiguous prefix of horizons whose task rows
are ready and beat the zero-prediction baseline, per the resonance README:
the horizon contracts to the longest contiguous path whose calls beat a coin
flip. Before any row has evidence the head still predicts its prior, so the
supported reach is a single tick.
*/
func (pc *PredictiveCoder) supportedHorizon() int {
	horizon := 1

	for candidate := 1; candidate <= pc.config.MaxHorizon; candidate++ {
		skill, ready := pc.manifold.TaskSkillAt(candidate)

		if !ready || skill <= 1.0 {
			break
		}

		horizon = candidate
	}

	return horizon
}

/*
Manifold returns the underlying ResonanceManifold.
*/
func (pc *PredictiveCoder) Manifold() *ResonanceManifold {
	return pc.manifold
}

/*
Evaluate settles candidate sensory features counterfactually without learning
or advancing temporal state, and returns the multi-target RLS predictions.
*/
func (pc *PredictiveCoder) Evaluate(features []float64) ([]RLSOutput, error) {
	if _, err := pc.manifold.SettleFromBatchOptions(features, nil, false, false); err != nil {
		return nil, fmt.Errorf("predictive coder: counterfactual evaluation failed: %w", err)
	}

	return pc.manifold.RolloutTaskForecast(1)
}

/*
ResolveTargets updates all RLS readout heads on a previously settled input.
*/
func (pc *PredictiveCoder) ResolveTargets(features []float64, targets []float64) error {
	if _, err := pc.manifold.SettleFromBatchOptions(features, targets, true, false); err != nil {
		return fmt.Errorf("predictive coder: resolve targets failed: %w", err)
	}

	return nil
}
