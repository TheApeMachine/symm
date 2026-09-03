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

	// Readout selects what the task head harvests as its features. Every
	// horizon holds a covariance matrix quadratic in this width, so at high
	// MaxHorizon the choice dominates the coder's memory: ReadoutAll is twice
	// as wide as ReadoutLatents and therefore four times the footprint per
	// horizon. The zero value is ReadoutAll, preserving the widest readout.
	Readout ReadoutMode
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
pendingPrediction is one observation's full forecast vector awaiting the
outcomes that will score it.

The head holds an independent model per horizon, so one observation issues a
whole curve at once: predictions[h-1] is what the horizon-h model said would
happen h steps after this observation. Each entry is scored separately, as and
when that many steps have actually elapsed, so a single observation keeps
resolving for as long as its furthest horizon.

The reference and the readout at issue time are retained because the target is
a transform over the change from then to now, and the model must be trained on
the features it actually saw when it committed.
*/
type pendingPrediction struct {
	predictions []float64
	reference   float64
	features    []float64
	step        int64

	// resolved marks which horizons have already been scored, so an
	// observation that stays pending across many steps never trains the same
	// horizon twice.
	resolved []bool
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
	free     []pendingPrediction
	resolved int
	last     *Resolution
}

/*
NewPredictiveCoder composes a coder over a manifold sized by the declared
architecture.

The supervised head forecasts a single scalar per horizon, so the target
dimension is one, and MaxHorizon becomes the number of independent horizon
models the head holds.

The learning rate is never invented here: it comes from the PaceController,
which derives it from how badly the manifold is reconstructing its own input.
A config supplying none gets a default controller rather than a fabricated
constant.
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

	if coder.pace == nil {
		coder.pace = NewPaceController()
	}

	if len(config.CustomArch) == 0 {
		return coder
	}

	coder.manifold = NewResonanceManifoldWithReadout(
		config.CustomArch,
		1,
		coder.horizon,
		coder.pace.Alpha(),
		config.Readout,
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
	// The manifold refuses an architecture it cannot build, so a nil here is a
	// rejected configuration surfacing at its first use rather than a panic.
	if coder.manifold == nil {
		return PredictiveOutput{}, errors.New(
			"learning: predictive coder has no manifold: the architecture was rejected",
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
resolve scores each pending observation at every horizon whose outcome has
now arrived.

The head holds an independent model per horizon, so horizon h is trained only
with the forecast row h issued, judged against what actually happened h steps
later. A row therefore only ever learns from outcomes at its own distance:
row 300 learns from 300-step-ahead moves, never from a next-tick move
relabelled as a long one.

An observation is retained until its furthest horizon has elapsed, so one
observation keeps teaching the curve for as long as it has unresolved rows.
A target the transform declares undefined is not a lesson; that horizon is
marked resolved and skipped rather than taught wrongly.
*/
func (coder *PredictiveCoder) resolve(input PredictiveInput) {
	if coder.target == nil || len(coder.pending) == 0 {
		return
	}

	retained := coder.pending[:0]

	for _, pending := range coder.pending {
		elapsed := int(input.Step - pending.step)

		// The outcome for horizon `elapsed` is exactly this observation, so
		// that is the only new row this step can settle for this pending.
		if elapsed >= 1 && elapsed <= len(pending.resolved) &&
			!pending.resolved[elapsed-1] {
			coder.score(input, pending, elapsed)
			pending.resolved[elapsed-1] = true
		}

		// Retain only while some horizon can still be reached.
		if elapsed < len(pending.resolved) {
			retained = append(retained, pending)

			continue
		}

		coder.recycle(pending)
	}

	coder.pending = retained
}

/*
score trains one horizon row against the outcome that just arrived and records
the resolution.
*/
func (coder *PredictiveCoder) score(
	input PredictiveInput,
	pending pendingPrediction,
	horizon int,
) {
	target, defined := coder.target(input.Reference, pending.reference)

	if !defined {
		return
	}

	prediction := pending.predictions[horizon-1]

	if coder.learn {
		// The row is trained on the readout it actually saw when it committed,
		// never on the current one.
		_ = coder.manifold.ObserveTask(
			horizon,
			pending.features,
			prediction,
			target,
		)
	}

	coder.resolved++

	// LastResolution reports the decision just settled. Several observations
	// can settle in one step, at different horizons; the nearest horizon is the
	// most recently committed decision, so it is the one a consumer scoring
	// "what did the head just get right" needs to see.
	if coder.last == nil || horizon <= coder.last.Horizon {
		coder.last = &Resolution{
			Prediction: prediction,
			Target:     target,
			Error:      target - prediction,
			Horizon:    horizon,
			Step:       pending.step,
		}
	}
}

/*
issue records this observation's whole forecast curve so every horizon can be
scored as its outcome arrives.

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
	pending := coder.take(len(predictions), len(readout))

	copy(pending.predictions, predictions)
	copy(pending.features, readout)

	pending.reference = input.Reference
	pending.step = input.Step

	coder.pending = append(coder.pending, pending)
}

/*
take returns a pending slot sized for this observation, reusing a recycled one
when possible. A coder tracking hundreds of horizons across hundreds of symbols
issues a curve every tick, so these buffers must not be reallocated per step.
*/
func (coder *PredictiveCoder) take(horizons int, readout int) pendingPrediction {
	if count := len(coder.free); count > 0 {
		pending := coder.free[count-1]
		coder.free = coder.free[:count-1]

		if len(pending.predictions) == horizons && len(pending.features) == readout {
			for index := range pending.resolved {
				pending.resolved[index] = false
			}

			return pending
		}
	}

	return pendingPrediction{
		predictions: make([]float64, horizons),
		features:    make([]float64, readout),
		resolved:    make([]bool, horizons),
	}
}

// recycle returns a fully resolved slot to the free list for reuse.
func (coder *PredictiveCoder) recycle(pending pendingPrediction) {
	coder.free = append(coder.free, pending)
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

	// The supported horizon is the CONTIGUOUS run of rows whose skill is
	// established, counted from the nearest. A gap ends it: a distant row that
	// happens to have seen data is not reachable evidence if the rows before it
	// have not, and reporting it would let a consumer trust a curve across a
	// stretch the head has never actually learned.
	for horizon := 1; horizon <= coder.horizon; horizon++ {
		skill, defined := coder.manifold.TaskSkillAt(horizon)

		if !defined {
			break
		}

		output.SupportedHorizon = horizon

		// Confidence is the skill at the nearest horizon: how much of the
		// target's variation the head explains, not a declared constant.
		if horizon == 1 {
			output.Confidence = skill
		}
	}

	output.Calibrated = output.SupportedHorizon > 0

	// The curve runs exactly as far as the head has learned. Rolling out the
	// full declared depth would append untrained rows, which emit near-zero and
	// read downstream as a genuine flat forecast rather than as absent evidence.
	if output.SupportedHorizon > 0 {
		forecasts, err := coder.manifold.RolloutTaskForecast(output.SupportedHorizon)

		if err == nil {
			// A fresh slice each step: a consumer retaining the artifact must
			// not see its curve change underneath it on the next step.
			output.ForwardCurve = make([]float64, len(forecasts))

			for index, forecast := range forecasts {
				output.ForwardCurve[index] = forecast.Value
			}
		}

		output.ForwardRetention = coder.manifold.RolloutRetention(output.SupportedHorizon)
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
