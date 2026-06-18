package resonance

import (
	"context"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

type featureContext struct {
	input      []float64
	lastPrice  float64
	volume     float64
	spread     float64
	elapsed    float64
	observedAt time.Time
}

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	pool   *qpool.Q[any]
	algo   io.ReadWriter
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	arch []int,
	alpha float64,
	batchSize int,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   dmt.NewTree(""),
		algo: nomagique.Number(
			vector.NewFeatureExtractor(
				datura.Acquire(
					"resonance", datura.APPJSON,
				),
			),
			probability.NewClassifier(
				resonance.FlowReading(),
				resonance.StressReading(),
				resonance.CouplingReading(),
			),
		),
	}

	return signal
}

func (signal *Signal) ensureEngine() error {
	if signal == nil {
		return fmt.Errorf("resonance: signal is nil")
	}

	if signal.engine != nil {
		return signal.err
	}

	if signal.err != nil {
		return signal.err
	}

	engine, engineErr := newBatchEngine(signal.arch, signal.alpha, signal.batchSize)

	if engineErr != nil {
		signal.err = engineErr

		return engineErr
	}

	signal.engine = engine

	return nil
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book", "trade", "ticker":
	}

	return nil
}

func (signal *Signal) SettleScopes(scopes []string) (map[string]logic.Measurement, error) {
	if signal == nil {
		return nil, fmt.Errorf("resonance: signal is nil")
	}

	if ensureErr := signal.ensureEngine(); ensureErr != nil {
		return nil, ensureErr
	}

	entries := make([]batchEntry, 0, len(scopes))
	contexts := make(map[string]featureContext, len(scopes))

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		features, ok := signal.featureContext(scope)

		if !ok {
			continue
		}

		slot, ok := signal.slots.assign(scope)

		if !ok {
			continue
		}

		entries = append(entries, batchEntry{
			slot:   slot,
			symbol: scope,
			input:  features.input,
		})
		contexts[scope] = features
	}

	if len(entries) == 0 {
		return map[string]logic.Measurement{}, signal.err
	}

	outcomes, settleErr := signal.engine.Settle(entries)

	if settleErr != nil {
		signal.err = settleErr

		return nil, settleErr
	}

	results := make(map[string]logic.Measurement, len(outcomes))
	settled := make([]settledSymbolEntry, 0, len(outcomes))

	for _, outcome := range outcomes {
		features, ok := contexts[outcome.symbol]

		if !ok {
			continue
		}

		measurement, publishable := signal.measurementFromOutcome(outcome, features)

		if !publishable {
			continue
		}

		results[outcome.symbol] = measurement

		wire, wireErr := buildSettledSymbolEntry(signal, outcome, measurement)

		if wireErr != nil {
			signal.err = wireErr

			continue
		}

		settled = append(settled, wire)
	}

	if publishErr := signal.publishUniverse(settled); publishErr != nil {
		signal.err = publishErr
	}

	signal.lastSettled = settled

	return results, signal.err
}

func (signal *Signal) Measure(in *datura.Artifact) logic.Measurement {
	scope := datura.Peek[string](in, "scope")

	return measurement.UnlessPublishable(), signal.err
}

func (signal *Signal) featureContext(scope string) (featureContext, bool) {
	vector, facts, ok := buildSensoryVector(scope, signal.baselines)

	if !ok {
		return featureContext{}, false
	}

	return featureContext{
		input:      vector,
		lastPrice:  facts.lastPrice,
		volume:     facts.volume,
		spread:     facts.spreadBps,
		elapsed:    facts.elapsed,
		observedAt: observedAt(time.Time{}),
	}, true
}

func (signal *Signal) measurementFromOutcome(
	outcome settleOutcome,
	features featureContext,
) (logic.Measurement, bool) {
	if !logic.ScalarFinite(outcome.surprise) {
		return logic.Measurement{}, false
	}

	peakActivation := 0.0

	for _, value := range outcome.latent {
		if !logic.ScalarFinite(value) {
			continue
		}

		peakActivation = math.Max(peakActivation, math.Abs(value))
	}

	confidence := 1.0 / (1.0 + outcome.surprise)

	if !logic.ScalarFinite(confidence) || !logic.ScalarFinite(peakActivation) {
		return logic.Measurement{}, false
	}

	if !logic.ScalarFinite(features.lastPrice) ||
		!logic.ScalarFinite(features.volume) ||
		!logic.ScalarFinite(features.spread) ||
		!logic.ScalarFinite(features.elapsed) {
		return logic.Measurement{}, false
	}

	return logic.Measurement{
		Source:     "resonance",
		Symbol:     outcome.symbol,
		Price:      features.lastPrice,
		Strength:   peakActivation,
		Volume:     features.volume,
		Spread:     features.spread,
		Elapsed:    features.elapsed,
		Category:   logic.CategoryType(signal.determineCategory(outcome.latent)),
		Confidence: confidence,
		Surprise:   outcome.surprise,
		ObservedAt: features.observedAt,
	}, true
}

func observedAt(timestamp time.Time) time.Time {
	if timestamp.IsZero() {
		return time.Now()
	}

	return timestamp
}

func (signal *Signal) determineCategory(latent []float64) string {
	if len(latent) == 0 {
		return CategoryCoupling
	}

	maxIdx := 0
	maxVal := 0.0

	for index, value := range latent {
		if math.Abs(value) > math.Abs(maxVal) {
			maxVal = value
			maxIdx = index
		}
	}

	switch maxIdx {
	case 0:
		return CategoryFlow
	case 1:
		return CategoryStress
	default:
		return CategoryCoupling
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	if signal.engine != nil {
		signal.engine.Close()
	}

	return nil
}
