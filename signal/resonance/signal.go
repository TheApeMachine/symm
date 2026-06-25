package resonance

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
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
Indicators: Dominant latent index 0 (or 2 when spread is valid).
Semantic Meaning: Orderly resonance — sensory state matches baseline.

2. Turbulent Resonance (CategoryStress)

Stress latent mode dominates the reconstruction.
Indicators: Dominant latent index 1.
Semantic Meaning: Microstructural turbulence — surprise in stress channels.

3. Equilibrium (CategoryCoupling)

Spread is zero or invalid — book coupling cannot be resolved.
Indicators: spread ≤ 0 forces equilibrium regardless of latent vector.
Semantic Meaning: No touch — market in indeterminate coupling.

# Summary of Resonance Categories

| Category             | Routing Rule              | Market "Feel"              |
|:---------------------|:--------------------------|:---------------------------|
| Laminar Resonance    | Dominant latent ≠ stress  | Orderly / Baseline Match   |
| Turbulent Resonance  | Dominant latent = stress  | Surprising Stress          |
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
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	pool             *qpool.Q[any]
	uiBroadcast      *qpool.BroadcastGroup
	engine           batchEngine
	slots            *slotRegistry
	trade            *marketTrade
	book             *marketBook
	ticker           *marketTicker
	arch             []int
	alpha            float64
	batchSize        int
	baselines        *senseRegistry
	lastSettled      []settledSymbolEntry
	tree             *dmt.Tree
	lastHydrateStamp int64
	marketRevision   sync.Map
	lastSettleRevision sync.Map
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
		pool:      pool,
		trade:     newMarketTrade(ctx),
		book:      newMarketBook(ctx),
		ticker:    newMarketTicker(ctx),
		arch:      resolvedArch,
		alpha:     resolvedAlpha,
		batchSize: resolvedBatch,
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

func (signal *Signal) ObserveIngest(artifact *datura.Artifact) {
	if signal == nil || artifact == nil {
		return
	}

	role, roleErr := artifact.Role()

	if roleErr != nil || role == "" {
		return
	}

	switch role {
	case "book":
		signal.observeBookArtifact(artifact)
	case "trade":
		signal.observeTradeArtifact(artifact)
	case "ticker":
		signal.observeTickerArtifact(artifact)
	}
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
		return fmt.Errorf("resonance: signal is nil")
	}

	if signal.err != nil {
		return signal.err
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

		return engineErr
	}

	if signal.engine != nil {
		signal.engine.Close()
	}

	signal.engine = engine
	signal.batchSize = required
	signal.slots = newSlotRegistry(required)

	return nil
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	scope, _ := datapoint.Scope()

	if scope == "" {
		return nil
	}

	results, settleErr := signal.SettleScopes([]string{scope})

	if settleErr != nil {
		return nil
	}

	measurement, ok := results[scope]

	if !ok || measurement == nil {
		return nil
	}

	return measurement
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
) (*datura.Artifact, bool) {
	if math.IsNaN(outcome.surprise) || math.IsInf(outcome.surprise, 0) {
		return nil, false
	}

	peakActivation := 0.0

	for _, value := range outcome.latent {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		peakActivation = math.Max(peakActivation, math.Abs(value))
	}

	if math.IsNaN(peakActivation) || math.IsInf(peakActivation, 0) {
		return nil, false
	}

	if math.IsNaN(features.lastPrice) || math.IsInf(features.lastPrice, 0) ||
		math.IsNaN(features.volume) || math.IsInf(features.volume, 0) ||
		math.IsNaN(features.spread) || math.IsInf(features.spread, 0) ||
		math.IsNaN(features.elapsed) || math.IsInf(features.elapsed, 0) {
		return nil, false
	}

	classified, classifyOK := signal.attentionFromOutcome(features, outcome)

	if !classifyOK {
		return nil, false
	}

	categoryIndex := classified.categoryIndex
	confidence := classified.confidence
	category := resonanceCategoryFromIndex(categoryIndex)

	if categoryIndex <= 0 || confidence <= 0 || outcome.symbol == "" {
		return nil, false
	}

	artifact := datura.Acquire("resonance", datura.Artifact_Type_json)

	if artifact == nil {
		return nil, false
	}

	artifact.WithRole("measurement")
	artifact.WithScope(outcome.symbol)
	artifact.SetTimestamp(features.observedAt.UnixNano())
	_ = artifact.SetOrigin("resonance")
	artifact.MergeOutput("category", float64(categoryIndex))
	artifact.Merge("category", category)
	artifact.Merge("classifier", map[string]any{
		"category":   categoryIndex,
		"confidence": confidence,
	})
	artifact.MergeOutput("confidence", confidence)
	artifact.MergeOutput("strength", peakActivation)
	artifact.Merge("surprise", outcome.surprise)
	artifact.Merge("price", features.lastPrice)
	artifact.Merge("volume", features.volume)
	artifact.Merge("spread", features.spread)
	artifact.Merge("elapsed", features.elapsed)
	artifact.Merge("observed_at", features.observedAt.UTC().Format(time.RFC3339Nano))

	return artifact, true
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
