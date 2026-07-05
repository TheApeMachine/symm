package resonance

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Resonance is the latent-attention perspective, detecting surprise in a
twelve-channel market sensory vector via a batch autoencoder.

1. What it measures exactly (in isolation)

The Resonance signal builds a fixed-width sensory vector from ticker, book,
and trade snapshots per symbol:

change, spread, log-volume, trade rate, buy pressure, touch imbalance,
depth imbalance, spread-wide ratio, tick cadence, trade notional, mid drift,
and |change| — each median-scaled against per-symbol baselines.

A batch autoencoder (batchEngine) encodes the vector, decodes it, and scores
reconstruction surprise. Attention modes derive from latent settle state.

---

2. Semantically, what story does it tell?

The Resonance signal tells the story of how "surprising" current microstructure
is relative to each symbol's learned baseline — laminar flow, turbulent stress,
or coupled equilibrium.

1. Laminar Resonance (CategoryFlow)

Default latent mode when flow dynamics dominate reconstruction.
Indicators: Nonstress latent activation dominates when spread is valid.
Semantic Meaning: Orderly resonance — sensory state matches baseline.

2. Turbulent Resonance (CategoryStress)

Stress latent mode dominates the reconstruction.
Indicators: Stress latent activation exceeds the other attention modes combined.
Semantic Meaning: Microstructural turbulence — surprise in stress channels.

3. Equilibrium (CategoryCoupling)

Spread is zero or invalid — book coupling cannot be resolved.
Indicators: spread ≤ 0 forces equilibrium regardless of latent vector.
Semantic Meaning: No touch — market in indeterminate coupling.

# Summary of Resonance Categories

| Category             | Routing Rule              | Market "Feel"              |
|:---------------------|:--------------------------|:---------------------------|
| Laminar Resonance    | Nonstress latent majority | Orderly / Baseline Match   |
| Turbulent Resonance  | Stress latent majority    | Surprising Stress          |
| Equilibrium          | spread ≤ 0                | No Touch / Indeterminate   |
*/

type featureContext struct {
	input           []float64
	lastPrice       float64
	volume          float64
	spread          float64
	spreadWideRatio float64
	elapsed         float64
	observedAt      time.Time
}

type Signal struct {
	ctx                context.Context
	cancel             context.CancelFunc
	err                error
	engine             batchEngine
	slots              *slotRegistry
	trade              *marketTrade
	book               *marketBook
	ticker             *marketTicker
	arch               []int
	alpha              float64
	batchSize          int
	baselines          *senseRegistry
	lastSettled        []settledSymbolEntry
	snapshot           map[string]any
	marketRevision     sync.Map
	lastSettleRevision sync.Map
}

func NewSignal(
	ctx context.Context,
	arch []int,
	alpha float64,
	batchSize int,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	// The autoencoder architecture's width depends only on the sensory channel
	// count and a per-symbol settle width, never on how many symbols are live.
	resolvedArch := arch

	if len(resolvedArch) == 0 {
		resolvedArch = DefaultArchitecture()
	}

	resolvedAlpha := alpha

	if resolvedAlpha <= 0 {
		resolvedAlpha = 1 / float64(SensoryChannelCount)
	}

	// batchSize is the engine's slot capacity — how many symbols can settle in
	// one batch. It is grown to the live universe on demand (ensureCapacity),
	// so an unset value seeds at 1 and expands, rather than being capped by a
	// config list that would silently drop the obscure movers we hunt.
	resolvedBatch := max(batchSize, 1)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		trade:     newMarketTrade(ctx),
		book:      newMarketBook(ctx),
		ticker:    newMarketTicker(ctx),
		arch:      resolvedArch,
		alpha:     resolvedAlpha,
		batchSize: resolvedBatch,
		slots:     newSlotRegistry(resolvedBatch),
		baselines: newSenseRegistry(),
	}

	if validateErr := validateArchitecture(resolvedArch); validateErr != nil {
		signal.err = validateErr

		return signal
	}

	return signal
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade", "ticker"}
}

/*
ensureCapacity (re)builds the batch engine so it can settle at least required
symbols in one batch. The engine and slot registry are fixed-size allocations,
so when the live universe outgrows the current capacity they are rebuilt larger
— never capped, because a cap would silently drop the obscure movers the system
exists to catch. Capacity only grows; it never shrinks back.
*/
func (signal *Signal) ensureCapacity(required int) error {
	if signal == nil {
		return errnie.Error(fmt.Errorf("resonance: signal is nil"))
	}

	if signal.err != nil {
		return errnie.Error(signal.err)
	}

	if required < 1 {
		required = 1
	}

	if signal.engine != nil && required <= signal.batchSize {
		return nil
	}

	engine, engineErr := newBatchEngine(signal.arch, signal.alpha, required)

	if engineErr != nil {
		signal.err = engineErr
		return errnie.Error(engineErr)
	}

	if signal.engine != nil {
		signal.engine.Close()
	}

	signal.engine = engine
	signal.batchSize = required
	signal.slots = newSlotRegistry(required)

	return nil
}

func (signal *Signal) Measure(
	input market.Input,
	_ *market.CrossSection,
) ([]*logic.Measurement, error) {
	switch input.Role {
	case "book":
		return nil, signal.observeBooks(input.Book)
	case "trade":
		return nil, signal.observeTrades(input.Trade)
	case "ticker":
		if err := signal.observeTickers(input.Ticker); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}
	default:
		return nil, errnie.Err(errnie.Validation, "resonance: unsupported input role "+input.Role, nil)
	}

	scopes := make([]string, 0, len(input.Ticker))
	seen := make(map[string]struct{}, len(input.Ticker))

	for _, ticker := range input.Ticker {
		if _, ok := seen[ticker.Symbol]; ok {
			continue
		}

		seen[ticker.Symbol] = struct{}{}
		scopes = append(scopes, ticker.Symbol)
	}

	if len(scopes) == 0 {
		return nil, nil
	}

	results, err := signal.SettleScopes(scopes)

	if err != nil {
		signal.err = errnie.Error(err)
		return nil, signal.err
	}

	measurements := make([]*logic.Measurement, 0, len(results))

	for _, scope := range scopes {
		measurement := results[scope]

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
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
		input:           vector,
		lastPrice:       facts.lastPrice,
		volume:          facts.volume,
		spread:          facts.spreadBps,
		spreadWideRatio: facts.spreadWideRatio,
		elapsed:         facts.elapsed,
		observedAt:      observedAt(tickerSnap.Observed),
	}, true
}

func (signal *Signal) measurementFromOutcome(
	outcome settleOutcome,
	features featureContext,
) (*logic.Measurement, error) {
	if math.IsNaN(outcome.surprise) || math.IsInf(outcome.surprise, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "resonance: surprise is non-finite", nil,
		))
	}

	peakActivation := 0.0

	for _, value := range outcome.latent {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		peakActivation = math.Max(peakActivation, math.Abs(value))
	}

	if math.IsNaN(peakActivation) || math.IsInf(peakActivation, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "resonance: latent activation is non-finite", nil,
		))
	}

	if math.IsNaN(features.lastPrice) || math.IsInf(features.lastPrice, 0) ||
		math.IsNaN(features.volume) || math.IsInf(features.volume, 0) ||
		math.IsNaN(features.spread) || math.IsInf(features.spread, 0) ||
		math.IsNaN(features.elapsed) || math.IsInf(features.elapsed, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "resonance: feature context is non-finite", nil,
		))
	}

	classified, classifyOK := signal.attentionFromOutcome(features, outcome)

	if !classifyOK {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "resonance: failed to classify attention", nil,
		))
	}

	categoryIndex := classified.categoryIndex
	confidence := classified.confidence
	category := resonanceCategoryFromIndex(categoryIndex)
	baseline := 1.0 / float64(resonanceLatentWidth)

	if categoryIndex <= 0 || confidence <= 0 || outcome.symbol == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "resonance: invalid attention outcome", nil,
		))
	}

	measurement := logic.NewMeasurement(
		logic.SourceResonance,
		outcome.symbol,
		features.observedAt,
	)

	if err := measurement.ApplyClassifier(
		float64(categoryIndex),
		confidence,
		baseline,
		baseline,
		peakActivation,
		signal.attentionDistribution(features, outcome),
	); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement.Status = category
	measurement.Surprise = outcome.surprise
	measurement.Elapsed = features.elapsed
	measurement.AddMetric("category", float64(categoryIndex))
	measurement.AddMetric("price", features.lastPrice)
	measurement.AddMetric("volume", features.volume)
	measurement.AddMetric("spread", features.spread)
	measurement.AddMetric("elapsed", features.elapsed)
	measurement.AddMetric("surprise", outcome.surprise)

	return measurement, nil
}

func (signal *Signal) attentionDistribution(
	features featureContext,
	outcome settleOutcome,
) map[string]float64 {
	if features.spread <= 0 {
		return map[string]float64{
			CategoryCoupling: 1,
		}
	}

	flow := 0.0
	stress := 0.0

	for index, value := range outcome.latent {
		activation := math.Abs(value)

		if index == 1 {
			stress += activation
			continue
		}

		flow += activation
	}

	total := flow + stress

	if total <= 0 {
		return map[string]float64{
			CategoryFlow: 1,
		}
	}

	return map[string]float64{
		CategoryFlow:   flow / total,
		CategoryStress: stress / total,
	}
}

type attentionOutcome struct {
	categoryIndex int
	confidence    float64
}

func (signal *Signal) attentionFromOutcome(
	features featureContext,
	outcome settleOutcome,
) (attentionOutcome, bool) {
	categoryIndex := AttentionCategoryIndex(features.spread, outcome.latent)
	confidence := AttentionConfidence(features.spread, outcome.surprise, outcome.latent)

	if categoryIndex <= 0 || confidence <= 0 || outcome.symbol == "" {
		return attentionOutcome{}, false
	}

	return attentionOutcome{
		categoryIndex: categoryIndex,
		confidence:    confidence,
	}, true
}

func resonanceCategoryFromIndex(categoryIndex int) string {
	switch categoryIndex {
	case 1:
		return CategoryFlow
	case 2:
		return CategoryStress
	case 3:
		return CategoryCoupling
	default:
		return ""
	}
}

func observedAt(timestamp time.Time) time.Time {
	if timestamp.IsZero() {
		return time.Now()
	}

	return timestamp
}

/*
FocusSettled returns the highest-surprise settled resonance layers from the last batch.
*/
func (signal *Signal) FocusSettled() (SettledSnapshot, bool) {
	if signal == nil || len(signal.lastSettled) == 0 {
		return SettledSnapshot{}, false
	}

	focusIndex := focusSymbolIndex(signal.lastSettled)
	entry := signal.lastSettled[focusIndex]

	return SettledSnapshot{
		Scope:    entry.outcome.symbol,
		Layers:   entry.layers,
		Surprise: entry.surprise,
		Energy:   entry.energy,
	}, true
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
