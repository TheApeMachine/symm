package pumpdump

import (
	"context"
	"io"
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
Signal: The Ignition Perspective

What it measures exactly (in isolation)

The PumpDump signal identifies pre-pump microstructure by looking for sudden
"verticality" in volume and price.

Volume Lift (RVOL): Measures fast and medium-term volume spikes against a
median hourly baseline.

Precursor Move: Uses a $PositiveMove$ dynamic to score how much the price
has already begun to detach from its recent anchor.

Spread Compression: Scores how much the bid/ask spread has tightened versus
its own baseline.

Move Classifier: A state-free primitive that maps these metrics into an
explicit "Pump" or "Dump" class.

Semantically, what story does it tell?

The "Ignition" Story: It identifies the exact moment a move stops being random
walk and becomes a vertical event driven by abnormal volume "lift".

The "Coiled Spring" Story: By tracking spread compression and book-side strength,
it identifies when a market is "tightly wound" and ready to snap.

# Probability Visualization Categories

| Category           | Volume Lift | Price Precursor | Market "Feel"        |
|--------------------|-------------|-----------------|----------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded    |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum     |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead          |
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	crossSection *CrossSection
	features     *Features
	trade        *feed.Trade
	book         *feed.Book
}

/*
NewSignal composes the verticality pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	verticality, verticalityErr := algorithm.NewVerticality()

	if verticalityErr != nil {
		cancel()

		return nil
	}

	crossSection := NewCrossSection(time.Minute, 0)
	tradeFeed := feed.NewTrade(ctx)
	tradeFeed.OnUpdate = func(update *krakenmarket.TradeUpdate) {
		_ = crossSection.observeTrade(update)
	}
	bookFeed := feed.NewBook(ctx)
	bookFeed.OnUpdate = func(update *krakenmarket.BookUpdate) {
		_ = crossSection.observeBook(update)
	}

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		crossSection: crossSection,
		trade:        tradeFeed,
		book:         bookFeed,
		features:     NewFeatures(ctx, crossSection, tradeFeed, bookFeed),
		algo: nomagique.Number(
			verticality,
			probability.NewClassifier(
				verticality.IgnitionReading(),
				verticality.CompressionReading(),
				verticality.TrendReading(),
				verticality.ExhaustionReading(),
			),
			probability.NewTransitionSurprise(5, 1.0/float64(viper.GetInt("signals.feed_ring_capacity"))),
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
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := in.Peek("scope")

	signal.trade.Scope = scope
	signal.book.Scope = scope
	signal.features.scope = scope

	frame := make([]byte, 8192)

	for _, reader := range []io.Reader{signal.trade, signal.book, signal.features} {
		readCount, _ := reader.Read(frame)

		if readCount == 0 {
			continue
		}

		if _, err := signal.algo.Write(frame[:readCount]); err != nil {
			return logic.Measurement{}, err
		}
	}

	out := datura.Acquire("pumpdump-out", datura.Artifact_Type_json)

	outCount, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	if _, err := out.Write(frame[:outCount]); err != nil {
		return logic.Measurement{}, err
	}

	// Input facts come straight from the feed; the pipeline output carries only
	// what the algorithm computed.
	snapshot := signal.features.Snapshot()

	categoryIndex := datura.Peek[int](out, "classifier.category")

	if categoryIndex == 0 {
		categoryIndex = datura.Peek[int](out, "verticality.category")
	}

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	strength := datura.Peek[float64](out, "verticality.strength")

	if strength <= 0 {
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
		Source:     logic.SourcePumpDump,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     snapshot.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   pumpDumpCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: datura.Peek[float64](out, "classifier.confidence"),
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}, nil
}

func pumpDumpCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryVerticalIgnition
	case 2:
		return logic.CategoryCoiledCompression
	case 3:
		return logic.CategoryOrganicTrend
	case 4:
		return logic.CategoryFadedExhaustion
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
