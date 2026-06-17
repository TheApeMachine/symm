package liquidity

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
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
	depth        *algorithm.Depth
	classifier   *probability.Classifier
	CrossSection *marketsection.CrossSection
	Metrics      *Metrics
	features     *Features
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
	classifier := probability.NewClassifier(
		depth.ScarcityReading(),
		depth.MedianReading(),
		depth.RobustReading(),
	)

	metrics := NewMetrics()
	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(update *krakenmarket.TickerUpdate) {
		if update == nil {
			return
		}

		row, rowErr := update.CompleteSymbol(1, update.Timestamp)

		if rowErr == nil {
			_ = crossSection.Observe(row)
		}
	}
	bookFeed := feed.NewBook(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		depth:        depth,
		classifier:   classifier,
		CrossSection: crossSection,
		Metrics:      metrics,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		book:         bookFeed,
		features:     NewFeatures(ctx, crossSection, metrics, tradeFeed, tickerFeed, bookFeed),
		algo: nomagique.Number(
			depth,
			classifier,
			probability.NewTransitionSurprise(
				4, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
			),
		),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book":
		signal.book.Update(
			datura.As[krakenmarket.BookUpdates](artifact),
		)
	case "trade":
		signal.trade.Update(
			datura.As[krakenmarket.TradeUpdates](artifact),
		)
	case "ticker":
		signal.ticker.Update(
			datura.As[krakenmarket.TickerUpdates](artifact),
		)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")

	signal.features.scope = scope

	snapshot := signal.features.Snapshot()
	at := snapshot.Observed

	if at.IsZero() {
		at = time.Now()
	}

	peers := signal.CrossSection.Volumes()

	if len(peers) < 2 {
		return logic.Measurement{}, nil
	}

	features := signal.features.Artifact()

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "depth.strength")

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	categoryIndex := datura.Peek[int](features, "classifier.category")

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence := datura.Peek[float64](features, "classifier.confidence")

	if !logic.ScalarFinite(confidence) || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	if snapshot.Spread <= 0 {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceLiquidity,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     snapshot.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   liquidityCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: at,
	}.UnlessPublishable(), nil
}

func liquidityCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryExtremeScarcity
	case 2:
		return logic.CategoryMedianDepth
	case 3:
		return logic.CategoryRobustLiquidity
	default:
		return logic.CategoryTypeNone
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
