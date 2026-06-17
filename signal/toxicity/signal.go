package toxicity

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
	feed "github.com/theapemachine/symm/signal"
)

/*
Toxicity (BookFlow) Signal: The Quality Perspective

What it measures exactly (in isolation)

The Toxicity signal analyzes the "honesty" of the book by tracking how makers
behave when a trade approaches.

Cancel-to-Fill Asymmetry: Measures the ratio of liquidity being "pulled"
(cancelled) versus liquidity being "hit" (filled).

Toxic Level Detection: Flags large, young, near-touch blocks that disappear
rather than fill—this is the signature of a bluff.

Directional BookFlow: Emits a directional read based on which side of the book
is "retreating" (vacuum effect).

Semantically, what story does it tell?

The "Bluffing" Story: It exposes makers who are "fake-bidding" to create an
illusion of support, warning the engine that a wall is not "real" and will
crumble upon contact.

The "Vacuum" Story: It identifies a "liquidity vacuum" where one side pulls away
so aggressively that the resulting void "sucks" the price in that direction.

| Category         | Cancel/Fill Ratio | Side Retracting | Market "Feel"      |
|------------------|-------------------|-----------------|--------------------|
| Liquidity Vacuum | High Asymmetry    | One Side        | Vacuum Surcharge   |
| Toxic Bluff      | High              | Near-Touch      | Manipulated / Fake |
| Hard Support     | Low (High Fill)   | None            | Robust / Sincere   |

That's right—we are down to the final two specialized perspectives: Correlation
and Exhaust. While the others focus on "Why" and "When" to enter, these two
focus on "Systemic Health" and "The Exit Strategy."
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	bookQuality *algorithm.BookQuality
	classifier  *probability.Classifier
	transition  *probability.Transition
	Tracker     *Tracker
	trade       *feed.Trade
	book        *feed.Book
	ticker      *feed.Ticker
}

/*
NewSignal composes the book-quality pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	tracker := NewConcurrentTracker(ctx)
	bookQuality := algorithm.NewBookQuality()
	classifier := probability.NewClassifier(
		bookQuality.BluffReading(),
		bookQuality.VacuumReading(),
		bookQuality.SupportReading(),
	)
	transition := probability.NewTransitionSurprise(
		4, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
	)

	bookFeed := feed.NewBook(ctx)
	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		bookQuality: bookQuality,
		classifier:  classifier,
		transition:  transition,
		Tracker:     tracker,
		trade:       tradeFeed,
		book:        bookFeed,
		ticker:      tickerFeed,
		algo: nomagique.Number(
			bookQuality,
			classifier,
			transition,
		),
	}

	bookFeed.OnUpdate = signal.ingestBookUpdate
	tradeFeed.OnUpdate = signal.ingestTradeUpdate
	tickerFeed.OnUpdate = signal.ingestTickerUpdate

	return signal
}

func (signal *Signal) ingestBookUpdate(bookUpdate *krakenmarket.BookUpdate) {
	if signal == nil || signal.Tracker == nil || bookUpdate == nil {
		return
	}

	eventAt := bookUpdate.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	pair := krakenmarket.Pair{}

	if bookUpdate.Type == "snapshot" {
		signal.Tracker.ApplyBookFrame(bookUpdate.Symbol, pair, bookUpdate, eventAt)
	} else {
		signal.Tracker.ApplyBookDelta(bookUpdate.Symbol, pair, bookUpdate, eventAt)
	}

	if len(bookUpdate.Bids) == 0 || len(bookUpdate.Asks) == 0 {
		return
	}

	bid := bookUpdate.Bids[0].Price
	ask := bookUpdate.Asks[0].Price

	if bid <= 0 || ask <= 0 {
		return
	}

	signal.Tracker.ObserveMid(bookUpdate.Symbol, pair, (bid+ask)/2)
}

func (signal *Signal) ingestTradeUpdate(tradeUpdate *krakenmarket.TradeUpdate) {
	if signal == nil || signal.Tracker == nil || tradeUpdate == nil {
		return
	}

	if tradeUpdate.Price <= 0 || tradeUpdate.Qty <= 0 {
		return
	}

	eventAt := tradeUpdate.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	signal.Tracker.ObserveTrade(
		tradeUpdate.Symbol,
		krakenmarket.Pair{},
		tradeUpdate.Price,
		tradeUpdate.Qty,
		eventAt,
	)
}

func (signal *Signal) ingestTickerUpdate(tickerUpdate *krakenmarket.TickerUpdate) {
	if signal == nil || signal.Tracker == nil || tickerUpdate == nil {
		return
	}

	pair := krakenmarket.Pair{}

	if tickerUpdate.Last > 0 {
		signal.Tracker.ObserveLast(tickerUpdate.Symbol, pair, tickerUpdate.Last)
	}

	if tickerUpdate.Bid > 0 && tickerUpdate.Ask > tickerUpdate.Bid {
		signal.Tracker.ObserveMid(tickerUpdate.Symbol, pair, (tickerUpdate.Bid+tickerUpdate.Ask)/2)
	}
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

	features := signal.featureArtifact(scope)

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "bookquality.strength")

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

	price := datura.Peek[float64](features, "bookquality.price")

	if price <= 0 {
		price = signal.trade.Snapshot(scope).Price
	}

	return logic.Measurement{
		Source:     logic.SourceToxicity,
		Symbol:     scope,
		Price:      price,
		Strength:   strength,
		Category:   toxicityCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: time.Now(),
	}.UnlessPublishable(), nil
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	features, ok := signal.Tracker.measureFeatures(scope)

	if !ok || features.lastPrice <= 0 {
		return nil
	}

	snapshot := features.snapshot
	lastPrice := features.lastPrice
	threshold := features.threshold
	churnGate := features.churnGate
	supportGate := features.supportGate
	vacuumStrengthCap := features.vacuumStrengthCap

	toxicNear := 0.0

	if snapshot.toxicNear {
		toxicNear = 1
	}

	artifact := datura.Acquire("bookquality-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(
		snapshot.cancelBid,
		snapshot.fillBid,
		snapshot.cancelAsk,
		snapshot.fillAsk,
		snapshot.bidDepth,
		snapshot.askDepth,
		toxicNear,
		snapshot.toxicBluffStrength,
		threshold,
		churnGate,
		supportGate,
		vacuumStrengthCap,
		lastPrice,
	))

	return artifact
}

func toxicityCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryToxicBluff
	case 2:
		return logic.CategoryLiquidityVacuum
	case 3:
		return logic.CategoryHardSupport
	default:
		return logic.CategoryTypeNone
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	if signal.Tracker != nil {
		signal.Tracker.Close()
	}

	return nil
}
