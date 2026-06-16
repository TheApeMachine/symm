package toxicity

import (
	"context"
	"io"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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

	tracker := NewTracker()
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
	bookFeed.OnUpdate = func(bookUpdate *krakenmarket.BookUpdate) {
		eventAt := bookUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		pair := krakenmarket.Pair{}

		if bookUpdate.Type == "snapshot" {
			tracker.ApplyBookFrame(bookUpdate.Symbol, pair, bookUpdate, eventAt)
		} else {
			tracker.ApplyBookDelta(bookUpdate.Symbol, pair, bookUpdate, eventAt)
		}

		if len(bookUpdate.Bids) == 0 || len(bookUpdate.Asks) == 0 {
			return
		}

		bid := bookUpdate.Bids[0].Price
		ask := bookUpdate.Asks[0].Price

		if bid <= 0 || ask <= 0 {
			return
		}

		tracker.ObserveMid(bookUpdate.Symbol, pair, (bid+ask)/2)
	}

	tradeFeed := feed.NewTrade(ctx)
	tradeFeed.OnUpdate = func(tradeUpdate *krakenmarket.TradeUpdate) {
		if tradeUpdate.Price <= 0 || tradeUpdate.Qty <= 0 {
			return
		}

		eventAt := tradeUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		tracker.ObserveTrade(
			tradeUpdate.Symbol,
			krakenmarket.Pair{},
			tradeUpdate.Price,
			tradeUpdate.Qty,
			eventAt,
		)
	}

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(tickerUpdate *krakenmarket.TickerUpdate) {
		pair := krakenmarket.Pair{}

		if tickerUpdate.Last > 0 {
			tracker.ObserveLast(tickerUpdate.Symbol, pair, tickerUpdate.Last)
		}

		if tickerUpdate.Bid > 0 && tickerUpdate.Ask > tickerUpdate.Bid {
			tracker.ObserveMid(tickerUpdate.Symbol, pair, (tickerUpdate.Bid+tickerUpdate.Ask)/2)
		}
	}

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

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch artifact.Peek("role") {
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
	scope := in.Peek("scope")

	frame := make([]byte, 8192)

	readCount, readErr := signal.readFeatures(scope, frame)

	if readCount == 0 {
		return logic.Measurement{}, nil
	}

	if readErr != nil && readErr != io.EOF {
		return logic.Measurement{}, readErr
	}

	if _, err := signal.algo.Write(frame[:readCount]); err != nil {
		return logic.Measurement{}, err
	}

	out := datura.Acquire("toxicity-out", datura.Artifact_Type_json)

	outCount, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	if _, err := out.Write(frame[:outCount]); err != nil {
		return logic.Measurement{}, err
	}

	if !signal.bookQuality.Outcome().Eligible {
		return logic.Measurement{}, nil
	}

	categoryIndex := signal.bookQuality.Outcome().Category

	if categoryIndex == 0 {
		categoryIndex = signal.classifier.CategoryIndex()
	}

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence, confidenceErr := signal.classifier.Confidence(categoryIndex)

	if confidenceErr != nil {
		return logic.Measurement{}, confidenceErr
	}

	outcome := signal.bookQuality.Outcome()

	surprise, surpriseErr := signal.transition.Observe(
		signal.classifier.Probabilities(),
		categoryIndex,
	)

	if surpriseErr != nil {
		return logic.Measurement{}, surpriseErr
	}

	return logic.Measurement{
		Source:     logic.SourceToxicity,
		Symbol:     scope,
		Price:      outcome.Price,
		Strength:   outcome.Strength,
		Category:   toxicityCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: time.Now(),
	}, nil
}

func (signal *Signal) readFeatures(scope string, buffer []byte) (int, error) {
	at := time.Now()

	snapshot, lastPrice, ok := signal.Tracker.Snapshot(scope, at)

	if !ok || lastPrice <= 0 {
		return 0, io.EOF
	}

	threshold := signal.Tracker.fillToCancelThreshold()
	churnGate := signal.Tracker.churnRatioGate(scope)
	supportGate := signal.Tracker.supportRatioGate(scope, threshold)

	bidRatio := cancelFillRatio(snapshot.cancelBid, snapshot.fillBid)
	askRatio := cancelFillRatio(snapshot.cancelAsk, snapshot.fillAsk)
	maxRatio := math.Max(bidRatio, askRatio)
	vacuumStrengthCap := signal.Tracker.vacuumStrengthLimit(scope, threshold, maxRatio)

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

	return artifact.Read(buffer)
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

	return nil
}
