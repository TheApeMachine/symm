package manifold

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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
	uiBroadcast   *qpool.BroadcastGroup
	algo          io.ReadWriter
	field         *Field
	worker        *compute.BatchWorker
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

	worker := compute.NewBatchWorker(
		ctx,
		manifoldBatchCapacity,
		100*time.Millisecond,
	)

	field.bindWorker(worker)

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
		worker:        worker,
		manifoldstate: manifoldstate,
		classifier:    classifier,
		transition:    transition,
	}

	onFeed := func(symbol string, eventAt time.Time) {
		_ = signal.publishFieldSnapshot(eventAt)
	}

	bookFeed := feed.NewBook(ctx)
	bookFeed.OnUpdate = func(bookUpdate *krakenmarket.BookUpdate) {
		eventAt := bookUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := *bookUpdate
		at := eventAt
		symbol := bookUpdate.Symbol

		if _, identityErr := krakenmarket.FuturesIdentityFromProduct(bookUpdate.Symbol); identityErr == nil {
			if feedErr := field.enqueueFuturesBook(frame, at); feedErr != nil {
				errnie.Error(manifoldFeedError(feedErr))
			}
		} else {
			if feedErr := field.enqueueBook(frame, at); feedErr != nil {
				errnie.Error(manifoldFeedError(feedErr))
			}
		}

		onFeed(symbol, at)
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
		symbol := tradeUpdate.Symbol
		tradeCopy := *tradeUpdate

		if feedErr := field.enqueueTrade(&tradeCopy, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}

		onFeed(symbol, at)
	}

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(tickerUpdate *krakenmarket.TickerUpdate) {
		eventAt := tickerUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := *tickerUpdate
		at := eventAt
		symbol := tickerUpdate.Symbol

		if feedErr := field.enqueueTicker(frame, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}

		onFeed(symbol, at)
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

	field.SetSnapshotPublisher(func(at time.Time) error {
		return signal.publishFieldSnapshot(at)
	})

	signal.uiBroadcast = pool.CreateBroadcastGroup("ui")

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

	signal.features.scope = scope
	signal.ticker.Scope = scope

	snapshot := signal.ticker.Snapshot(scope)

	frame := make([]byte, 8192)

	readCount, readErr := signal.features.Read(frame)

	if readCount == 0 {
		return logic.Measurement{}, nil
	}

	if readErr != nil && readErr != io.EOF {
		return logic.Measurement{}, readErr
	}

	if _, err := signal.algo.Write(frame[:readCount]); err != nil {
		return logic.Measurement{}, err
	}

	outCount, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	_ = outCount

	if !signal.manifoldstate.Outcome().Eligible {
		return logic.Measurement{}, nil
	}

	categoryIndex := signal.manifoldstate.Outcome().Category

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

	outcome := signal.manifoldstate.Outcome()

	surprise, surpriseErr := signal.transition.Observe(
		signal.classifier.Probabilities(),
		categoryIndex,
	)

	if surpriseErr != nil {
		return logic.Measurement{}, surpriseErr
	}

	observedAt := snapshot.Observed

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	return logic.Measurement{
		Source:     logic.SourceManifold,
		Symbol:     scope,
		Price:      outcome.Price,
		Strength:   outcome.Strength,
		Volume:     snapshot.Volume,
		Spread:     outcome.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   manifoldCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: observedAt,
	}, nil
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
publishFieldSnapshot ships the last integrated rho projection to the ui broadcast.
*/
func (signal *Signal) publishFieldSnapshot(eventAt time.Time) error {
	if signal == nil || signal.uiBroadcast == nil {
		return nil
	}

	if eventAt.IsZero() {
		return fmt.Errorf("manifold: field snapshot event time is zero")
	}

	if !signal.field.hasPublishableSnapshot() {
		return nil
	}

	payload, err := signal.field.snapshotPayload(eventAt)

	if err != nil {
		return err
	}

	if payload == nil {
		return nil
	}

	artifact := datura.Acquire("manifold-field", datura.Artifact_Type_json)
	artifact.WithRole("manifold")
	artifact.WithDestination("ui")

	marshaled, marshalErr := sonic.Marshal(payload)

	if marshalErr != nil {
		return fmt.Errorf("manifold: marshal field snapshot: %w", marshalErr)
	}

	if setErr := artifact.SetPayload(marshaled); setErr != nil {
		return setErr
	}

	return signal.uiBroadcast.Send(artifact)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	if signal.worker != nil {
		signal.worker.Close()
	}

	if signal.field != nil {
		signal.field.Close()
	}

	return nil
}
