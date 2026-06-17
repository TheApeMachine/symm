package depthflow

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
	bookflow     *algorithm.Bookflow
	classifier   *probability.Classifier
	CrossSection *marketsection.CrossSection
	features     *Features
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
	classifier := probability.NewClassifier(
		bookflow.LoadedReading(),
		bookflow.SpoofReading(),
		bookflow.ThinningReading(),
		bookflow.NeutralReading(),
	)

	tradeFeed := feed.NewTrade(ctx)
	bookFeed := feed.NewBook(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		bookflow:     bookflow,
		classifier:   classifier,
		CrossSection: crossSection,
		trade:        tradeFeed,
		book:         bookFeed,
		ticker:       tickerFeed,
		features:     NewFeatures(ctx, crossSection, bookFeed, tradeFeed),
		algo: nomagique.Number(
			bookflow,
			classifier,
			probability.NewTransitionSurprise(
				5, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
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

	snapshot := signal.features.BookSnapshot(scope)
	features := signal.features.Artifact()

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "bookflow.strength")

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

	tradeSnapshot := signal.trade.Snapshot(scope)

	return logic.Measurement{
		Source:     logic.SourceDepthFlow,
		Symbol:     scope,
		Price:      snapshot.Mid,
		Strength:   strength,
		Volume:     tradeSnapshot.Volume,
		Spread:     snapshot.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   depthflowCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}.UnlessPublishable(), nil
}

func depthflowCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryLoadedImbalance
	case 2:
		return logic.CategorySpoofTrap
	case 3:
		return logic.CategoryBookThinning
	case 4:
		return logic.CategoryDenseNeutrality
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
