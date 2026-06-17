package sentiment

import (
	"context"
	"io"
	"math"
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
Sentiment is the Bullish Breadth perspective, measuring global market
conviction by looking at the behavior of the entire universe simultaneously.

1. What it measures exactly (in isolation)

The Sentiment signal measures global market conviction by looking at the
behavior of the entire universe simultaneously.

Market Breadth: The ratio of symbols with a positive $changePct$ versus the
total number of symbols.

Leadership Performance: Tracks the median performance of the "top" symbols to
see if the leaders are actually leading.

---

2. Semantically, what story does it tell?

The Sentiment signal tells the story of global conviction and rising tides.

The "Rising Tide" Story: It tells you if an asset's move is a solo effort or
if it is being carried by a global "risk-on" regime where every asset is
moving in unison.

The "Conviction" Story: It distinguishes between a "fake" leader move (where
only one asset is up) and a high-conviction market environment (breadth
> 0.55).

1. Risk-On Surge

Broad participation with strong leadership.
Indicators: High breadth (> 0.55) with strong leader performance.
Semantic Meaning: Rising tide/global buy — global risk-on regime.

2. Divergent Move

Leaders are strong but breadth is thin.
Indicators: Low breadth with strong leader performance.
Semantic Meaning: Idiosyncratic alpha — a solo leader effort.

3. Systemic Slump

Both breadth and leadership are weak.
Indicators: Low breadth with weak leader performance.
Semantic Meaning: Global risk-off — systemic slump across the universe.

# Summary of Sentiment Categories

| Category       | Breadth        | Leader Strength | Market "Feel"            |
|:---------------|:---------------|:----------------|:-------------------------|
| Risk-On Surge  | High (>0.55)   | Strong          | Rising Tide / Global Buy |
| Divergent Move | Low            | Strong          | Idiosyncratic Alpha      |
| Systemic Slump | Low            | Weak            | Global Risk-Off          |
*/
/*
Signal measures global market conviction from breadth and leadership performance.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	pool           *qpool.Q[any]
	subscribers    *sync.Map
	algo           io.ReadWriter
	CrossSection   *marketsection.CrossSection
	features       *Features
	trade          *feed.Trade
	ticker         *feed.Ticker
	book           *feed.Book
	lastCategory   logic.CategoryType
	lastCategoryAt time.Time
}

/*
NewSignal composes the conviction pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context, pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := marketsection.NewCrossSection(
		&marketsection.CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   64,
			MinBars:     8,
			BreadthHist: 8,
		},
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"failed to create cross section",
			err,
		).With("config", &marketsection.CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   64,
			MinBars:     8,
			BreadthHist: 8,
		}))
		cancel()
		return nil
	}

	conviction := algorithm.NewConviction()

	classifier := probability.NewClassifier(
		conviction.SurgeReading(),
		conviction.DivergentReading(),
		conviction.SlumpReading(),
	)

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
		CrossSection: crossSection,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		book:         bookFeed,
		features: NewFeatures(
			ctx, crossSection, tradeFeed, tickerFeed, bookFeed,
		),
		algo: nomagique.Number(
			conviction,
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

	features := signal.features.Artifact()

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	snapshot := signal.features.Snapshot()
	at := snapshot.Observed

	if at.IsZero() {
		at = time.Now()
	}

	rawCategory := sentimentCategory(
		int(datura.Peek[float64](features, "conviction.category")),
	)

	category := signal.applyHysteresis(
		rawCategory,
		datura.Peek[float64](features, "conviction.breadth"),
		datura.Peek[float64](features, "conviction.surgeThreshold"),
		at,
	)

	if sentimentCategoryIndex(category) == 0 {
		return logic.Measurement{}, nil
	}

	if snapshot.Spread <= 0 {
		return logic.Measurement{}, nil
	}

	position := logic.PositionTypeNone

	if snapshot.Move > 0 {
		position = logic.PositionTypeLong
	}

	if snapshot.Move < 0 {
		position = logic.PositionTypeShort
	}

	return logic.Measurement{
		Source:     logic.SourceSentiment,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   datura.Peek[float64](features, "conviction.strength"),
		Volume:     snapshot.Volume,
		Spread:     snapshot.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: datura.Peek[float64](features, "classifier.confidence"),
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: at,
	}.UnlessPublishable(), nil
}

func (signal *Signal) applyHysteresis(
	proposed logic.CategoryType,
	breadth float64,
	surgeThreshold float64,
	at time.Time,
) logic.CategoryType {
	exitRiskOn := math.Max(surgeThreshold-0.05, 0)

	if signal.lastCategory == logic.CategoryTypeNone {
		signal.lastCategory = proposed
		signal.lastCategoryAt = at

		return proposed
	}

	if proposed == signal.lastCategory {
		signal.lastCategoryAt = at
		return proposed
	}

	if at.Sub(signal.lastCategoryAt) < time.Second {
		return signal.lastCategory
	}

	if signal.lastCategory == logic.CategoryRiskOnSurge && breadth >= exitRiskOn {
		return signal.lastCategory
	}

	signal.lastCategory = proposed
	signal.lastCategoryAt = at

	return proposed
}

func sentimentCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryRiskOnSurge
	case 2:
		return logic.CategoryDivergentMove
	case 3:
		return logic.CategorySystemicSlump
	default:
		return logic.CategoryTypeNone
	}
}

func sentimentCategoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryRiskOnSurge:
		return 1
	case logic.CategoryDivergentMove:
		return 2
	case logic.CategorySystemicSlump:
		return 3
	default:
		return 0
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
