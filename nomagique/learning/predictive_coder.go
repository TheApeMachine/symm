package learning

import (
	"errors"
	"math"
)

/*
PredictiveCoderConfig declares one coder's structure and learning policy.

CustomArch is the layer widths of the underlying manifold. MaxHorizon is how
far ahead the supervised head is allowed to be asked to forecast. Target maps
a reference series into the quantity the head learns. Pace supplies the
adaptive learning rate; when absent the coder runs at the manifold's own.
*/
type PredictiveCoderConfig struct {
	CustomArch []int
	MaxHorizon int
	Target     TargetTransform
	Pace       *PaceController
	Learn      bool
}

/*
PredictiveInput is one observation offered to the coder.

Reference is the series the target transform is computed over, and
HasReference states whether a usable prior reference existed — a first
observation has nothing to forecast against. Step orders observations so a
prediction issued at t can be resolved against the outcome at t+1.
*/
type PredictiveInput struct {
	Features     []float64
	Reference    float64
	HasReference bool
	Step         int64
	Time         float64
}

/*
Resolution records one prediction scored against the outcome that arrived
after it: what the head issued, what actually happened, and the error between.
*/
type Resolution struct {
	Prediction float64
	Target     float64
	Error      float64
	Horizon    int
	Step       int64
}

/*
PredictiveOutput is the coder's reading after one observation.

Calibrated states whether the head has resolved enough predictions for its
readings to mean anything; until then a consumer must not treat the forecast
as evidence.
*/
type PredictiveOutput struct {
	Dynamics         *ResonanceDynamics
	ForwardCurve     []float64
	ForwardRetention []float64
	SupportedHorizon int
	Calibrated       bool
	ResolvedSteps    int
	Readout          []float64
	Confidence       float64
	LastResolution   *Resolution
}

/*
ResonanceDynamics reports the manifold's internal state after settling: how
much free energy remains, how well it reconstructs its input, and how the
temporal operator is tracking.
*/
type ResonanceDynamics struct {
	Energy              float64
	PredictionEnergy    float64
	ReconstructionError float64
	TemporalError       float64
	HasTemporalError    bool
	Alpha               float64
}

/*
pendingPrediction is one issued forecast awaiting the outcome that will score
it. The reference at issue time is retained because the target is a transform
over the change between then and now.
*/
type pendingPrediction struct {
	prediction float64
	reference  float64
	features   []float64
	step       int64
}

/*
PredictiveCoder learns to forecast a transform of a reference series from a
feature vector, by settling a resonance manifold over the features and
training its supervised head against outcomes that actually arrived.

It is an orchestration over three surviving components — the manifold, the
target transform, and the adaptive pace controller — and holds no learning
mathematics of its own.

Predictions are resolved causally: the forecast issued at one step is scored
only once the outcome arrives, so the head is never trained against a target
it was allowed to see.
*/
type PredictiveCoder struct {
	manifold *ResonanceManifold
	target   TargetTransform
	pace     *PaceController
	learn    bool

	horizon  int
	pending  []pendingPrediction
	resolved int
	last     *Resolution
}

/*
NewPredictiveCoder composes a coder over a manifold sized by the declared
architecture.

The supervised head forecasts a single scalar per horizon, so the target
dimension is one. A configuration with no architecture cannot build a
manifold and yields a coder that reports nothing rather than panicking.
*/
func NewPredictiveCoder(config PredictiveCoderConfig) *PredictiveCoder {
	coder := &PredictiveCoder{
		target:  config.Target,
		pace:    config.Pace,
		learn:   config.Learn,
		horizon: config.MaxHorizon,
	}

	if coder.horizon < 1 {
		coder.horizon = 1
	}

	if len(config.CustomArch) == 0 {
		return coder
	}

	alpha := 0.0

	if coder.pace != nil {
		alpha = coder.pace.Alpha()
	}

	coder.manifold = NewResonanceManifoldWithHorizon(
		config.CustomArch,
		1,
		coder.horizon,
		alpha,
	)

	return coder
}

// Manifold exposes the underlying resonance manifold.
func (coder *PredictiveCoder) Manifold() *ResonanceManifold { return coder.manifold }

/*
Step settles the manifold over one observation, resolves whatever predictions
the new outcome has made scorable, and issues a fresh forecast.
*/
func (coder *PredictiveCoder) Step(input PredictiveInput) (PredictiveOutput, error) {
	if coder.manifold == nil {
		return PredictiveOutput{}, errors.New(
			"learning: predictive coder requires an architecture",
		)
	}

	if len(input.Features) == 0 {
		return PredictiveOutput{}, errors.New(
			"learning: predictive coder requires a feature vector",
		)
	}

	if err := coder.manifold.Settle(input.Features, true); err != nil {
		return PredictiveOutput{}, err
	}

	// The pace controller reads how badly the manifold is reconstructing its
	// own input and sets the learning rate from it, so the rate is derived
	// rather than configured.
	if coder.pace != nil {
		alpha := coder.pace.Update(coder.manifold.ReconstructionError())

		if err := coder.manifold.SetAlpha(alpha); err != nil {
			return PredictiveOutput{}, err
		}
	}

	if input.HasReference {
		coder.resolve(input)
	}

	coder.issue(input)

	return coder.read(), nil
}

/*
resolve scores every pending prediction whose outcome has now arrived and
trains the head against it. A target the transform declares undefined is not
a lesson, so the prediction is dropped rather than taught wrongly.
*/
func (coder *PredictiveCoder) resolve(input PredictiveInput) {
	if coder.target == nil || len(coder.pending) == 0 {
		return
	}

	retained := coder.pending[:0]

	for _, pending := range coder.pending {
		horizon := int(input.Step - pending.step)

		if horizon < 1 {
			retained = append(retained, pending)

			continue
		}

		if horizon > coder.horizon {
			continue
		}

		target, defined := coder.target(input.Reference, pending.reference)

		if !defined {
			continue
		}

		if coder.learn {
			// The head is trained on the features it actually saw when it
			// issued the prediction, not on the current ones.
			_ = coder.manifold.ObserveTask(
				horizon,
				pending.features,
				pending.prediction,
				target,
			)
		}

		coder.resolved++
		coder.last = &Resolution{
			Prediction: pending.prediction,
			Target:     target,
			Error:      target - pending.prediction,
			Horizon:    horizon,
			Step:       pending.step,
		}
	}

	coder.pending = retained
}

/*
issue records the head's current forecast so a later outcome can score it.
An observation with no usable reference cannot anchor a target, so nothing is
issued against it.
*/
func (coder *PredictiveCoder) issue(input PredictiveInput) {
	if !input.HasReference {
		return
	}

	predictions := coder.manifold.TaskPrediction()

	if len(predictions) == 0 {
		return
	}

	readout := coder.manifold.ReadoutVector()
	features := make([]float64, len(readout))
	copy(features, readout)

	coder.pending = append(coder.pending, pendingPrediction{
		prediction: predictions[0],
		reference:  input.Reference,
		features:   features,
		step:       input.Step,
	})
}

/*
read assembles the coder's current reading: the forward forecast curve, how
far ahead it is actually supported, and the manifold's own dynamics.
*/
func (coder *PredictiveCoder) read() PredictiveOutput {
	output := PredictiveOutput{
		ResolvedSteps:  coder.resolved,
		Readout:        coder.manifold.ReadoutVector(),
		LastResolution: coder.last,
	}

	forecasts, err := coder.manifold.RolloutTaskForecast(coder.horizon)

	if err == nil {
		output.ForwardCurve = make([]float64, 0, len(forecasts))

		for horizon, forecast := range forecasts {
			output.ForwardCurve = append(output.ForwardCurve, forecast.Value)

			// The supported horizon runs only as far as the head's skill is
			// still established; beyond it the curve is extrapolation.
			if forecast.Ready {
				output.SupportedHorizon = horizon + 1
			}
		}
	}

	output.ForwardRetention = coder.manifold.RolloutRetention(coder.horizon)

	// Confidence is the head's own skill at the first horizon: how much of the
	// target's variation it actually explains, not a declared constant.
	if skill, defined := coder.manifold.TaskSkillAt(1); defined {
		output.Confidence = skill
		output.Calibrated = output.SupportedHorizon > 0
	}

	temporalError, hasTemporal := coder.manifold.TemporalError()

	output.Dynamics = &ResonanceDynamics{
		Energy:              coder.manifold.Energy(),
		PredictionEnergy:    coder.manifold.PredictionEnergy(),
		ReconstructionError: coder.manifold.ReconstructionError(),
		TemporalError:       temporalError,
		HasTemporalError:    hasTemporal,
	}

	if coder.pace != nil {
		output.Dynamics.Alpha = coder.pace.Alpha()
	}

	if math.IsNaN(output.Confidence) || math.IsInf(output.Confidence, 0) {
		output.Confidence = 0
	}

	return output
}

// ResolvedSteps returns how many predictions have been scored against outcomes.
func (coder *PredictiveCoder) ResolvedSteps() int { return coder.resolved }
