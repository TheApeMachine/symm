package resonance

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
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
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	uiBroadcast *qpool.BroadcastGroup
	engine      batchEngine
	slots       *slotRegistry
	trade       *marketTrade
	book        *marketBook
	ticker      *marketTicker
	arch        []int
	alpha       float64
	batchSize   int
	baselines   *senseRegistry
	lastSettled []settledSymbolEntry
	tree        *dmt.Tree
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	arch []int,
	alpha float64,
	batchSize int,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	resolvedArch := arch

	if len(resolvedArch) == 0 {
		resolvedArch = DefaultArchitecture()
	}

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		trade:     newMarketTrade(ctx),
		book:      newMarketBook(ctx),
		ticker:    newMarketTicker(ctx),
		arch:      resolvedArch,
		alpha:     alpha,
		batchSize: batchSize,
		slots:     newSlotRegistry(batchSize),
		baselines: newSenseRegistry(),
		tree:      tree,
	}

	if validateErr := validateArchitecture(resolvedArch); validateErr != nil {
		signal.err = validateErr

		return signal
	}

	if pool != nil {
		signal.uiBroadcast = pool.CreateBroadcastGroup("ui")
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

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	if signal == nil {
		return nil
	}

	scope, _ := query.Scope()

	if scope == "" {
		scope = datura.Peek[string](&query, "scope")
	}

	if scope == "" {
		return nil
	}

	results, settleErr := signal.SettleScopes([]string{scope})

	if settleErr != nil {
		signal.err = settleErr

		return nil
	}

	measurement, ok := results[scope]

	if !ok {
		return nil
	}

	artifact := measurementArtifact(measurement)

	if artifact == nil {
		return nil
	}

	InsertMeasurement(signal.tree, artifact)

	return artifact
}

func (signal *Signal) featureContext(scope string) (featureContext, bool) {
	vector, facts, ok := buildSensoryVector(
		scope,
		signal.ticker,
		signal.book,
		signal.trade,
		signal.baselines,
	)

	if !ok {
		return featureContext{}, false
	}

	tickerSnap := signal.ticker.Snapshot(scope)

	return featureContext{
		input:      vector,
		lastPrice:  facts.lastPrice,
		volume:     facts.volume,
		spread:     facts.spreadBps,
		elapsed:    facts.elapsed,
		observedAt: observedAt(tickerSnap.Observed),
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
