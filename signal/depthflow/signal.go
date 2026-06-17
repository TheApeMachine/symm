package depthflow

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
	feed "github.com/theapemachine/symm/signal"
)

/*
DepthFlow is the "Weight of the Book" perspective, measuring the asymmetry of
intent by looking at multiple levels of the order book, weighted by their
distance from the mid-price.

1. What it measures exactly (in isolation)

The DepthFlow signal measures distance-decayed book imbalance with
trade-pressure confirmation.

Weighted Depth Imbalance (WBI): Applies an exponential decay kernel
(exp(-λ · d)) to levels. Deep "spoof" walls are down-weighted, while
liquidity near the touch is prioritized.

Toxic Filter: It actively subtracts "toxic" levels — large, young blocks near
the touch that are frequently cancelled rather than filled — from the
imbalance calculation.

Trade Pressure EMA: Integrates recent trade sides into a running pressure
index to see if the book imbalance is actually resulting in trades.

Spoof Skew: Specifically flags when deep-book volume contradicts the touch
(e.g., a massive buy wall exists while the top-of-book is being sold into).

---

2. Semantically, what story does it tell?

The DepthFlow signal tells the story of structural gravity and manipulation
in the resting book.

The "Structural Wall" Story: It identifies when the "gravity" of the book is
pulling price in a certain direction.

The "Spoofing" Story: Using the SpoofSkew metric, it warns the engine when a
side is trying to "fake" depth to lure other participants into a trap.

The "Book Decay" Story: By tracking book_thinning and spread_widen events, it
identifies when a side's defensive walls are crumbling.

1. Loaded Imbalance

The book's weight agrees with trade pressure.
Indicators: High WBI with high, confirming trade pressure.
Semantic Meaning: Structural gravity — the wall is real and directional.

2. Spoof Trap

Deep-book shape contradicts what trades are doing.
Indicators: High WBI with low or contradicting trade pressure.
Semantic Meaning: Manipulated/Fake — a bluff wall near the touch.

3. Book Thinning

Defensive depth is disappearing at the touch.
Indicators: Rapidly falling WBI with variable trade pressure.
Semantic Meaning: Exhaustion/Crumbling — hollow, fragile support.

4. Dense Neutrality

Both sides carry balanced, thick depth.
Indicators: Balanced WBI with low trade pressure.
Semantic Meaning: Robust stability — two-sided, sincere liquidity.

# Summary of DepthFlow Categories

| Category         | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"            |
|:-----------------|:-------------------------|:------------------|:-------------------------|
| Loaded Imbalance | High                     | High (Agrees)     | Structural Gravity       |
| Spoof Trap       | High                     | Low (Contradicts) | Manipulated/Fake           |
| Book Thinning    | Rapidly Falling          | Variable          | Exhaustion/Crumbling       |
| Dense Neutrality | Balanced                 | Low               | Robust Stability           |
*/
/*
Signal measures distance-decayed book imbalance with trade-pressure confirmation.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
	trade        *feed.Trade
	book         *feed.Book
	ticker       *feed.Ticker
}

/*
NewSignal composes the bookflow pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, crossSectionErr := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 64,
	})

	if crossSectionErr != nil {
		cancel()

		return nil
	}

	bookflow := algorithm.NewBookflow()

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		tree:         dmt.NewTree(""),
		CrossSection: crossSection,
		trade:        feed.NewTrade(ctx),
		book:         feed.NewBook(ctx),
		ticker:       feed.NewTicker(ctx),
		algo: nomagique.Number(
			bookflow,
			probability.NewClassifier(
				bookflow.LoadedReading(),
				bookflow.SpoofReading(),
				bookflow.ThinningReading(),
				bookflow.NeutralReading(),
			),
		),
	}
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book":
		signal.book.Update(artifact)
	case "trade":
		signal.trade.Update(artifact)
	case "ticker":
		signal.ticker.Update(artifact)
	case "measurement":
		if artifact != nil {
			signal.Measure(*artifact)
		}
	}

	return nil
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	signal.trade.Scope = scope
	signal.book.Scope = scope
	signal.ticker.Scope = scope
	signal.trade.ResetReadHead()
	signal.book.ResetReadHead()
	signal.ticker.ResetReadHead()

	observeTrades(signal.CrossSection, signal.trade, scope)

	snapshot := bookSnapshot(signal.CrossSection, signal.book, scope)

	if snapshot.Spread <= 0 {
		return nil
	}

	feature := signal.featureArtifact(scope)

	if feature == nil {
		return nil
	}

	processed := datura.Acquire("depthflow", datura.APPJSON)

	if processed == nil {
		feature.Release()
		return nil
	}

	payload, payloadOK := feed.ArtifactPayload(feature)

	feature.Release()

	if !payloadOK {
		processed.Release()
		return nil
	}

	if !feed.ValidFloatPayload(payload, 4) {
		processed.Release()
		return nil
	}

	if processed.WithPayload(payload) == nil {
		processed.Release()
		return nil
	}

	if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
		_ = processed.WithError(flipErr)
	}

	if datura.Peek[int](processed, "classifier.category") <= 0 {
		processed.Release()
		return nil
	}

	if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
		processed.Release()
		return nil
	}

	processed.WithRole("measurement")
	processed.WithScope(scope)

	feed.InsertMeasurement(signal.tree, processed)

	return processed
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	observeTrades(signal.CrossSection, signal.trade, scope)

	snapshot := bookSnapshot(signal.CrossSection, signal.book, scope)

	if snapshot.Mid <= 0 || len(snapshot.WeightedHistory) == 0 || len(snapshot.Level1History) == 0 {
		return nil
	}

	tradePressure := signal.CrossSection.TradePressure(scope)

	flatOK := 0.0

	if snapshot.FlatOK {
		flatOK = 1
	}

	const depthflowHeaderFloats = 11

	maxFloats := feed.MaxFeatureFloats(
		"bookflow-features",
		"features",
		scope,
		depthflowHeaderFloats,
	)
	maxVariableFloats := maxFloats - depthflowHeaderFloats

	weightedHistory := snapshot.WeightedHistory
	level1History := snapshot.Level1History
	flatHistory := snapshot.FlatHistory

	if maxVariableFloats > 0 {
		trimmed := feed.TrimHistoryTails(
			[][]float64{weightedHistory, level1History, flatHistory},
			maxVariableFloats,
		)

		weightedHistory = trimmed[0]
		level1History = trimmed[1]
		flatHistory = trimmed[2]
	}

	samples := []float64{
		snapshot.Weighted,
		snapshot.Level1,
		snapshot.Flat,
		flatOK,
		snapshot.Mid,
		snapshot.Spread,
		snapshot.TouchDepth,
		tradePressure,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}

	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	artifact := datura.Acquire("bookflow-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(samples...))

	return artifact
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
