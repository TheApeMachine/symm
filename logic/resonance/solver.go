package resonance

import (
	"context"
	"fmt"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
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
 1. LeadLag     (covariance_score_gain_median): Median gain in |covariance score| from lag alignment across defined peers.
 2. Liquidity   (relative_spread): Top-of-book bid-ask spread divided by midpoint (instantaneous immediacy cost).
 3. Sentiment   (advance_fraction): Cross-sectional universe breadth (fraction of advancing assets; systemic consensus).
 4. CVD         (signed_net_fraction): Aggressor flow ratio: net notional divided by gross notional in [-1, 1] (taker flow).
 5. DepthFlow   (book_imbalance): Displayed book notional imbalance in [-1, 1] (maker queue injection vs pull).
 6. Morphology  (book_shape_distance): Wasserstein-1 distance between folded bid/ask depth distributions (book geometry).
 7. Hawkes      (excitation_fraction:buy): Endogenous self-exciting arrival cascade share (momentum feedback / clustering).
 8. PumpDump    (spread_ratio): Relative spread expansion ratio against its adaptive baseline (volume-clock velocity).
 9. Toxicity    (net_withdrawal_fraction:bid): Maker cancellation/retreat rate (defensive flee from adverse selection).
 10. Derivatives (basis): Futures price basis relative to index (institutional leverage / funding pressure).

ADAPTIVE STANDARDIZATION:
Each feature channel is standardized causally through an Estimator + CausalResidual pair.
Observations advance only on envelopes where the corresponding signal measurement fired,
preventing variance collapse from repeated identical pseudo-observations.
*/
type Solver struct {
	*runtime.System
	detectors     *sync.Map
	standardizers *sync.Map
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
	pace float64,
) *Solver {
	solver := &Solver{
		System:        runtime.NewSystem(ctx, "resonance"),
		detectors:     &sync.Map{},
		standardizers: &sync.Map{},
		references:    &sync.Map{},
		steps:         &sync.Map{},
		pace:          pace,
	}

	solver.Transition(runtime.READY)
	return solver
}

/*
Step advances the symbol's predictive coder over the canonical microstructure
sensory features from completed prior-stage signal outputs and writes the
resulting resonance metrics onto a fresh owned Measurement in one Write.
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
		solver.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("resonance: prior for %s has no venue time (At)", symbol),
			nil,
		))
		return nil
	}

	midpoint := firstPositive(prior, "midpoint", "last_price", "price", "last")

	var priors []*data.Measurement
	if prior.Source == "runtime:join" {
		priors = prior.Peers()
	} else {
		priors = append([]*data.Measurement{prior}, prior.Peers()...)
	}

	var signals [11]*data.Measurement
	for _, peer := range priors {
		if peer == nil || peer.Label != symbol {
			continue
		}

		if idx := signalIndex(peer.Source); idx >= 0 {
			signals[idx] = peer
			continue
		}

		for index := 0; index < len(signals); index++ {
			if signals[index] != nil {
				continue
			}

			if _, ok := extractHeadlineMetric(index, peer); ok {
				signals[index] = peer
			}
		}
	}

	features := solver.scorer(symbol).Step(signals)

	loadedStep, _ := solver.steps.LoadOrStore(symbol, &atomic.Int64{})
	step := loadedStep.(*atomic.Int64).Add(1)

	coder := solver.coder(symbol, len(features))
	if coder == nil {
		return nil
	}

	hasReference := 0.0
	if _, held := solver.references.Load(symbol); held {
		hasReference = 1
	}
	if midpoint > 0 {
		solver.references.Store(symbol, midpoint)
	}

	stepStarted := time.Now()
	out := data.Read[[12][]float64](coder.Next(data.NewValue([2][]float64{
		features,
		{midpoint, hasReference, float64(step), float64(at.UnixNano()) / 1e9},
	}).Next(nil)))

	if solver.ObserveModule != nil {
		solver.ObserveModule("resonance", time.Since(stepStarted))
	}

	if err := coder.Error(); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbol),
			err,
		))
		return nil
	}

	metrics, metadata, err := publishReturns(out, coderArch(len(features)))

	if err != nil {
		solver.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: unable to publish coder reading for %s", symbol),
			err,
		))
		return nil
	}

	measurement := data.NewMeasurement(
		prior.Epoch, prior.Label, solver.Name(), prior.SeqIdx, prior.Tick, metadata...,
	)
	measurement.Peers(prior)
	measurement.At = prior.At
	measurement.From = prior.From
	if measurement.From.IsZero() {
		measurement.From = measurement.At
	}

	return measurement.Write(metrics...)
}

/*
coderArch is the predictive coder's layer stack for a feature width: the
sensory layer, an overcomplete 4x dictionary, a 2x bottleneck and a top
latent of feature width. The manifold reports one entry per layer in
reading[7], so publishing decodes against this same stack.
*/
func coderArch(featureDim int) []int {
	if featureDim <= 0 {
		featureDim = 11
	}

	return []int{featureDim, featureDim * 4, featureDim * 2, featureDim}
}

func (solver *Solver) coder(symbol string, featureDim int) *learning.PredictiveCoder {

	if loaded, found := solver.detectors.Load(symbol); found {
		if coder, ok := loaded.(*learning.PredictiveCoder); ok {
			return coder
		}
	}

	arch := coderArch(featureDim)
	created := learning.NewPredictiveCoder(
		arch, 8, learning.NewDirectionalTarget(0), nil, solver.pace, true, learning.ReadoutAll,
	)
	coder, ok := created.(*learning.PredictiveCoder)
	if !ok {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector construction failed for %s", symbol),
			nil,
		))
		return nil
	}

	actual, _ := solver.detectors.LoadOrStore(symbol, coder)
	return actual.(*learning.PredictiveCoder)
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
	if measurement == nil {
		return 0, false
	}

	var candidates []string

	switch index {
	case 0: // Correlation
		candidates = []string{
			"covariance_score", "cohort_covariance_score", "covariance",
			"absolute_covariance_score", "relative_return_energy",
		}
	case 1: // LeadLag
		candidates = []string{
			"covariance_score_gain_median", "led_peer_share",
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
			"book_imbalance_zscore", "book_imbalance",
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
		if metric, found := lookupMetric(measurement, label); found {
			if metric.Standardized != 0 || metric.Raw != 0 {
				return metric.Raw, true
			}
		}
	}

	for _, label := range candidates {
		if metric, found := lookupMetric(measurement, label); found {
			return metric.Raw, true
		}
	}

	return 0, false
}

/*
lookupMetric finds a metric by exact label, or by "<label>@<peer>" so correlation
and leadlag peer-suffixed facts remain addressable as their unsuffixed headline.
*/
func lookupMetric(measurement *data.Measurement, label string) (*data.Metric, bool) {
	if measurement == nil {
		return nil, false
	}

	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			continue
		}
		return entry.Metric, true
	}

	prefix := label + "@"
	for entry := range measurement.Read() {
		if entry.Err != nil {
			continue
		}
		if entry.Metric.Label == label || strings.HasPrefix(entry.Metric.Label, prefix) {
			return entry.Metric, true
		}
	}

	return nil, false
}

func firstPositive(measurement *data.Measurement, labels ...string) float64 {
	for _, label := range labels {
		if metric, found := lookupMetric(measurement, label); found && metric.Raw > 0 {
			return metric.Raw
		}
	}
	return 0
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
	moments      [11]core.Primitive
	residuals    [11]core.Primitive
	standardized [11]float64
}

func newFeatureScorer() *featureScorer {
	scorer := &featureScorer{}

	for index := range scorer.moments {
		scorer.moments[index] = statistic.NewEstimator()
		scorer.residuals[index] = statistic.NewCausalResidual()
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
		if measurement == nil {
			continue
		}

		val, ok := extractHeadlineMetric(index, measurement)
		if !ok {
			continue
		}

		var reading *statistic.MomentReading
		for out := range scorer.moments[index].Next(data.NewValue(val).Next(nil)) {
			reading = (*statistic.MomentReading)(out)
		}
		if err := scorer.moments[index].Error(); err != nil || reading == nil {
			continue
		}

		var residual statistic.CausalResidualResult
		for out := range scorer.residuals[index].Next(data.NewValue(*reading).Next(nil)) {
			residual = *(*statistic.CausalResidualResult)(out)
		}
		if err := scorer.residuals[index].Error(); err != nil {
			continue
		}

		scorer.standardized[index] = residual.ZScore
	}

	features := make([]float64, 11)
	copy(features, scorer.standardized[:])
	return features
}

/*
publishReturns projects the coder's *[12][]float64 reading into Measurement metrics
and provenance metadata. Energy and surprise come from the manifold summary; the
coder summary carries calibration and horizon evidence.

Every layer of the predictive-coding stack is published as
layer_<i>_state_<j> / layer_<i>_prediction_<j> (plus layer_<i>_error and
layer_<i>_temporal), decoded from reading[7] against arch. latent_<j> is the
top layer only; the forward curve is forward_curve_<h>. A reading[7] whose
length disagrees with arch is an error rather than a partial decode.
*/
func publishReturns(out [12][]float64, arch []int) ([]*data.Metric, []*data.StringEntry, error) {
	metrics := make([]*data.Metric, 0, 16)
	metadata := make([]*data.StringEntry, 0, 8)

	if len(out[0]) > 1 {
		metrics = append(metrics, data.NewMetric(
			"energy", out[0][1], data.UnitDimensionless, data.TimescaleInstantaneous,
		))
	}
	if len(out[0]) > 5 {
		metrics = append(metrics, data.NewMetric(
			"surprise", out[0][5], data.UnitDimensionless, data.TimescaleInstantaneous,
		))
	}

	for i, val := range out[2] {
		metrics = append(metrics, data.NewMetric(
			fmt.Sprintf("latent_%d", i), val, data.UnitDimensionless, data.TimescaleInstantaneous,
		))
	}

	if len(out[7]) > 0 {
		offset := 0

		for layer, rows := range arch {
			width := 2 + 2*rows

			if offset+width > len(out[7]) {
				return nil, nil, fmt.Errorf(
					"resonance: layer reading holds %d values, layer %d of arch %v needs %d",
					len(out[7]), layer, arch, offset+width,
				)
			}

			block := out[7][offset : offset+width]
			offset += width

			metrics = append(metrics,
				data.NewMetric(fmt.Sprintf("layer_%d_error", layer), block[0], data.UnitDimensionless, data.TimescaleInstantaneous),
				data.NewMetric(fmt.Sprintf("layer_%d_temporal", layer), block[1], data.UnitDimensionless, data.TimescaleInstantaneous),
			)

			for j := range rows {
				metrics = append(metrics,
					data.NewMetric(fmt.Sprintf("layer_%d_state_%d", layer, j), block[2+j], data.UnitDimensionless, data.TimescaleInstantaneous),
					data.NewMetric(fmt.Sprintf("layer_%d_prediction_%d", layer, j), block[2+rows+j], data.UnitDimensionless, data.TimescaleInstantaneous),
				)
			}
		}

		if offset != len(out[7]) {
			return nil, nil, fmt.Errorf(
				"resonance: layer reading holds %d values, arch %v accounts for %d",
				len(out[7]), arch, offset,
			)
		}
	}

	for i, val := range out[11] {
		metrics = append(metrics, data.NewMetric(
			fmt.Sprintf("forward_curve_%d", i), val, data.UnitDimensionless, data.TimescaleInstantaneous,
		))
	}

	if len(out[10]) > 0 {
		metadata = append(metadata,
			&data.StringEntry{Key: "supported_horizon", Value: fmt.Sprintf("%g", out[10][0])},
		)
	}
	if len(out[10]) > 1 {
		calibrated := "false"
		if out[10][1] != 0 {
			calibrated = "true"
		}
		metadata = append(metadata, &data.StringEntry{Key: "calibrated", Value: calibrated})
	}
	if len(out[10]) > 2 {
		metadata = append(metadata,
			&data.StringEntry{Key: "resolved_steps", Value: fmt.Sprintf("%g", out[10][2])},
		)
	}
	if len(out[10]) > 4 {
		metadata = append(metadata,
			&data.StringEntry{Key: "confidence", Value: fmt.Sprintf("%f", out[10][4])},
		)
	}

	if len(metrics) == 0 {
		metrics = append(metrics, data.NewMetric(
			"energy", 0, data.UnitDimensionless, data.TimescaleInstantaneous,
		))
	}

	return metrics, metadata, nil
}
