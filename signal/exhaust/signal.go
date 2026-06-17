package exhaust

import (
	"context"
	"io"
	"sync"

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
Signal classifies microstructure decay modes that advise when to close a position.

| Category            | Primary Metric  | Urgency  | Market "Feel"                 |
|---------------------|-----------------|----------|-------------------------------|
| Mechanical Collapse | Book Thinning   | High     | Crumbling Walls / Flash-Risk  |
| Thermal Exhaustion  | Pressure Fade   | Moderate | Dying Momentum / Topping Out  |
| Fragile Expansion   | Spread Widen    | Moderate | Increasing Friction / Risky   |
| Active Reversal     | Imbalance Flip  | High     | Sentiment Flip / Counter-Move |
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	decay        *algorithm.Decay
	classifier   *probability.Classifier
	crossSection *CrossSection
	book         *feed.Book
	trade        *feed.Trade
	ticker       *feed.Ticker
}

/*
NewSignal composes the decay pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection := NewCrossSection(featureRingCapacity)
	decay := algorithm.NewDecay()
	classifier := probability.NewClassifier(
		decay.MechanicalReading(),
		decay.FragileReading(),
		decay.ThermalReading(),
		decay.ReversalReading(),
	)

	bookFeed := feed.NewBook(ctx)
	bookFeed.OnUpdate = crossSection.observeBook
	tradeFeed := feed.NewTrade(ctx)
	tradeFeed.OnUpdate = crossSection.observeTrade
	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = crossSection.observeTick

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		decay:        decay,
		classifier:   classifier,
		crossSection: crossSection,
		book:         bookFeed,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		algo: nomagique.Number(
			decay,
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

	features := signal.featureArtifact(scope)

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "decay.urgency")

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

	snapshot := signal.trade.Snapshot(scope)

	price := snapshot.Price

	if price <= 0 {
		payload, payloadOK := signal.crossSection.payload(scope)

		if payloadOK && len(payload) > 0 {
			price = payload[0]
		}
	}

	return logic.Measurement{
		Source:     logic.SourceExhaustion,
		Symbol:     scope,
		Price:      price,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     signal.book.Spread(scope),
		Elapsed:    snapshot.Elapsed,
		Category:   exhaustCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}.UnlessPublishable(), nil
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	payload, ok := signal.crossSection.payload(scope)

	if !ok || len(payload) == 0 {
		return nil
	}

	artifact := datura.Acquire("decay-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(payload...))

	return artifact
}

func exhaustCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryMechanicalCollapse
	case 2:
		return logic.CategoryFragileExpansion
	case 3:
		return logic.CategoryThermalExhaustion
	case 4:
		return logic.CategoryActiveReversal
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
