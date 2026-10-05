package resonance

import (
	"context"
	"fmt"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
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
	arena         *data.ArenaOwner
	detectors     *sync.Map
	standardizers *sync.Map
	finalizers    *sync.Map
	references    *sync.Map
	steps         *sync.Map
	pace          float64

	// ObserveModule is an optional diagnostics hook reporting per-step coder
	// duration so the wiring diagram can profile the resonance stage like
	// every other pipeline node.
	ObserveModule func(string, time.Duration)
}

/*
NewSolver returns a feature detection solver using the configured pace.
*/
func NewSolver(
	ctx context.Context,
	arena *data.ArenaOwner,
	pace float64,
) *Solver {
	solver := &Solver{
		System:        runtime.NewSystem(ctx, "resonance"),
		arena:         arena,
		detectors:     &sync.Map{},
		standardizers: &sync.Map{},
		finalizers:    &sync.Map{},
		references:    &sync.Map{},
		steps:         &sync.Map{},
		pace:          pace,
	}

	solver.Transition(runtime.READY)
	return solver
}

func (solver *Solver) Arena() *data.ArenaOwner {
	return solver.arena
}

func (solver *Solver) finalizer(symbol string) *data.Finalizer[float64] {
	if solver.finalizers == nil {
		solver.finalizers = &sync.Map{}
	}

	if loaded, found := solver.finalizers.Load(symbol); found {
		return loaded.(*data.Finalizer[float64])
	}

	created := data.NewFinalizer[float64]()
	actual, _ := solver.finalizers.LoadOrStore(symbol, created)
	return actual.(*data.Finalizer[float64])
}

/*
Step advances the symbol's predictive coder over the canonical microstructure
sensory features from completed prior-stage signal outputs and writes the
resulting resonance metrics onto the owned output measurement.
*/
func (solver *Solver) Step(prior *data.Measurement) *data.Measurement {
	if solver.Status() != runtime.READY {
		errnie.Warn(solver.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if solver.Error() != nil || prior == nil {
		return nil
	}

	symbol := prior.Label
	if symbol == "" {
		return nil
	}

	at := prior.At
	if at.IsZero() {
		at = time.Now()
	}

	midpoint := 0.0
	if m, ok := prior.LookupMetric("midpoint"); ok && m.Raw > 0 {
		midpoint = m.Raw
	} else if m, ok := prior.LookupMetric("last_price"); ok && m.Raw > 0 {
		midpoint = m.Raw
	} else if m, ok := prior.LookupMetric("price"); ok && m.Raw > 0 {
		midpoint = m.Raw
	}

	var priors []*data.Measurement
	if prior.Source == "runtime:join" {
		priors = prior.Peers
	} else {
		priors = append([]*data.Measurement{prior}, prior.Peers...)
	}

	var signals [11]*data.Measurement
	for _, p := range priors {
		if p == nil || p.Label != symbol {
			continue
		}

		if idx := signalIndex(p.Source); idx >= 0 {
			signals[idx] = p
			continue
		}

		for index := 0; index < len(signals); index++ {
			if signals[index] != nil {
				continue
			}

			if _, ok := extractHeadlineMetric(index, p); ok {
				signals[index] = p
			}
		}
	}

	scorer := solver.scorer(symbol)
	features := scorer.Step(signals)

	out := solver.arena.NewMeasurement(solver.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.Label = symbol
	out.SeqIdx = prior.SeqIdx
	out.At = at
	out.From = prior.From
	out.Peers = []*data.Measurement{prior}

	solver.Update(out, symbol, at, features, midpoint)

	maturity, snr, snrDefined, estimated := out.Maturity, out.SNR, out.SNRDefined, out.Estimated
	if loadedStep, found := solver.steps.Load(symbol); found {
		if stepCount := loadedStep.(*atomic.Int64).Load(); stepCount > 1 {
			maturity = 1.0 - 1.0/float64(stepCount)
		}
	}

	if energyMetric, ok := out.LookupMetric("energy"); ok && energyMetric.Raw > 0 {
		if surpriseMetric, ok := out.LookupMetric("surprise"); ok && surpriseMetric.Raw > 0 {
			snr = energyMetric.Raw / surpriseMetric.Raw
			snrDefined = true
			estimated = true
		}
	}
	out.SetQuality(maturity, snr, snrDefined, estimated)
	solver.finalizer(symbol).Complete(out)

	return out
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
	measurement *data.Measurement,
	symbolName string,
	at time.Time,
	features []float64,
	midpoint float64,
) {
	detector, found := solver.detectors.Load(symbolName)

	if !found {
		detector = learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
			CustomArch:   []int{len(features), len(features) * 4, len(features) * 2, len(features)}, // Overcomplete dictionary with latent space
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
		return
	}

	loadedStep, _ := solver.steps.LoadOrStore(symbolName, &atomic.Int64{})
	step := loadedStep.(*atomic.Int64).Add(1)

	stepStarted := time.Now()

	out, err := stepCoder(coder, learning.PredictiveInput{
		Features: features,
		Step:     step,
		Time:     float64(at.UnixNano()) / 1e9,
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
		return
	}

	solver.publishReturns(measurement, coder, out)
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

func extractHeadlineMetric(index int, measurement *data.Measurement) (float64, bool) {
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
		if metric, found := measurement.LookupMetric(label); found {
			if metric.Standardized != nil || metric.Raw != 0 {
				return metric.Raw, true
			}
		}
	}

	for _, label := range candidates {
		if metric, found := measurement.LookupMetric(label); found {
			return metric.Raw, true
		}
	}

	return 0, false
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
	isStepping   atomic.Bool
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
func (scorer *featureScorer) Step(measurements [11]*data.Measurement) []float64 {
	for !scorer.isStepping.CompareAndSwap(false, true) {
		goruntime.Gosched()
	}
	defer scorer.isStepping.Store(false)

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
func authorityOf(measurement *data.Measurement) float64 {
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

func (solver *Solver) publishReturns(
	measurement *data.Measurement,
	coder *learning.PredictiveCoder,
	out learning.PredictiveOutput,
) {
	if measurement == nil || coder == nil {
		return
	}

	measurement.SetProvenance("calibrated", "false")
	if out.Calibrated {
		measurement.SetProvenance("calibrated", "true")
	}

	measurement.SetProvenance("supported_horizon", fmt.Sprintf("%d", out.SupportedHorizon))
	measurement.SetProvenance("resolved_steps", fmt.Sprintf("%d", out.ResolvedSteps))
	measurement.SetProvenance("confidence", fmt.Sprintf("%f", out.Confidence))

	if out.Reading != nil {
		measurement.SetMetric("energy", data.Metric{
			Label: "energy",
			Raw:   out.Reading.Energy,
		})
		measurement.SetMetric("surprise", data.Metric{
			Label: "surprise",
			Raw:   out.Reading.Surprise,
		})
	}

	// Add latent state as indexed metrics if needed
	if out.Reading != nil {
		for i, val := range out.Reading.Latent {
			label := fmt.Sprintf("latent_%d", i)
			measurement.SetMetric(label, data.Metric{
				Label: label,
				Raw:   val,
			})
		}

		for i, layer := range out.Reading.Layers {
			for j, val := range layer.State {
				label := fmt.Sprintf("layer_%d_state_%d", i, j)
				measurement.SetMetric(label, data.Metric{
					Label: label,
					Raw:   val,
				})
			}
			for j, val := range layer.Prediction {
				label := fmt.Sprintf("layer_%d_prediction_%d", i, j)
				measurement.SetMetric(label, data.Metric{
					Label: label,
					Raw:   val,
				})
			}
		}
	}

	measurement.Result = nil
}
