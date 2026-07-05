package resonance

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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

type Signal[T any] struct {
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

func NewSignal[T any](
	ctx context.Context,
	arch []int,
	alpha float64,
	batchSize int,
) *Signal[T] {
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

	signal := &Signal[T]{
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

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"book", "trade", "ticker"}
}

func (signal *Signal[T]) Categories() []types.CategoryType {
	return []types.CategoryType{
		types.LaminarResonance,
		types.TurbulentResonance,
		types.Equilibrium,
	}
}

/*
ensureCapacity (re)builds the batch engine so it can settle at least required
symbols in one batch. The engine and slot registry are fixed-size allocations,
so when the live universe outgrows the current capacity they are rebuilt larger
— never capped, because a cap would silently drop the obscure movers the system
exists to catch. Capacity only grows; it never shrinks back.
*/
func (signal *Signal[T]) ensureCapacity(required int) error {
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

func (signal *Signal[T]) Measure(
	input T,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.BookData:
		return nil, signal.observeBooks(kraken.BookDataSlice{row})
	case kraken.TradeData:
		return nil, signal.observeTrades(kraken.TradeDataSlice{row})
	case kraken.TickerData:
		if err := signal.observeTickers(kraken.TickerDataSlice{row}); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		results, err := signal.SettleScopes([]string{row.Symbol})

		if err != nil {
			signal.err = errnie.Error(err)
			return nil, signal.err
		}

		measurement := results[row.Symbol]

		if measurement == nil {
			return nil, nil
		}

		return []*types.Measurement{measurement}, nil
	}

	return nil, nil
}

func (signal *Signal[T]) featureContext(scope string) (featureContext, bool) {
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

func (signal *Signal[T]) measurementFromOutcome(
	outcome settleOutcome,
	features featureContext,
) (*types.Measurement, error) {
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

	classified, classifyErr := signal.attentionFromOutcome(features, outcome)

	if classifyErr != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, classifyErr.Error(), classifyErr,
		))
	}

	categoryIndex := classified.categoryIndex
	confidence := classified.confidence
	category := resonanceCategoryFromIndex(categoryIndex)

	if categoryIndex <= 0 || confidence <= 0 || outcome.symbol == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "resonance: invalid attention outcome", nil,
		))
	}

	distribution := signal.attentionDistribution(categoryIndex, features, outcome)
	flow, stress, coupling := signal.attentionScores(features, outcome)
	categories := []types.CategoryType{
		types.CategoryType(CategoryFlow),
		types.CategoryType(CategoryStress),
		types.CategoryType(CategoryCoupling),
	}
	strengths := []float64{
		flow,
		stress,
		coupling,
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, categoryType := range categories {
		categorySurprisal := 0.0

		if index+1 == categoryIndex {
			categorySurprisal = outcome.surprise
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       categoryType,
			Confidence: distribution[string(categoryType)],
			Surprisal:  categorySurprisal,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourceResonance,
		Symbol:        outcome.symbol,
		At:            features.observedAt,
		Status:        category,
		Elapsed:       features.elapsed,
		EntryBaseline: classified.entryBaseline,
		ExitBaseline:  classified.exitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"category": float64(categoryIndex),
			"price":    features.lastPrice,
			"volume":   features.volume,
			"spread":   features.spread,
			"elapsed":  features.elapsed,
			"surprise": outcome.surprise,
			"strength": peakActivation,
		},
	}

	return measurement, nil
}

func (signal *Signal[T]) attentionDistribution(
	categoryIndex int,
	features featureContext,
	outcome settleOutcome,
) map[string]float64 {
	if categoryIndex == 3 {
		return map[string]float64{
			CategoryCoupling: 1,
		}
	}

	flow, stress, coupling := signal.attentionScores(features, outcome)
	total := flow + stress + coupling

	if total <= 0 {
		return map[string]float64{
			CategoryFlow: 1,
		}
	}

	return map[string]float64{
		CategoryFlow:     flow / total,
		CategoryStress:   stress / total,
		CategoryCoupling: coupling / total,
	}
}

type attentionOutcome struct {
	categoryIndex int
	confidence    float64
	entryBaseline float64
	exitBaseline  float64
}

func (signal *Signal[T]) attentionFromOutcome(
	features featureContext,
	outcome settleOutcome,
) (attentionOutcome, error) {
	categoryIndex := AttentionCategoryIndex(features.spread, outcome.latent)
	flow, stress, coupling := signal.attentionScores(features, outcome)
	scores := []float64{flow, stress, coupling}
	confidence, err := probability.CategoryShareConfidence(scores, categoryIndex)

	if err != nil {
		return attentionOutcome{}, err
	}

	_, entryBaseline, exitBaseline, err := probability.CategoryEvidenceBaselines(scores, categoryIndex)

	if err != nil {
		return attentionOutcome{}, err
	}

	if categoryIndex <= 0 || confidence <= 0 || outcome.symbol == "" {
		return attentionOutcome{}, errnie.Err(
			errnie.Validation,
			"resonance: invalid attention outcome",
			nil,
		)
	}

	return attentionOutcome{
		categoryIndex: categoryIndex,
		confidence:    confidence,
		entryBaseline: entryBaseline,
		exitBaseline:  exitBaseline,
	}, nil
}

func (signal *Signal[T]) attentionScores(
	features featureContext,
	outcome settleOutcome,
) (float64, float64, float64) {
	if features.spread <= 0 {
		return 0, 0, 1 / (1 + math.Abs(outcome.surprise))
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

	if flow+stress <= 0 {
		flow = 1 / (1 + math.Abs(outcome.surprise))
	}

	return flow, stress, 0
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
func (signal *Signal[T]) FocusSettled() (SettledSnapshot, bool) {
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

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() error {
	signal.cancel()

	if signal.engine != nil {
		signal.engine.Close()
	}

	return nil
}
