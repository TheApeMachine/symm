package toxicity

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
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
Toxicity is the Quality perspective, analyzing the "honesty" of the book by
tracking how makers behave when a trade approaches.

1. What it measures exactly (in isolation)

The Toxicity signal analyzes the "honesty" of the book by tracking how makers
behave when a trade approaches.

Cancel-to-Fill Asymmetry: Measures the ratio of liquidity being "pulled"
(cancelled) versus liquidity being "hit" (filled).

Toxic Level Detection: Flags large, young, near-touch blocks that disappear
rather than fill — this is the signature of a bluff.

Directional BookFlow: Emits a directional read based on which side of the book
is "retreating" (vacuum effect).

---

2. Semantically, what story does it tell?

The Toxicity signal tells the story of sincere versus fake liquidity.

The "Bluffing" Story: It exposes makers who are "fake-bidding" to create an
illusion of support, warning the engine that a wall is not "real" and will
crumble upon contact.

The "Vacuum" Story: It identifies a "liquidity vacuum" where one side pulls
away so aggressively that the resulting void "sucks" the price in that
direction.

1. Liquidity Vacuum

One side is retreating and creating a void.
Indicators: High cancel/fill asymmetry with one side retracting.
Semantic Meaning: Vacuum surcharge — the void itself drives price.

2. Toxic Bluff

Near-touch blocks disappear rather than fill.
Indicators: High cancel/fill ratio at near-touch levels.
Semantic Meaning: Manipulated/fake — a bluff wall about to crumble.

3. Hard Support

Liquidity fills rather than cancels on approach.
Indicators: Low cancel/fill ratio (high fill rate) with no side retracting.
Semantic Meaning: Robust/sincere — the wall will hold on contact.

# Summary of Toxicity Categories

| Category         | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:-----------------|:------------------|:----------------|:-----------------------|
| Liquidity Vacuum | High Asymmetry    | One Side        | Vacuum Surcharge       |
| Toxic Bluff      | High              | Near-Touch      | Manipulated / Fake     |
| Hard Support     | Low (High Fill)   | None            | Robust / Sincere       |
*/
/*
Signal analyzes book honesty from cancel-to-fill asymmetry and toxic level detection.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	surpriseTree *dmt.Tree
	Tracker      *Tracker
	measureScope string
	trade        *feed.Trade
	book         *feed.Book
	ticker       *feed.Ticker
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
	surpriseTree, _ := dmt.NewTree("")

	bookFeed := feed.NewBook(ctx)
	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		surpriseTree: surpriseTree,
		Tracker:      tracker,
		trade:        tradeFeed,
		book:         bookFeed,
		ticker:       tickerFeed,
		algo: nomagique.Number(
			bookQuality,
			probability.NewClassifier(
				bookQuality.BluffReading(),
				bookQuality.VacuumReading(),
				bookQuality.SupportReading(),
			),
			probability.NewDMTSurprise(
				surpriseTree,
				4,
			),
		),
	}

	bookFeed.OnUpdate = signal.ingestBookUpdate
	tradeFeed.OnUpdate = signal.ingestTradeUpdate
	tickerFeed.OnUpdate = signal.ingestTickerUpdate

	return signal
}

func (signal *Signal) ingestBookUpdate(bookRecord *feed.BookRecord) {
	if signal == nil || signal.Tracker == nil || bookRecord == nil {
		return
	}

	eventAt := bookRecord.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	update := bookRecordToKraken(bookRecord)
	pair := krakenmarket.Pair{}

	signal.Tracker.ApplyBookFrame(bookRecord.Symbol, pair, &update, eventAt)

	if len(bookRecord.Bids) == 0 || len(bookRecord.Asks) == 0 {
		return
	}

	bid := bookRecord.Bids[0].Price
	ask := bookRecord.Asks[0].Price

	if bid <= 0 || ask <= 0 {
		return
	}

	signal.Tracker.ObserveMid(bookRecord.Symbol, pair, (bid+ask)/2)
}

func (signal *Signal) ingestTradeUpdate(tradeRecord *feed.TradeRecord) {
	if signal == nil || signal.Tracker == nil || tradeRecord == nil {
		return
	}

	if tradeRecord.Price <= 0 || tradeRecord.Qty <= 0 {
		return
	}

	eventAt := tradeRecord.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	signal.Tracker.ObserveTrade(
		tradeRecord.Symbol,
		krakenmarket.Pair{},
		tradeRecord.Price,
		tradeRecord.Qty,
		eventAt,
	)
}

func (signal *Signal) ingestTickerUpdate(tickerRecord *feed.TickerRecord) {
	if signal == nil || signal.Tracker == nil || tickerRecord == nil {
		return
	}

	pair := krakenmarket.Pair{}

	if tickerRecord.Last > 0 {
		signal.Tracker.ObserveLast(tickerRecord.Symbol, pair, tickerRecord.Last)
	}

	if tickerRecord.Bid > 0 && tickerRecord.Ask > tickerRecord.Bid {
		signal.Tracker.ObserveMid(
			tickerRecord.Symbol,
			pair,
			(tickerRecord.Bid+tickerRecord.Ask)/2,
		)
	}
}

func bookRecordToKraken(record *feed.BookRecord) krakenmarket.BookUpdate {
	update := krakenmarket.BookUpdate{
		Symbol:    record.Symbol,
		Type:      "snapshot",
		Timestamp: record.Timestamp,
	}

	for _, bid := range record.Bids {
		update.Bids = append(update.Bids, krakenmarket.BookLevel{
			Price: bid.Price,
			Qty:   bid.Qty,
		})
	}

	for _, ask := range record.Asks {
		update.Asks = append(update.Asks, krakenmarket.BookLevel{
			Price: ask.Price,
			Qty:   ask.Qty,
		})
	}

	return update
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
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")
	signal.measureScope = scope
	signal.trade.Scope = scope
	signal.book.Scope = scope
	signal.ticker.Scope = scope
	signal.trade.ResetReadHead()
	signal.book.ResetReadHead()
	signal.ticker.ResetReadHead()

	out := datura.Acquire("toxicity-out", datura.Artifact_Type_json).WithScope(scope)

	if out == nil {
		return logic.Measurement{}, nil
	}

	errnie.Does(func() (int64, error) {
		return transport.Copy(
			signal.algo,
			io.MultiReader(signal.trade, signal.book, signal.ticker, signal),
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "failed to copy to algo", err))
	})

	if err := transport.NewFlipFlop(out, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](out, "bookquality.strength")

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	categoryIndex := datura.Peek[int](out, "classifier.category")

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence := datura.Peek[float64](out, "classifier.confidence")

	if !logic.ScalarFinite(confidence) || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	price := datura.Peek[float64](out, "bookquality.price")

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
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: time.Now(),
	}.UnlessPublishable(), nil
}

func (signal *Signal) Read(buffer []byte) (int, error) {
	artifact := signal.featureArtifact(signal.measureScope)

	if artifact == nil {
		return 0, io.EOF
	}

	return feed.ReadFeatureArtifact(buffer, artifact)
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

	if signal.surpriseTree != nil {
		_ = signal.surpriseTree.Close()
	}

	if signal.Tracker != nil {
		signal.Tracker.Close()
	}

	return nil
}
