package learning

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
PredictiveCoderConfig declares one coder's structure and learning policy.

CustomArch is the layer widths of the underlying manifold. MaxHorizon is how
far ahead the supervised head is allowed to be asked to forecast. Target maps
a reference series into the quantity the head learns. Pace supplies the
adaptive learning rate; when absent the coder runs at the manifold's own.
*/
type PredictiveCoderConfig struct {
	CustomArch   []int
	MaxHorizon   int
	Target       TargetTransform
	Pace         *Pace
	InitialAlpha float64
	Learn        bool

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
	pace     *Pace
	alpha    float64
	learn    bool

	horizon int
	ledger  *TemporalLedger
	last    *Resolution
}

/*
NewPredictiveCoder composes a coder over a manifold sized by the declared
architecture.

The supervised head forecasts a single scalar per horizon, so the target
dimension is one, and MaxHorizon becomes the number of independent horizon
models the head holds.

The learning rate is never invented here: it comes from the configured Primitive pace graph,
which derives it from how badly the manifold is reconstructing its own input.
A config supplying none gets a default controller rather than a fabricated
constant.
*/
func NewPredictiveCoder(config PredictiveCoderConfig) *PredictiveCoder {
	coder := &PredictiveCoder{
		target:  config.Target,
		pace:    config.Pace,
		alpha:   config.InitialAlpha,
		learn:   config.Learn,
		horizon: config.MaxHorizon,
	}

	if coder.horizon < 1 {
		coder.horizon = 1
	}

	coder.ledger = NewTemporalLedger(coder.horizon, coder.target)

	if coder.alpha == 0 {
		coder.alpha = 0.03
	}

	if coder.pace == nil {
		coder.pace = NewPace(PaceConfig{
			Rest: coder.alpha, Lower: 0.005, Upper: 0.150,
			Gain: 0.1, Band: 0.2, Window: 256,
		})
	}

	if len(config.CustomArch) == 0 {
		return coder
	}

	coder.manifold = NewResonanceManifoldWithReadout(
		config.CustomArch,
		1,
		coder.horizon,
		coder.alpha,
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

	if coder.learn {
		if err := coder.manifold.Settle(input.Features, false); err != nil {
			return PredictiveOutput{}, err
		}

		if err := coder.manifold.Learn(nil); err != nil {
			return PredictiveOutput{}, err
		}
	}

	if !coder.learn {
		if err := coder.manifold.Settle(input.Features, true); err != nil {
			return PredictiveOutput{}, err
		}
	}

	// The pace controller reads how badly the manifold is reconstructing its
	// own input and sets the learning rate from it, so the rate is derived
	// rather than configured.
	if coder.pace != nil {
		reading, err := transport.Evaluate(coder.pace, transport.Values(coder.manifold.ReconstructionError()))
		if err != nil {
			return PredictiveOutput{}, err
		}
		coder.alpha = reading.Alpha

		if err := coder.manifold.SetAlpha(coder.alpha); err != nil {
			return PredictiveOutput{}, err
		}
	}

	if input.HasReference && input.Reference > 0 {
		if coder.learn {
			outcome, err := coder.ledger.Resolve(coder.manifold, input.Step, input.Reference)
			if err != nil {
				return PredictiveOutput{}, err
			}

			if outcome != nil {
				coder.last = &Resolution{
					Prediction: outcome.Prediction,
					Target:     outcome.Target,
					Error:      outcome.Error,
					Horizon:    outcome.Horizon,
					Step:       outcome.Step,
				}
			}
		}

		predictions := coder.manifold.TaskPrediction()
		readout := coder.manifold.ReadoutVector()

		if len(predictions) > 0 && len(readout) > 0 {
			coder.ledger.Issue(input.Step, input.Reference, readout, predictions, coder.horizon)
		}
	}

	return coder.read(), nil
}

/*
read assembles the coder's current reading: the forward forecast curve, how
far ahead it is actually supported, and the manifold's own dynamics.
*/
func (coder *PredictiveCoder) read() PredictiveOutput {
	output := PredictiveOutput{
		ResolvedSteps:  coder.ledger.TotalResolutions(),
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
		output.Dynamics.Alpha = coder.alpha
	}

	return output
}

// ResolvedSteps returns how many predictions have been scored against outcomes.
func (coder *PredictiveCoder) ResolvedSteps() int { return coder.ledger.TotalResolutions() }

// PendingCount returns how many observations are currently awaiting outcome resolution.
func (coder *PredictiveCoder) PendingCount() int {
	if coder.ledger == nil {
		return 0
	}

	return len(coder.ledger.pending)
}
