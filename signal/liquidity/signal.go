package liquidity

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	marketsection "github.com/theapemachine/symm/market"
	feed "github.com/theapemachine/symm/signal"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in "thin"
markets by ranking a symbol's volume against the broader market.

1. What it measures exactly (in isolation)

The Liquidity signal identifies opportunities in thin markets by ranking
quote volume against peers.

Cross-Section Ranking: Ranks the dailyQuoteVol of all subscribed symbols.

Illiquidity Score: Specifically identifies symbols trading strictly below the
cross-section median of their peers.

Peak Scarcity: Uses a Peak gate to find symbols that are currently the most
illiquid in the universe.

---

2. Semantically, what story does it tell?

The Liquidity signal tells the story of convexity and market neglect.

The "Convexity" Story: It signals where a small amount of order flow will
cause the largest price displacement. It finds the "thinnest" pipes in the
exchange where price can move most easily.

The "Neglect" Story: It identifies assets that are being ignored by the
broader market, making them prime targets for sudden volatility once a trade
actually arrives.

1. Extreme Scarcity

The symbol is the most illiquid in the subscribed universe.
Indicators: Peak illiquidity rank with very low volume.
Semantic Meaning: High convexity/fragile — small flow, large displacement.

2. Median Depth

The symbol trades near the cross-section median.
Indicators: Middle rank with normal volume.
Semantic Meaning: Standard efficiency — typical market depth.

3. Robust Liquidity

The symbol ranks among the deepest markets.
Indicators: Bottom (deep) rank with high volume.
Semantic Meaning: Efficient/safe — thick, well-traded pipes.

# Summary of Liquidity Categories

| Category         | Rank vs. Peers   | Volume   | Market "Feel"                |
|:-----------------|:-----------------|:---------|:-----------------------------|
| Extreme Scarcity | Peak Illiquidity | Very Low | High Convexity / Fragile     |
| Median Depth     | Middle           | Normal   | Standard Efficiency          |
| Robust Liquidity | Bottom (Deep)    | High     | Efficient / Safe             |
*/
/*
Signal identifies opportunities in thin markets by ranking quote volume against peers.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	CrossSection *marketsection.CrossSection
	Metrics      *Metrics
	trade        *feed.Trade
	ticker       *feed.Ticker
	book         *feed.Book
}

/*
NewSignal composes the depth pipeline and subscribes to market channels.
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

	depth := algorithm.NewDepth()

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(symbol string, element []byte) {
		row, rowErr := tickerElementSymbolRow(symbol, element)

		if rowErr == nil {
			_ = crossSection.Observe(row)
		}
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		CrossSection: crossSection,
		Metrics:      NewMetrics(),
		trade:        feed.NewTrade(ctx),
		ticker:       tickerFeed,
		book:         feed.NewBook(ctx),
		algo: nomagique.Number(
			depth,
			probability.NewClassifier(
				depth.ScarcityReading(),
				depth.MedianReading(),
				depth.RobustReading(),
			),
		),
	}
}

func tickerElementSymbolRow(symbol string, element []byte) (*krakenmarket.Symbol, error) {
	var update krakenmarket.TickerUpdate

	if err := feed.UnmarshalElement(element, &update); err != nil {
		return nil, err
	}

	update.Symbol = symbol

	eventAt, eventOK := feed.ElementTime(element, "timestamp")

	if eventOK {
		update.Timestamp = eventAt
	}

	at := update.Timestamp

	if at.IsZero() {
		at = time.Now()
	}

	return update.CompleteSymbol(1, at)
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

	snapshot := scopeSnapshot(signal.CrossSection, scope, signal.trade, signal.ticker, signal.book)

	if snapshot.Spread <= 0 {
		return nil
	}

	peers := signal.CrossSection.Volumes()

	if len(peers) < 2 {
		return nil
	}

	feature := signal.featureArtifact(scope)

	if feature == nil {
		return nil
	}

	processed := datura.Acquire("liquidity", datura.APPJSON)

	if processed == nil {
		feature.Release()
		return nil
	}

	payload, payloadErr := feature.Payload()

	feature.Release()

	if payloadErr != nil {
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

	return processed
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	snapshot := scopeSnapshot(signal.CrossSection, scope, signal.trade, signal.ticker, signal.book)

	if snapshot.Price <= 0 || snapshot.Volume <= 0 {
		return nil
	}

	at := snapshot.Observed

	if at.IsZero() {
		at = time.Now()
	}

	row, rowErr := krakenmarket.NewSymbolRow(
		scope,
		snapshot.Price,
		0,
		snapshot.Volume,
		1,
		at,
	)

	if rowErr == nil {
		_ = signal.CrossSection.Observe(row)
	}

	peers := signal.CrossSection.Volumes()

	if len(peers) < 2 {
		return nil
	}

	relativeVolume, baselineReady, baselineErr := signal.Metrics.observeVolume(
		scope,
		at,
		snapshot.Volume,
	)

	if baselineErr != nil {
		return nil
	}

	scaledQuoteVol, scaledPeers := algorithm.AbsoluteScaledVolumes(
		snapshot.Volume,
		peers,
		relativeVolume,
		baselineReady,
	)

	const liquidityHeaderFloats = 4

	maxFloats := feed.MaxFeatureFloats(
		"depth-features",
		"features",
		scope,
		liquidityHeaderFloats,
	)
	maxPeers := maxFloats - liquidityHeaderFloats

	if maxPeers < 2 {
		return nil
	}

	if len(scaledPeers) > maxPeers {
		scaledPeers = feed.TrimLargestFloats(scaledPeers, maxPeers)
	}

	samples := []float64{scaledQuoteVol, float64(len(scaledPeers))}
	samples = append(samples, scaledPeers...)
	samples = append(samples, relativeVolume)

	baselineFlag := 0.0

	if baselineReady {
		baselineFlag = 1
	}

	samples = append(samples, baselineFlag)

	artifact := datura.Acquire("depth-features", datura.Artifact_Type_json)
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
