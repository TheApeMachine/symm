package leadlag

import (
	"context"
	"io"
	"sync"
	"time"

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
Signal measures temporal correlation between the anchor pair and each follower.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	lag         *algorithm.Lag
	classifier  *probability.Classifier
	Section     *Section
	trade       *feed.Trade
	ticker      *feed.Ticker
}

/*
NewSignal composes the lag pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section, _ := NewSectionFromConfig()
	lagStage := algorithm.NewLag()
	classifier := probability.NewClassifier(
		lagStage.InefficientReading(),
		lagStage.SyncReading(),
		lagStage.DecoupledReading(),
		lagStage.StallReading(),
	)

	tradeFeed := feed.NewTrade(ctx)
	tradeFeed.OnUpdate = func(update *krakenmarket.TradeUpdate) {
		if update == nil || update.Price <= 0 {
			return
		}

		section.ObservePrice(update.Symbol, update.Price, update.Timestamp)
	}

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(update *krakenmarket.TickerUpdate) {
		if update == nil {
			return
		}

		price := update.Last

		if price <= 0 {
			price = (update.Ask + update.Bid) / 2
		}

		if price <= 0 {
			return
		}

		section.ObservePrice(update.Symbol, price, update.Timestamp)
	}

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		lag:         lagStage,
		classifier:  classifier,
		Section:     section,
		trade:       tradeFeed,
		ticker:      tickerFeed,
		algo: nomagique.Number(
			lagStage,
			classifier,
			probability.NewTransitionSurprise(
				5, 1.0/float64(feed.FeedRingCapacity()),
			),
		),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
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

	snapshot := signal.Section.Features(scope)
	features := signal.featureArtifact(scope)

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "lag.strength")

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

	observedAt := snapshot.ObservedAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	price := snapshot.Price

	return logic.Measurement{
		Source:     logic.SourceLeadLag,
		Symbol:     scope,
		Price:      price,
		Strength:   strength,
		Category:   leadlagCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: observedAt,
	}.UnlessPublishable(), nil
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	snapshot := signal.Section.Features(scope)

	if snapshot.Price <= 0 {
		return nil
	}

	isAnchor := 0.0

	if snapshot.IsAnchor {
		isAnchor = 1
	}

	moveReady := 0.0

	if snapshot.MoveReady {
		moveReady = 1
	}

	moveMoved := 0.0

	if snapshot.MoveMoved {
		moveMoved = 1
	}

	lagOK := 0.0

	if snapshot.LagOK {
		lagOK = 1
	}

	contempOK := 0.0

	if snapshot.ContempOK {
		contempOK = 1
	}

	artifact := datura.Acquire("lag-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(
		isAnchor,
		snapshot.Price,
		moveReady,
		moveMoved,
		snapshot.StallMargin,
		lagOK,
		float64(snapshot.LagBars),
		snapshot.LagCorr,
		contempOK,
		snapshot.ContempCorr,
		float64(snapshot.SampleCount),
	))

	return artifact
}

func leadlagCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryInefficientLag
	case 2:
		return logic.CategorySynchronizedDrift
	case 3:
		return logic.CategoryDecoupledMove
	case 4:
		return logic.CategoryAnchorStall
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
