package resonance

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	nmtypes "github.com/theapemachine/symm/nomagique/types"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Solver runs one feature detector per symbol over the standardized measurement
stream. The detector owns the entire predictive loop — settling, learning,
and the overcomplete sparse representation — so the solver is only the loop:
readings in, features shaped, output stored on the symbol.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	status        *runtime.Status
	detectors     *sync.Map
	queues        *sync.Map
	schemas       *sync.Map
	standardizers *sync.Map
	states        *sync.Map
	references    *sync.Map
	returnNoise   *sync.Map
	steps         *sync.Map
	pace          float64

	// ObserveModule is an optional diagnostics hook reporting per-step coder
	// duration so the wiring diagram can profile the resonance stage like
	// every other pipeline node.
	ObserveModule func(string, time.Duration)
	observe       func(*types.Envelope)
}

/*
returnNoiseTracker maintains Welford moments over a symbol's per-step log
returns so the ledger's directional target can require a move larger than the
symbol's own typical step noise before calling a direction. The scale is
estimated per symbol, which keeps the target honest across symbols whose price
levels differ by orders of magnitude.
*/
type returnNoiseTracker struct {
	mean  float64
	m2    float64
	count float64
}

func (tracker *returnNoiseTracker) observe(sample float64) {
	tracker.count++
	delta := sample - tracker.mean
	tracker.mean += delta / tracker.count
	delta2 := sample - tracker.mean
	tracker.m2 += delta * delta2
}

func (tracker *returnNoiseTracker) scale() (float64, bool) {
	if tracker.count < 2 {
		return 0, false
	}

	return math.Sqrt(tracker.m2 / (tracker.count - 1)), true
}

/*
Event is one enqueued ticker observation routed to a single feature detector.
Keying the queue by symbol makes every observation that shares a coder land in
the same lock-free FIFO; a drain goroutine replays that queue serially so the
manifold is only ever touched by one goroutine at a time.
*/
type Event struct {
	symbol   string
	at       time.Time
	features []float64
}

/*
NewSolver returns a feature detection solver using the configured pace.
*/
func NewSolver(
	ctx context.Context,
	pace float64,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	return &Solver{
		ctx:           ctx,
		cancel:        cancel,
		status:        runtime.NewStatus(),
		detectors:     &sync.Map{},
		schemas:       &sync.Map{},
		standardizers: &sync.Map{},
		states:        &sync.Map{},
		references:    &sync.Map{},
		returnNoise:   &sync.Map{},
		steps:         &sync.Map{},
		pace:          pace,
	}
}

func (solver *Solver) Name() string {
	return "resonance"
}

func (solver *Solver) Error() error { return solver.err }

func (solver *Solver) Status() types.Status {
	return types.READY
}

/*
Step advances the symbol's predictive coder over this envelope's ticker
observation and writes the resulting artifact back onto the envelope. An
envelope carrying no TickerData (e.g. a Trade or Level3 envelope) is a no-op.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	envelope.Resonance = solver.StepTicker(envelope.TickerData)

	if envelope.Resonance == nil {
		return envelope
	}

	if solver.observe != nil {
		solver.observe(envelope)
	}

	// The observer has synchronously consumed the live manifold while this
	// handler owns it. Downstream rings retain the scalar artifact, never the
	// mutable coder that the next ticker step will advance.
	envelope.Resonance.Manifold = nil

	return envelope
}

/* SetObserver installs the synchronous observer for producer-owned model state. */
func (solver *Solver) SetObserver(observer func(*types.Envelope)) {
	solver.observe = observer
}

// StepTicker advances one symbol's predictive coder over one ticker observation and returns the resulting artifact.
func (solver *Solver) StepTicker(ticker kraken.TickerData) *types.ResonanceArtifact {
	return solver.Update(
		ticker.Symbol,
		ticker.Timestamp,
		[]float64{
			ticker.Ask.Float64(),
			ticker.Bid.Float64(),
			ticker.AskQty,
			ticker.BidQty,
			ticker.Change.Float64(),
			ticker.ChangePct,
			ticker.High.Float64(),
			ticker.Low.Float64(),
			ticker.Last.Float64(),
			ticker.Volume,
			ticker.Vwap,
		},
	)
}

/*
Update steps one feature detector for one symbol and publishes the settled
output as the symbol-keyed resonance frame the frontend renders: the readout
representation as the latent row, with energy and surprise alongside.

The same pass publishes the coder's manifold, its calibrated return forecast,
and its physical dynamics onto the symbol's resonance map. The downstream
graph and causal solvers read exactly these slots; without them the predictive
readiness gate can never open and the planner would remain structurally flat.
The previous midpoint is retained per symbol so `coder.Step` receives an
honest reference and the temporal ledger actually supervises the task head —
otherwise the skill posterior never calibrates regardless of how long the
stream runs.
*/
func (solver *Solver) Update(
	symbolName string,
	at time.Time,
	features []float64,
) *types.ResonanceArtifact {
	midpoint := midpointOrLast(features)

	priorMidpoint := 0.0

	if prior, found := solver.references.Load(symbolName); found {
		priorMidpoint, _ = prior.(float64)
	}

	// Observe this step's log return so the directional target can require a
	// move beyond the symbol's own recent step noise before calling a direction.
	if midpoint > 0 && priorMidpoint > 0 {
		trackerLoader, _ := solver.returnNoise.LoadOrStore(symbolName, &returnNoiseTracker{})
		trackerLoader.(*returnNoiseTracker).observe(math.Log(midpoint / priorMidpoint))
	}

	detector, found := solver.detectors.Load(symbolName)
	if !found {
		detector = learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
			CustomArch: []int{len(features), len(features) * 4, len(features) * 2, len(features)},  // Overcomplete dictionary with latent space
			MaxHorizon: 300,                                                                        // Multi-step forward rollouts up to t+8
			Target:     solver.directionalTarget(symbolName),                                       // Noise-scaled directional call
			Pace:       learning.NewPaceController(learning.PaceConfig{InitialAlpha: solver.pace}), // Adaptive learning pace
			Learn:      true,
		})
		solver.detectors.Store(symbolName, detector)
	}

	coder, ok := detector.(*learning.PredictiveCoder)

	if !ok || coder == nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			nil,
		))
		return nil
	}

	standardized, stdErr := solver.standardize(symbolName, features)

	if stdErr != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: standardization failed for %s", symbolName),
			stdErr,
		))
		return nil
	}

	hasReference := priorMidpoint > 0
	loadedStep, _ := solver.steps.LoadOrStore(symbolName, &atomic.Int64{})
	step := loadedStep.(*atomic.Int64).Add(1)

	stepStarted := time.Now()

	out, err := coder.Step(learning.PredictiveInput{
		Features:     standardized,
		Reference:    midpoint,
		HasReference: hasReference,
		Step:         step,
		Time:         float64(at.UnixNano()) / 1e9,
	})

	if solver.ObserveModule != nil {
		solver.ObserveModule("resonance", time.Since(stepStarted))
	}

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			err,
		))
		return nil
	}

	if midpoint > 0 {
		solver.references.Store(symbolName, midpoint)
	}

	return solver.publishReturns(symbolName, at, coder, out)
}

/*
midpointOrLast extracts an honest reference price from the standardized feature
row. The ticker feature layout is [Ask, Bid, AskQty, BidQty, Change, ChangePct,
High, Low, Last, Volume, Vwap]. The midpoint of the live touch is preferred;
when that is not available the last traded price is used as the reference.
*/
func midpointOrLast(features []float64) float64 {
	if len(features) >= 9 {
		if features[0] > 0 && features[1] > 0 {
			return (features[0] + features[1]) / 2
		}

		if features[8] > 0 {
			return features[8]
		}
	}

	return 0
}

/*
directionalTarget returns the ledger target transform for one symbol: a
directional call on the log return over the resolved horizon, deadbanded by one
typical per-step log-return move. A call therefore requires the cumulative move
to exceed the symbol's own recent noise, which keeps the same target honest for
symbols priced orders of magnitude apart. Before the noise estimate firms up the
deadband is zero and the head learns raw direction, which recursive least
squares averages out.
*/
func (solver *Solver) directionalTarget(symbolName string) learning.TargetTransform {
	return func(current float64, past float64) (float64, bool) {
		if math.IsNaN(current) || math.IsInf(current, 0) ||
			math.IsNaN(past) || math.IsInf(past, 0) ||
			current <= 0 || past <= 0 {
			return 0, false
		}

		logReturn := math.Log(current / past)
		deadband := 0.0

		if loader, found := solver.returnNoise.Load(symbolName); found {
			if tracker, valid := loader.(*returnNoiseTracker); valid {
				if scale, ready := tracker.scale(); ready {
					deadband = scale
				}
			}
		}

		if math.Abs(logReturn) <= deadband {
			return 0, true
		}

		if logReturn > 0 {
			return 1, true
		}

		return -1, true
	}
}

/*
standardize z-scores one symbol's feature row per feature, using Welford
moments retained in one nomagique.Number per feature width. Raw ticker fields
span wildly different magnitudes (price vs quantity vs percentage), so the
manifold is fed adaptive z-scores rather than heterogeneous magnitudes; the
target reference stays in raw price so delayed resolution remains wall-clock
honest.

Each feature occupies its own namespaced adaptive.Standardizer primitive inside
the Number, so the running moments persist in the Number's committed frame and
advance only when an observation is real. A failed standardizer measurement now
propagates as the frame's Err rather than a silent zero.
*/
func (solver *Solver) standardize(
	symbolName string,
	features []float64,
) ([]float64, error) {
	if len(features) == 0 {
		return features, nil
	}

	width := len(features)
	key := symbolName + "\x00" + strconv.Itoa(width)

	loaded, _ := solver.standardizers.LoadOrStore(key, newFeatureScorer(width))
	scorer, valid := loaded.(*featureScorer)

	if !valid || scorer == nil {
		scorer = newFeatureScorer(width)
		solver.standardizers.Store(key, scorer)
	}

	return scorer.Score(features), nil
}

/*
featureScorer standardizes a feature vector, one causal estimator per slot.

Each slot owns its own estimator, so a feature's running moments cannot be
contaminated by another's. Scoring is causal: a feature is measured against
the moments its slot showed BEFORE it, so a burst of near-identical values
cannot collapse its own scale and blow the score up.
*/
type featureScorer struct {
	slots        []equation.CausalResidual
	pipelines    []nomagique.Pipeline
	standardized []float64
}

func newFeatureScorer(width int) *featureScorer {
	scorer := &featureScorer{
		slots:        make([]equation.CausalResidual, width),
		pipelines:    make([]nomagique.Pipeline, width),
		standardized: make([]float64, width),
	}

	for index := range scorer.slots {
		scorer.pipelines[index] = *nomagique.Number(&nomagique.Chain{
			A: &scorer.slots[index],
			B: calculus.Finite{},
		})
	}

	return scorer
}

/*
Score steps every slot's pipeline once and returns the standardized vector.
The retained slice is reused, so a steady-state score does not allocate.
*/
func (scorer *featureScorer) Score(features []float64) []float64 {
	for index, value := range features {
		if index >= len(scorer.pipelines) {
			break
		}

		scorer.pipelines[index].Step(nmtypes.Number(value))
		scorer.standardized[index] = float64(scorer.slots[index].ZScore())
	}

	return scorer.standardized
}

/*
resonanceDynamics maps the coder's manifold reading onto the telemetry wire
type. nomagique carries no telemetry types, so the projection happens here at
the domain boundary.
*/
func resonanceDynamics(
	dynamics *learning.ResonanceDynamics,
) *wire.EnvelopeResonanceDynamicsT {
	if dynamics == nil {
		return nil
	}

	return &wire.EnvelopeResonanceDynamicsT{
		Ready:            1,
		StoredEnergy:     dynamics.Energy,
		SuppliedPower:    dynamics.PredictionEnergy,
		Dissipation:      dynamics.ReconstructionError,
		PassivityResidue: dynamics.TemporalError,
		MemoryScale:      dynamics.Alpha,
	}
}

/*
publishReturns stores the manifold and its calibrated outputs on the symbol so
the graph and causal stages can build real predictive evidence. The return
forecast carries the coder's reward prediction; it is a direction readout, not
a priced return. The manifold is stored under the symbol key because that is
the slot both downstream solvers load. The forecast and dynamics are published
only once the head is calibrated, so the graph never sees a fabricated
posterior before outcomes exist.
*/
func (solver *Solver) publishReturns(
	symbol string,
	at time.Time,
	coder *learning.PredictiveCoder,
	out learning.PredictiveOutput,
) *types.ResonanceArtifact {
	if symbol == "" || coder == nil || coder.Manifold() == nil {
		return nil
	}

	artifact := types.ResonanceArtifact{
		Symbol:           symbol,
		At:               at,
		Manifold:         coder.Manifold(),
		Dynamics:         resonanceDynamics(out.Dynamics),
		ForwardCurve:     out.ForwardCurve,
		ForwardRetention: out.ForwardRetention,
		SupportedHorizon: out.SupportedHorizon,
		Calibrated:       out.Calibrated,
		ResolvedSteps:    out.ResolvedSteps,
		Readout:          out.Readout,
		Confidence:       out.Confidence,
	}

	if out.LastResolution != nil {
		artifact.LastResolutionPrediction = out.LastResolution.Prediction
		artifact.LastResolutionTarget = out.LastResolution.Target
		artifact.LastResolutionError = out.LastResolution.Error
	}

	// The coder produces a prior-based forecast on its very first step, so the
	// artifact always carries the current call. Calibration stays truthful on
	// the artifact and downstream modules weigh it themselves; the solver never
	// withholds an output just because the head has not calibrated yet.
	horizon := max(out.SupportedHorizon, 1)

	forecasts, err := coder.Manifold().RolloutTaskForecast(horizon)

	if err == nil && len(forecasts) > 0 {
		// The last curve element is the supported horizon's cumulative
		// directional prediction, which is the call the artifact carries.
		forecast := forecasts[len(forecasts)-1]
		call := 0.0

		if forecast.Ready {
			if forecast.Value > 0 {
				call = 1
			} else if forecast.Value < 0 {
				call = -1
			}
		}

		artifact.Forecast = &types.ResonanceReturnForecast{
			Distribution:  forecast,
			Horizon:       horizon,
			CandidateCall: call,
			Call:          call,
			StableCall:    call,
		}
	}

	return &artifact
}

/*
Close stops the solver context.
*/
func (solver *Solver) Close() error {
	if solver.cancel != nil {
		solver.cancel()
	}

	return nil
}
