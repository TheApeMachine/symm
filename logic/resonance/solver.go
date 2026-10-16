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
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Solver orchestrates Predictive Coding across the multi-sensory microstructure stream.
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

	if pace <= 0 {
		solver.Transition(runtime.ERROR)
		errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("resonance: pace must be positive, got %f", pace),
			nil,
		))

		return solver
	}

	solver.Transition(runtime.READY)
	return solver
}

/*
Step advances the symbol's predictive coder over the canonical microstructure
sensory features from completed prior-stage signal outputs.
*/
func (solver *Solver) Step(prior *data.Value) *data.Value {
	if solver.Status() != runtime.READY {
		errnie.Warn(solver.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if solver.Error() != nil || prior == nil {
		return nil
	}

	symbol := prior.Scope
	if symbol == "" {
		return nil
	}

	metrics := data.Metrics(prior)
	midpoint := firstPositive(metrics, "midpoint", "last_price", "price", "last")

	features := solver.scorer(symbol).Step(metrics)

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

	sec := float64(prior.Timestamp) / 1e9
	if prior.Timestamp < 1e11 {
		sec = float64(prior.Timestamp)
	}

	stepStarted := time.Now()
	out := transport.Read[[12][]float64](coder.Next(transport.NewValue([2][]float64{
		features,
		{midpoint, hasReference, float64(step), sec},
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

	res, err := publishReturns(prior, out, coderArch(len(features)))
	if err != nil {
		solver.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: unable to publish coder reading for %s", symbol),
			err,
		))
		return nil
	}

	return res
}

func coderArch(featureDim int) []int {
	if featureDim <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("resonance: featureDim must be positive, got %d", featureDim),
			nil,
		))
		return nil
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
	if arch == nil {
		return nil
	}

	pace := learning.NewPace(solver.pace, solver.pace*0.1, 0.150, 0.1, 0.2, 256)
	target := learning.NewDirectionalTarget(0)
	coder := learning.NewPredictiveCoder(arch, 8, target, pace, 0.05, true, learning.ReadoutAll).(*learning.PredictiveCoder)

	actual, _ := solver.detectors.LoadOrStore(symbol, coder)
	return actual.(*learning.PredictiveCoder)
}

func extractHeadlineMetric(index int, metrics map[string]float64) (float64, bool) {
	if len(metrics) == 0 {
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
		if val, found := lookupMetric(metrics, label); found && val != 0 {
			return val, true
		}
	}

	for _, label := range candidates {
		if val, found := lookupMetric(metrics, label); found {
			return val, true
		}
	}

	return 0, false
}

func lookupMetric(metrics map[string]float64, label string) (float64, bool) {
	if len(metrics) == 0 {
		return 0, false
	}

	if val, found := metrics[label]; found {
		return val, true
	}

	prefix := label + "@"
	for k, val := range metrics {
		if strings.HasPrefix(k, prefix) {
			return val, true
		}
	}

	return 0, false
}

func firstPositive(metrics map[string]float64, labels ...string) float64 {
	for _, label := range labels {
		if val, found := lookupMetric(metrics, label); found && val > 0 {
			return val
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

func (scorer *featureScorer) Step(metrics map[string]float64) []float64 {
	for !scorer.isStepping.CompareAndSwap(false, true) {
		goruntime.Gosched()
	}
	defer scorer.isStepping.Store(false)

	for index := range 11 {
		val, ok := extractHeadlineMetric(index, metrics)
		if !ok {
			continue
		}

		var reading *statistic.MomentReading
		for out := range scorer.moments[index].Next(transport.NewValue(val).Next(nil)) {
			reading = (*statistic.MomentReading)(out)
		}
		if err := scorer.moments[index].Error(); err != nil || reading == nil {
			continue
		}

		var residual statistic.CausalResidualResult
		for out := range scorer.residuals[index].Next(transport.NewValue(*reading).Next(nil)) {
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

func publishReturns(prior *data.Value, out [12][]float64, arch []int) (*data.Value, error) {
	if len(out[7]) > 0 {
		offset := 0
		for layer, rows := range arch {
			width := 2 + 2*rows
			if offset+width > len(out[7]) {
				return nil, fmt.Errorf("resonance: layer reading holds %d values, layer %d of arch %v needs %d", len(out[7]), layer, arch, offset+width)
			}
			offset += width
		}
		if offset != len(out[7]) {
			return nil, fmt.Errorf("resonance: layer reading holds %d values, arch %v accounts for %d", len(out[7]), arch, offset)
		}
	}

	if len(out[0]) > 1 {
		data.AppendVector(prior, []float64{out[0][1]}, "energy")
	} else {
		data.AppendVector(prior, []float64{0}, "energy")
	}

	if len(out[0]) > 5 {
		data.AppendVector(prior, []float64{out[0][5]}, "surprise")
	}

	for i, val := range out[2] {
		data.AppendVector(prior, []float64{val}, fmt.Sprintf("latent_%d", i))
	}

	if len(out[7]) > 0 {
		offset := 0
		for layer, rows := range arch {
			width := 2 + 2*rows
			block := out[7][offset : offset+width]
			offset += width

			data.AppendVector(prior, []float64{block[0]}, fmt.Sprintf("layer_%d_error", layer))
			data.AppendVector(prior, []float64{block[1]}, fmt.Sprintf("layer_%d_temporal", layer))

			for j := range rows {
				data.AppendVector(prior, []float64{block[2+j]}, fmt.Sprintf("layer_%d_state_%d", layer, j))
				data.AppendVector(prior, []float64{block[2+rows+j]}, fmt.Sprintf("layer_%d_prediction_%d", layer, j))
			}
		}
	}

	for i, val := range out[11] {
		data.AppendVector(prior, []float64{val}, fmt.Sprintf("forward_curve_%d", i))
	}

	return prior, nil
}
