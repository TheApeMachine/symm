package resonance

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"

	"github.com/theapemachine/errnie"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Solver orchestrates Predictive Coding across the multi-sensory microstructure stream.

PREDICTIVE CODING PHILOSOPHY:
Predictive Coding (grounded in the Free Energy Principle and hierarchical predictive
processing; Friston 2005, Rao & Ballard 1999) does NOT predict future price. Expecting
a neural manifold to divine directional price outcomes from microstructure metrics is a
crystal-ball fallacy.

Instead, the generative manifold learns the internal structure and joint continuous
co-activation patterns across 11 orthogonal sensory channels.
SURPRISE IS A BREAK IN THE COMMON FLOW:
Top-down expectations continuously predict bottom-up sensory arrivals. When the market
moves in typical concerted flow (e.g. aggressive buying accompanied by order book replenishment,
spread narrowing, and volume cascade), top-down generative predictions cancel incoming sensory
evidence, resulting in minimal prediction error (low Free Energy / low surprise).

Surprise surges when the expected multi-sensory co-activation pattern breaks:
for instance, when aggressive taker flow violently accelerates (A = 0.9) while limit book depth
retreats (B = 0.1) and adverse selection toxicity spikes (C = 0.8) contrary to historical co-occurrence.
This un-cancelled prediction error across the hierarchical manifold IS Surprise. It signals
structural regime rupture, liquidity absorption, iceberg execution, or market dislocation.

THE 11 CANONICAL HEADLINE FEATURES (ORTHOGONAL MICROSTRUCTURE DIMENSIONS):
Resonance ingests exactly one defensible, scale-free headline metric from each of the 11 signal
families, capturing independent microstructural facets without high cross-collinearity:
 0. Correlation (relative_return_energy): Focal asset return variance relative to cohort average (volatility excitation).
 1. LeadLag     (best_lag_correlation): Peak cross-correlation with leading peer (information transmission latency).
 2. Liquidity   (relative_spread): Top-of-book bid-ask spread divided by midpoint (instantaneous immediacy cost).
 3. Sentiment   (advance_fraction): Cross-sectional universe breadth (fraction of advancing assets; systemic consensus).
 4. CVD         (signed_net_fraction): Aggressor flow ratio: net notional divided by gross notional in [-1, 1] (taker flow).
 5. DepthFlow   (observed_notional_imbalance): Mutation activity imbalance in [-1, 1] (maker queue injection vs pull).
 6. Morphology  (book_shape_distance): Wasserstein-1 distance between folded bid/ask depth distributions (book geometry).
 7. Hawkes      (excitation_fraction:buy): Endogenous self-exciting arrival cascade share (momentum feedback / clustering).
 8. PumpDump    (spread_ratio): Relative spread expansion ratio against its adaptive baseline (volume-clock velocity).
 9. Toxicity    (net_withdrawal_fraction:bid): Maker cancellation/retreat rate (defensive flee from adverse selection).
 10. Derivatives (basis): Futures price basis relative to index (institutional leverage / funding pressure).

ADAPTIVE STANDARDIZATION:
Each feature channel is standardized causally through nomagique's adaptive.Baseline (backed
by adaptive.Window). Starting with span 1 on observation #1, each channel normalizes into
empirical z-scores without static windows, hardcoded sigmas, or arbitrary magic constants.
*/
type Solver struct {
	*runtime.System
	detectors     *sync.Map
	standardizers *sync.Map
	references    *sync.Map
	returnNoise   *sync.Map
	steps         *sync.Map
	pace          float64

	// ObserveModule is an optional diagnostics hook reporting per-step coder
	// duration so the wiring diagram can profile the resonance stage like
	// every other pipeline node.
	ObserveModule func(string, time.Duration)
	observe       func(*types.ResonanceArtifact)
}

/*
SetObserver installs the synchronous observer for producer-owned model state.
*/
func (solver *Solver) SetObserver(observer func(*types.ResonanceArtifact)) {
	solver.observe = observer
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
NewSolver returns a feature detection solver using the configured pace.
*/
func NewSolver(
	ctx context.Context,
	pace float64,
) *Solver {
	solver := &Solver{
		System:        runtime.NewSystem(ctx, "resonance"),
		detectors:     &sync.Map{},
		standardizers: &sync.Map{},
		references:    &sync.Map{},
		returnNoise:   &sync.Map{},
		steps:         &sync.Map{},
		pace:          pace,
	}
	solver.Transition(runtime.READY)

	return solver
}

/*
Step advances the symbol's predictive coder over the canonical microstructure
sensory features carried in Peers and writes the resulting resonance metrics
onto the measurement.
*/
func (solver *Solver) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if solver.Status() != runtime.READY {
		errnie.Warn(solver.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if solver.Error() != nil {
		return measurement
	}

	if measurement == nil {
		return nil
	}

	symbol := measurement.Label

	if symbol == "" {
		for _, peer := range measurement.Peers {
			if peer != nil && peer.Label != "" {
				symbol = peer.Label
				break
			}
		}
	}

	if symbol == "" {
		return measurement
	}

	at := measurement.At

	if at.IsZero() {
		for _, peer := range measurement.Peers {
			if peer != nil && !peer.At.IsZero() {
				at = peer.At
				break
			}
		}
	}

	if at.IsZero() {
		at = time.Now()
	}

	midpoint := 0.0

	if measurement.Label == symbol && measurement.Metrics != nil {
		if metric, found := measurement.Metrics["midpoint"]; found && metric.Raw > 0 {
			midpoint = metric.Raw
		}

		if midpoint == 0.0 {
			if metric, found := measurement.Metrics["last_price"]; found && metric.Raw > 0 {
				midpoint = metric.Raw
			}
		}

		if midpoint == 0.0 {
			if metric, found := measurement.Metrics["price"]; found && metric.Raw > 0 {
				midpoint = metric.Raw
			}
		}
	}

	if midpoint == 0.0 {
		for _, peer := range measurement.Peers {
			if peer == nil || peer.Label != symbol || peer.Metrics == nil {
				continue
			}

			if metric, found := peer.Metrics["midpoint"]; found && metric.Raw > 0 {
				midpoint = metric.Raw
				break
			}

			if metric, found := peer.Metrics["last_price"]; found && metric.Raw > 0 {
				midpoint = metric.Raw
				break
			}

			if metric, found := peer.Metrics["price"]; found && metric.Raw > 0 {
				midpoint = metric.Raw
				break
			}
		}
	}

	var signals [11]*data.Measurement[float64]

	if measurement.Label == symbol {
		if idx := signalIndex(measurement.Source); idx >= 0 {
			signals[idx] = measurement
		}
	}

	for _, peer := range measurement.Peers {
		if peer == nil || peer.Label != symbol {
			continue
		}

		if idx := signalIndex(peer.Source); idx >= 0 {
			signals[idx] = peer
		}
	}

	scorer := solver.scorer(symbol)
	features := scorer.Step(signals)

	resonance := solver.Update(symbol, at, features, midpoint)

	if resonance != nil && resonance.Snapshot != nil {
		if energyMetric, ok := measurement.Metrics["energy"]; ok {
			measurement.Metrics["energy"] = energyMetric.Write(resonance.Snapshot.Energy)
		}

		if surpriseMetric, ok := measurement.Metrics["surprise"]; ok {
			measurement.Metrics["surprise"] = surpriseMetric.Write(resonance.Snapshot.Surprise)
		}
	}

	measurement.Label = symbol
	measurement.At = at

	if resonance != nil && solver.observe != nil {
		solver.observe(resonance)
	}

	return measurement
}

func (solver *Solver) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement("resonance", map[string]data.Metric[float64]{
		"energy": data.NewMetric[float64](
			"energy", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"surprise": data.NewMetric[float64](
			"surprise", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
	})

	measurement.Metadata["peer-interest"] = "*"
	return measurement
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
	midpoint float64,
) *types.ResonanceArtifact {
	priorMidpoint := 0.0

	if prior, found := solver.references.Load(symbolName); found {
		priorMidpoint, _ = prior.(float64)
	}

	if midpoint <= 0 && priorMidpoint > 0 {
		midpoint = priorMidpoint
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
			CustomArch:   []int{len(features), len(features) * 4, len(features) * 2, len(features)}, // Overcomplete dictionary with latent space
			MaxHorizon:   10,                                                                        // Forward rollouts to t+10: a next-tick call is not actionable
			Target:       solver.directionalTarget(symbolName),                                      // Noise-scaled directional call
			InitialAlpha: solver.pace,                                                               // Adaptive learning pace
			Learn:        true,
		})
		solver.detectors.Store(symbolName, detector)
	}

	coder, ok := detector.(*learning.PredictiveCoder)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			nil,
		))
		return nil
	}

	hasReference := priorMidpoint > 0 && midpoint > 0
	loadedStep, _ := solver.steps.LoadOrStore(symbolName, &atomic.Int64{})
	step := loadedStep.(*atomic.Int64).Add(1)

	stepStarted := time.Now()

	out, err := stepCoder(coder, learning.PredictiveInput{
		Features:     features,
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

func signalIndex(source string) int {
	prefix := source
	if idx := strings.IndexByte(prefix, ':'); idx >= 0 {
		prefix = prefix[:idx]
	}

	switch prefix {
	case "correlation":
		return 0
	case "leadlag":
		return 1
	case "liquidity":
		return 2
	case "sentiment":
		return 3
	case "cvd":
		return 4
	case "depthflow":
		return 5
	case "morphology":
		return 6
	case "hawkes":
		return 7
	case "pumpdump":
		return 8
	case "toxicity":
		return 9
	case "derivatives":
		return 10
	default:
		return -1
	}
}

func extractHeadlineMetric(index int, measurement *data.Measurement[float64]) (float64, bool) {
	if measurement == nil || measurement.Err != nil || len(measurement.Metrics) == 0 {
		return 0, false
	}

	var candidates []string

	switch index {
	case 0: // Correlation
		candidates = []string{
			"signed_correlation", "cohort_signed_correlation", "covariance",
			"absolute_correlation", "relative_return_energy",
		}
	case 1: // LeadLag
		candidates = []string{
			"best_lag_correlation", "contemporaneous_correlation",
			"absolute_correlation_gain", "lag_fraction",
		}
	case 2: // Liquidity
		candidates = []string{
			"touch_notional_imbalance", "relative_spread", "spread", "depth_zscore:bid",
		}
	case 3: // Sentiment
		candidates = []string{
			"signed_fraction_zscore", "signed_fraction", "signed_median",
			"median_absolute_zscore", "advance_fraction", "breadth", "median_return",
		}
	case 4: // CVD
		candidates = []string{
			"signed_net_fraction_zscore", "signed_net_fraction",
			"cumulative_volume_delta", "net_notional_rate", "signed_count_fraction",
		}
	case 5: // DepthFlow
		candidates = []string{
			"observed_notional_imbalance_zscore", "observed_notional_imbalance",
			"observed_notional_rate_zscore", "observed_notional_rate",
			"mutation_activity_imbalance",
		}
	case 6: // Morphology
		candidates = []string{
			"morphology_change_zscore", "morphology_change",
			"book_shape_distance", "book_shape_ks",
		}
	case 7: // Hawkes
		candidates = []string{
			"branching_spectral_radius", "conditional_intensity:buy",
			"arrival_rate", "event_fraction:buy", "excitation_fraction:buy",
		}
	case 8: // PumpDump
		candidates = []string{
			"spread_zscore", "notional_rate_zscore", "spread_ratio",
			"notional_rate_ratio", "relative_spread",
		}
	case 9: // Toxicity
		candidates = []string{
			"fill_fraction_zscore:bid", "fill_fraction_zscore:ask",
			"net_withdrawal_fraction:bid", "retreat_fraction:bid", "touch_fill_fraction:bid",
		}
	case 10: // Derivatives
		candidates = []string{
			"basis_zscore", "open_interest_growth_zscore", "basis",
			"open_interest_growth_rate", "liquidation_signed_fraction", "log_basis",
		}
	}

	for _, label := range candidates {
		if metric, found := measurement.Metrics[label]; found {
			if metric.Standardized != nil || metric.Raw != 0 {
				return metric.Raw, true
			}
		}
	}

	for _, label := range candidates {
		if metric, found := measurement.Metrics[label]; found {
			return metric.Raw, true
		}
	}

	return 0, false
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
func (solver *Solver) directionalTarget(symbolName string) core.Primitive {
	return &logDirectionalTarget{
		deadband: func() float64 {
			loader, found := solver.returnNoise.Load(symbolName)
			if !found {
				return 0
			}

			tracker, valid := loader.(*returnNoiseTracker)
			if !valid {
				return 0
			}

			if scale, ready := tracker.scale(); ready {
				return scale
			}

			return 0
		},
	}
}

/*
logDirectionalTarget classifies the log return between one issued reference
and its resolved reference, deadbanded live by the symbol's own measured
per-step log-return noise. A call therefore requires the cumulative move to
exceed the symbol's own recent noise, which keeps the same target honest for
symbols priced orders of magnitude apart.
*/
type logDirectionalTarget struct {
	err      error
	deadband func() float64
	out      float64
}

func (op *logDirectionalTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*learning.Observation)(arriving)

			if sample.Current <= 0 || sample.Past <= 0 {
				op.Error(fmt.Errorf(
					"%w: resonance: directional target references must be positive",
					core.ErrDomain,
				))
				return
			}

			logReturn := math.Log(sample.Current / sample.Past)

			if math.Abs(logReturn) <= op.deadband() {
				op.out = 0
			} else if logReturn > 0 {
				op.out = 1
			} else {
				op.out = -1
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *logDirectionalTarget) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
stepCoder drives one observation through the coder primitive and returns its
published reading.
*/
func stepCoder(
	coder core.Primitive,
	input learning.PredictiveInput,
) (learning.PredictiveOutput, error) {
	evaluation := transport.NewEvaluate(coder)
	var output learning.PredictiveOutput

	for out := range evaluation.Next(transport.NewValues(input).Next(nil)) {
		output = *(*learning.PredictiveOutput)(out)
	}

	return output, evaluation.Error()
}

func (solver *Solver) scorer(symbol string) *featureScorer {
	loaded, found := solver.standardizers.Load(symbol)

	if found {
		if s, ok := loaded.(*featureScorer); ok && s != nil {
			return s
		}
	}

	created := newFeatureScorer()
	actual, _ := solver.standardizers.LoadOrStore(symbol, created)
	return actual.(*featureScorer)
}

/*
featureScorer owns the 11 causal adaptive standardizers for one symbol's sensory stream.
Observations advance only on envelopes where the corresponding signal measurement fired,
preventing variance collapse from repeated identical pseudo-observations.
*/
type featureScorer struct {
	mu           sync.Mutex
	pipelines    [11]core.Primitive
	standardized [11]float64
	lastReading  [11]adaptive.BaselineReading
}

func newFeatureScorer() *featureScorer {
	scorer := &featureScorer{}

	for index := range scorer.pipelines {
		scorer.pipelines[index] = adaptive.NewBaseline(adaptive.NewWindow())
	}

	return scorer
}

/*
Step advances only the pipelines for signals that actually produced a valid measurement
on this envelope. Absent signals retain their last standardized z-score without observing,
preventing variance collapse from repeated identical pseudo-observations.
*/
func (scorer *featureScorer) Step(measurements [11]*data.Measurement[float64]) []float64 {
	scorer.mu.Lock()
	defer scorer.mu.Unlock()

	for index, measurement := range measurements {
		if measurement == nil || measurement.Err != nil {
			continue
		}

		val, ok := extractHeadlineMetric(index, measurement)

		if !ok {
			continue
		}

		var reading adaptive.BaselineReading

		for out := range scorer.pipelines[index].Next(transport.NewOne(unsafe.Pointer(&val)).Next(nil)) {
			reading = *(*adaptive.BaselineReading)(out)
		}

		scorer.lastReading[index] = reading
		scorer.standardized[index] = reading.ZScore * authorityOf(measurement) * reading.Maturity
	}

	features := make([]float64, 11)
	copy(features, scorer.standardized[:])
	return features
}

/*
authorityOf derives one measurement's evidence authority weight through the
canonical finalizer, quality, and authority primitives. Finalization runs when
the measurement carries no derived quality yet. A failed derivation inhibits
the observation entirely, matching the old zero-authority behavior.
*/
func authorityOf(measurement *data.Measurement[float64]) float64 {
	if !measurement.SNRDefined && measurement.Maturity == 0 {
		held := measurement

		for range data.NewFinalizer[float64]().Next(transport.NewOne(unsafe.Pointer(&held)).Next(nil)) {
		}
	}

	quality := data.QualityReading{
		SNR:        measurement.SNR,
		SNRDefined: measurement.SNRDefined,
		Estimated:  measurement.Estimated,
		Maturity:   measurement.Maturity,
	}

	evidence := transport.NewEvaluate(data.NewAuthority())
	var authority float64

	for out := range evidence.Next(transport.NewOne(unsafe.Pointer(&quality)).Next(nil)) {
		authority = *(*float64)(out)
	}

	if err := evidence.Error(); err != nil {
		return 0
	}

	return authority
}

/*
resonanceDynamics maps the coder's manifold reading onto the telemetry wire
type. nomagique carries no telemetry types, so the projection happens here at
the domain boundary.
*/
func resonanceDynamics(
	dynamics *learning.ResonanceDynamics,
) *wire.ResonanceDynamicsT {
	if dynamics == nil {
		return nil
	}

	return &wire.ResonanceDynamicsT{
		Ready:            1,
		StoredEnergy:     dynamics.Energy,
		SuppliedPower:    dynamics.PredictionEnergy,
		Dissipation:      dynamics.ReconstructionError,
		PassivityResidue: dynamics.TemporalError,
		MemoryScale:      dynamics.Alpha,
	}
}

/*
publishReturns stores the coder's calibrated outputs on the symbol so the
graph and causal stages can build real predictive evidence. The return
forecast carries the coder's reward prediction; it is a direction readout, not
a priced return. The coder's own manifold snapshot is stored under the symbol
key because that is the slot both downstream solvers load. The forecast and
dynamics are published only once the head is calibrated, so the graph never
sees a fabricated posterior before outcomes exist.
*/
func (solver *Solver) publishReturns(
	symbol string,
	at time.Time,
	coder *learning.PredictiveCoder,
	out learning.PredictiveOutput,
) *types.ResonanceArtifact {
	if symbol == "" || coder == nil {
		return nil
	}

	artifact := types.ResonanceArtifact{
		Symbol:           symbol,
		At:               at,
		Snapshot:         out.Reading,
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

	if len(out.Forecast) > 0 {
		// The last curve element is the supported horizon's cumulative
		// directional prediction, which is the call the artifact carries.
		forecast := out.Forecast[len(out.Forecast)-1]
		call := 0.0

		if forecast.Ready {
			if forecast.Value > 0 {
				call = 1
			}

			if forecast.Value < 0 {
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
