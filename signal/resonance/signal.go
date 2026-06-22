package resonance

import (
	"context"
	"fmt"
	"math"
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
reconstruction surprise. Three latent modes map to attention categories.

This is NOT a nomagique.Number pipeline; classification uses dominant latent
index with spread ≤ 0 routing to equilibrium.

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

	if signal.tree != nil {
		if wire := measurement.Pack(); len(wire) > 0 {
			signal.tree.Insert(measurement.Prefix(), wire)
		}
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

	confidence := 1.0 / (1.0 + outcome.surprise)

	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || math.IsNaN(peakActivation) || math.IsInf(peakActivation, 0) {
		return nil, false
	}

	if math.IsNaN(features.lastPrice) || math.IsInf(features.lastPrice, 0) ||
		math.IsNaN(features.volume) || math.IsInf(features.volume, 0) ||
		math.IsNaN(features.spread) || math.IsInf(features.spread, 0) ||
		math.IsNaN(features.elapsed) || math.IsInf(features.elapsed, 0) {
		return nil, false
	}

	category := signal.determineCategory(features, outcome.latent)
	categoryIndex := resonanceCategoryIndex(category)

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

func resonanceCategoryIndex(category string) int {
	switch category {
	case CategoryFlow:
		return 1
	case CategoryStress:
		return 2
	case CategoryCoupling:
		return 3
	default:
		return 0
	}
}

func observedAt(timestamp time.Time) time.Time {
	if timestamp.IsZero() {
		return time.Now()
	}

	return timestamp
}

func (signal *Signal) determineCategory(features featureContext, latent []float64) string {
	if features.spread <= 0 {
		return CategoryCoupling
	}

	if len(latent) == 0 {
		return CategoryFlow
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
	case 1:
		return CategoryStress
	default:
		return CategoryFlow
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
