package learning

import (
	context "context"
	"errors"

	capnp "capnproto.org/go/capnp/v3"
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
	TargetName   string
	Deadband     float64
	UsePace      bool
	Rest         float64
	Lower        float64
	Upper        float64
	Gain         float64
	Band         float64
	Window       int
	InitialAlpha float64
	Learn        bool

	// elects what the task head harvests as its features. Every
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
PredictiveOutput is the coder's reading after one observation.

Calibrated states whether the head has resolved enough predictions for its
readings to mean anything; until then a consumer must not treat the forecast
as evidence. Reading carries the manifold's own settled snapshot, and Forecast
is the head's rollout at the supported horizon.
*/
type PredictiveOutput struct {
	Dynamics         *ResonanceDynamics
	Reading          *ManifoldReading
	Forecast         []RLSOutput
	ForwardCurve     []float64
	ForwardRetention []float64
	SupportedHorizon int
	Calibrated       bool
	ResolvedSteps    int
	Pending          int
	Readout          []float64
	Confidence       float64
	LastResolution   *Resolution
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
type PredictiveCoderServer struct {
	DownstreamPredictiveCoder func(context.Context, PredictiveOutput) error
	manifold                  *ResonanceManifoldServer

	// Embedded atoms for math
	directionalTarget *DirectionalTargetServer
	binaryTarget      *BinaryTargetServer
	identityTarget    *IdentityTargetServer
	deltaTarget       *DeltaTargetServer
	ratioTarget       *RatioTargetServer
	targetName        string

	pace  *PaceServer
	alpha float64
	learn bool

	horizon  int
	ledger   *TemporalLedgerServer
	last     *Resolution
	pending  int
	resolved int
	out      PredictiveOutput
	err      error
}

/*
NewPredictiveCoder composes a coder over a manifold sized by the declared
architecture.

The supervised head forecasts a single scalar per horizon, so the target
dimension is one, and MaxHorizon becomes the number of independent horizon
models the head holds.

The learning rate is never invented here: it comes from the configured
Primitive pace graph, which derives it from how badly the manifold is
reconstructing its own input. A config supplying none gets a default
controller rather than a fabricated constant.
*/
func NewPredictiveCoderServer(config PredictiveCoderConfig) *PredictiveCoderServer {
	coder := &PredictiveCoderServer{
		targetName: config.TargetName,
		alpha:      config.InitialAlpha,
		learn:      config.Learn,
		horizon:    config.MaxHorizon,
	}

	if coder.horizon < 1 {
		coder.horizon = 1
	}

	if coder.alpha == 0 {
		coder.alpha = 0.03
	}

	if config.UsePace {
		coder.pace = &PaceServer{
			Rest:   config.Rest,
			Lower:  config.Lower,
			Upper:  config.Upper,
			Gain:   config.Gain,
			Band:   config.Band,
			Window: config.Window,
		}
	} else {
		// Default pace controller if not explicitly skipped but UsePace is false?
		// We'll assume the caller passes UsePace=true if they want it.
		// For backward compatibility with the old types.Value stub:
		coder.pace = &PaceServer{
			Rest:   coder.alpha,
			Lower:  0.005,
			Upper:  0.150,
			Gain:   0.1,
			Band:   0.2,
			Window: 256,
		}
	}

	switch config.TargetName {
	case "Directional":
		coder.directionalTarget = &DirectionalTargetServer{Deadband: config.Deadband}
	case "Binary":
		coder.binaryTarget = &BinaryTargetServer{}
	case "Identity":
		coder.identityTarget = &IdentityTargetServer{}
	case "Delta":
		coder.deltaTarget = &DeltaTargetServer{}
	case "Ratio":
		coder.ratioTarget = &RatioTargetServer{}
	default:
		// Default to Directional with 0 deadband for safety
		coder.directionalTarget = &DirectionalTargetServer{Deadband: 0}
		coder.targetName = "Directional"
	}

	if len(config.CustomArch) == 0 {
		return coder
	}

	coder.manifold = NewResonanceManifoldServer(
		config.CustomArch,
		1,
		coder.horizon,
		coder.alpha,
		config.Readout,
	)

	coder.ledger = NewTemporalLedgerServer(coder.horizon, coder.manifold, coder)

	return coder
}

/*
Step receives a PredictiveInput, settles the manifold, resolves pending predictions,
issues a fresh forecast, and yields the PredictiveOutput.
*/
func (predictiveCoder *PredictiveCoderServer) Write(ctx context.Context, call PredictiveCoder_write) error {
	args := call.Args()
	featuresList, _ := args.Features()
	features := make([]float64, featuresList.Len())
	for i := 0; i < featuresList.Len(); i++ {
		features[i] = featuresList.At(i)
	}

	input := PredictiveInput{
		Features:     features,
		Reference:    args.Reference(),
		HasReference: args.HasReference(),
		Step:         args.Step(),
		Time:         args.Time(),
	}
	out, err := predictiveCoder.step(input)
	if err != nil {
		predictiveCoder.err = err
		return err
	}
	if predictiveCoder.DownstreamPredictiveCoder != nil {
		return predictiveCoder.DownstreamPredictiveCoder(ctx, out)
	}
	return nil
}

func (predictiveCoder *PredictiveCoderServer) Done(ctx context.Context, call PredictiveCoder_done) error {
	return nil
}

func (predictiveCoder *PredictiveCoderServer) Error() error {
	return predictiveCoder.err
}

/*
step settles the manifold over one observation, resolves whatever predictions
the new outcome has made scorable, and issues a fresh forecast.
*/
func (predictiveCoder *PredictiveCoderServer) step(input PredictiveInput) (PredictiveOutput, error) {
	// The manifold refuses an architecture it cannot build, so a nil here is a
	// rejected configuration surfacing at its first use rather than a panic.
	if predictiveCoder.manifold == nil {
		return PredictiveOutput{}, errors.New(
			"learning: predictive coder has no manifold: the architecture was rejected",
		)
	}

	if len(input.Features) == 0 {
		return PredictiveOutput{}, errors.New(
			"learning: predictive coder requires a feature vector",
		)
	}

	settled, err := predictiveCoder.manifoldExecute(ManifoldCommand{
		Batch: &BatchIntent{
			Input:           input.Features,
			Learn:           predictiveCoder.learn,
			AdvanceTemporal: !predictiveCoder.learn,
		},
	})

	if err != nil {
		return PredictiveOutput{}, err
	}

	// The pace controller reads how badly the manifold is reconstructing its
	// own input and sets the learning rate from it, so the rate is derived
	// rather than configured.
	if predictiveCoder.pace != nil {
		var alpha float64
		predictiveCoder.pace.DownstreamPace = func(c context.Context, v float64) error {
			alpha = v
			return nil
		}

		_, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
		args, _ := NewPace_write_Params(seg)
		args.SetErrorMagnitude(settled.ReconstructionError)
		predictiveCoder.pace.WriteParams(context.Background(), args)

		predictiveCoder.alpha = alpha

		settled, err = predictiveCoder.manifoldExecute(ManifoldCommand{
			Alpha: &AlphaIntent{Alpha: predictiveCoder.alpha},
		})

		if err != nil {
			return PredictiveOutput{}, err
		}
	}

	if input.HasReference && input.Reference > 0 {
		if predictiveCoder.learn {
			ledgerReading, err := predictiveCoder.ledgerExecute(LedgerCommand{
				Resolve: &ResolveIntent{
					Step:      input.Step,
					Reference: input.Reference,
				},
			})

			if err != nil {
				return PredictiveOutput{}, err
			}

			predictiveCoder.pending = ledgerReading.Pending
			predictiveCoder.resolved = ledgerReading.Total

			if ledgerReading.Outcome != nil {
				predictiveCoder.last = &Resolution{
					Prediction: ledgerReading.Outcome.Prediction,
					Target:     ledgerReading.Outcome.Target,
					Error:      ledgerReading.Outcome.Error,
					Horizon:    ledgerReading.Outcome.Horizon,
					Step:       ledgerReading.Outcome.Step,
				}
			}
		}

		settled, err = predictiveCoder.manifoldExecute(ManifoldCommand{
			Reading: &ReadingIntent{},
		})

		if err != nil {
			return PredictiveOutput{}, err
		}

		if len(settled.TaskPrediction) > 0 && len(settled.Readout) > 0 {
			ledgerReading, err := predictiveCoder.ledgerExecute(LedgerCommand{
				Issue: &IssueIntent{
					Step:        input.Step,
					Reference:   input.Reference,
					Features:    settled.Readout,
					Predictions: settled.TaskPrediction,
					Horizon:     predictiveCoder.horizon,
				},
			})

			if err != nil {
				return PredictiveOutput{}, err
			}

			predictiveCoder.pending = ledgerReading.Pending
		}
	}

	return predictiveCoder.read(settled), nil
}

/*
read assembles the coder's reading from the manifold's settled snapshot: the
forward forecast curve, how far ahead it is actually supported, and the
manifold's own dynamics.
*/
func (predictiveCoder *PredictiveCoderServer) read(settled ManifoldReading) PredictiveOutput {
	output := PredictiveOutput{
		Reading:        &settled,
		Readout:        settled.Readout,
		LastResolution: predictiveCoder.last,
	}

	output.ResolvedSteps = predictiveCoder.resolved
	output.Pending = predictiveCoder.pending

	// The supported horizon is the CONTIGUOUS run of rows whose skill is
	// established, counted from the nearest. A gap ends it: a distant row that
	// happens to have seen data is not reachable evidence if the rows before it
	// have not, and reporting it would let a consumer trust a curve across a
	// stretch the head has never actually learned.
	for horizon := 1; horizon <= predictiveCoder.horizon; horizon++ {
		if horizon > len(settled.SkillReady) || !settled.SkillReady[horizon-1] {
			break
		}

		output.SupportedHorizon = horizon

		// Confidence is the skill at the nearest horizon: how much of the
		// target's variation the head explains, not a declared constant.
		if horizon == 1 {
			output.Confidence = settled.Skill[0]
		}
	}

	output.Calibrated = output.SupportedHorizon > 0

	// The curve runs exactly as far as the head has learned. Rolling out the
	// full declared depth would append untrained rows, which emit near-zero and
	// read downstream as a genuine flat forecast rather than as absent evidence.
	horizon := max(output.SupportedHorizon, 1)

	forecast, err := predictiveCoder.manifoldExecute(ManifoldCommand{
		Forecast: &ForecastIntent{Steps: horizon},
	})

	if err == nil && len(forecast.Forecast) > 0 {
		// A fresh slice each step: a consumer retaining the artifact must
		// not see its curve change underneath it on the next step.
		output.Forecast = forecast.Forecast
	}

	if output.SupportedHorizon > 0 {
		output.ForwardCurve = make([]float64, output.SupportedHorizon)

		for index, forecast := range output.Forecast {
			output.ForwardCurve[index] = forecast.Value
		}

		retention, err := predictiveCoder.manifoldExecute(ManifoldCommand{
			Retention: &RetentionIntent{Steps: output.SupportedHorizon},
		})

		if err == nil {
			output.ForwardRetention = retention.Retention
		}
	}

	output.Dynamics = &ResonanceDynamics{
		Energy:              settled.Energy,
		PredictionEnergy:    settled.PredictionEnergy,
		ReconstructionError: settled.ReconstructionError,
		TemporalError:       settled.TemporalError,
		HasTemporalError:    settled.HasTemporalError,
	}

	if predictiveCoder.pace != nil {
		output.Dynamics.Alpha = predictiveCoder.alpha
	}

	return output
}

/*
manifoldExecute drives one manifold command and returns its reading.
*/
func (predictiveCoder *PredictiveCoderServer) manifoldExecute(
	command ManifoldCommand,
) (ManifoldReading, error) {
	if predictiveCoder.manifold == nil {
		return ManifoldReading{}, errors.New("learning: predictive coder has no manifold")
	}

	return predictiveCoder.manifold.Execute(command)
}

/*
ledgerExecute drives one ledger command and returns its reading.
*/
func (predictiveCoder *PredictiveCoderServer) ledgerExecute(
	command LedgerCommand,
) (LedgerReading, error) {
	if predictiveCoder.ledger == nil {
		return LedgerReading{}, errors.New("learning: predictive coder has no ledger")
	}
	return predictiveCoder.ledger.Execute(command)
}
