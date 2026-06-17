package manifold

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
	"github.com/theapemachine/symm/signal/compute"
)

const manifoldBatchCapacity = 8192

/*
Signal classifies the 3D manifold state for one symbol.

PressureGradNorm captures cross-axis basis and beta dislocations.
CoherenceMag2 captures systemic herding / superfluid collapse.
GuidanceSpeed is the pilot-wave trend velocity from aligned Ψ.
ViscosityProxy inverts divergence — laminar when large, turbulent when small.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	subscribers   *sync.Map
	algo          io.ReadWriter
	field         *Field
	serial        *compute.SerialPool
	manifoldstate *algorithm.Manifoldstate
	classifier    *probability.Classifier
	transition    *probability.Transition
	features      *Features
	trade         *feed.Trade
	book          *feed.Book
	ticker        *feed.Ticker
}

/*
NewSignal composes the manifold-state pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	field := errnie.Does(func() (*Field, error) {
		return NewField()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create field",
			err,
		))
	}).Value()

	serial := compute.NewSerialPool(
		ctx,
		manifoldBatchCapacity,
		100*time.Millisecond,
	)

	field.bindSerial(serial)

	manifoldstate := algorithm.NewManifoldstate()
	classifier := probability.NewClassifier(
		manifoldstate.HerdReading(),
		manifoldstate.ShockReading(),
		manifoldstate.DriftReading(),
		manifoldstate.NoiseReading(),
	)
	transition := probability.NewTransitionSurprise(
		5, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
	)

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		subscribers:   &sync.Map{},
		field:         field,
		serial:        serial,
		manifoldstate: manifoldstate,
		classifier:    classifier,
		transition:    transition,
	}

	bookFeed := feed.NewBook(ctx)
	bookFeed.OnUpdate = func(bookUpdate *krakenmarket.BookUpdate) {
		eventAt := bookUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := *bookUpdate
		at := eventAt

		if _, identityErr := krakenmarket.FuturesIdentityFromProduct(bookUpdate.Symbol); identityErr == nil {
			if feedErr := field.enqueueFuturesBook(frame, at); feedErr != nil {
				errnie.Error(manifoldFeedError(feedErr))
			}

			return
		}

		if feedErr := field.enqueueBook(frame, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}
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

		at := eventAt
		tradeCopy := *tradeUpdate

		if feedErr := field.enqueueTrade(&tradeCopy, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}
	}

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(tickerUpdate *krakenmarket.TickerUpdate) {
		eventAt := tickerUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := *tickerUpdate
		at := eventAt

		if feedErr := field.enqueueTicker(frame, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}
	}

	signal.trade = tradeFeed
	signal.book = bookFeed
	signal.ticker = tickerFeed
	signal.features = NewFeatures(ctx, field)
	signal.algo = nomagique.Number(
		manifoldstate,
		classifier,
		transition,
	)

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
	signal.ticker.Scope = scope

	snapshot := signal.ticker.Snapshot(scope)
	features := signal.features.Artifact()

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "manifoldstate.strength")

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

	observedAt := snapshot.Observed

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	return logic.Measurement{
		Source:     logic.SourceManifold,
		Symbol:     scope,
		Price:      snapshot.Last,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     signal.book.Spread(scope),
		Elapsed:    snapshot.Elapsed,
		Category:   manifoldCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: observedAt,
	}.UnlessPublishable(), nil
}

func manifoldCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategorySystemicHerd
	case 2:
		return logic.CategoryLiquidityShock
	case 3:
		return logic.CategorySynchronizedDrift
	case 4:
		return logic.CategoryStochasticNoise
	default:
		return logic.CategoryTypeNone
	}
}

/*
FieldSnapshot builds the manifold dashboard payload from the last integrated field.
*/
func (signal *Signal) FieldSnapshot(eventAt time.Time) (map[string]any, error) {
	if signal == nil || signal.field == nil {
		return nil, nil
	}

	if !signal.field.hasPublishableSnapshot() {
		return nil, nil
	}

	return signal.field.snapshotPayload(eventAt)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	if signal.serial != nil {
		signal.serial.Close()
	}

	if signal.field != nil {
		signal.field.Close()
	}

	return nil
}
