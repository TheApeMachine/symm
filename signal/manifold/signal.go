package manifold

import (
	"context"
	"io"
	"strings"
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
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
	field       *Field
	serial      *compute.SerialPool
	trade       *feed.Trade
	book        *feed.Book
	ticker      *feed.Ticker
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

	if field != nil {
		field.bindSerial(serial)
	}

	manifoldstate := algorithm.NewManifoldstate()

	bookFeed := feed.NewBook(ctx)
	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        dmt.NewTree(""),
		field:       field,
		serial:      serial,
		trade:       tradeFeed,
		book:        bookFeed,
		ticker:      tickerFeed,
		algo: nomagique.Number(
			manifoldstate,
			probability.NewClassifier(
				manifoldstate.HerdReading(),
				manifoldstate.ShockReading(),
				manifoldstate.DriftReading(),
				manifoldstate.NoiseReading(),
			),
		),
	}

	bookFeed.OnRecord = func(bookRecord *feed.BookRecord) {
		if bookRecord == nil || field == nil {
			return
		}

		eventAt := bookRecord.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := bookRecordToKraken(bookRecord)
		at := eventAt

		if _, identityErr := krakenmarket.FuturesIdentityFromProduct(bookRecord.Symbol); identityErr == nil {
			if feedErr := field.enqueueFuturesBook(frame, at); feedErr != nil {
				errnie.Error(manifoldFeedError(feedErr))
			}

			signal.publishFeatures(bookRecord.Symbol)

			return
		}

		if feedErr := field.enqueueBook(frame, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}

		signal.publishFeatures(bookRecord.Symbol)
	}

	tradeFeed.OnRecord = func(tradeRecord *feed.TradeRecord) {
		if tradeRecord == nil || tradeRecord.Price <= 0 || tradeRecord.Qty <= 0 || field == nil {
			return
		}

		eventAt := tradeRecord.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		at := eventAt
		tradeCopy := tradeRecordToKraken(tradeRecord)

		if feedErr := field.enqueueTrade(&tradeCopy, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}

		signal.publishFeatures(tradeRecord.Symbol)
	}

	tickerFeed.OnRecord = func(tickerRecord *feed.TickerRecord) {
		if tickerRecord == nil || field == nil {
			return
		}

		eventAt := tickerRecord.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := tickerRecordToKraken(tickerRecord)
		at := eventAt

		if feedErr := field.enqueueTicker(frame, at); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}

		signal.publishFeatures(tickerRecord.Symbol)
	}

	return signal
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

func tradeRecordToKraken(record *feed.TradeRecord) krakenmarket.TradeUpdate {
	return krakenmarket.TradeUpdate{
		Symbol:    record.Symbol,
		Side:      record.Side,
		Price:     record.Price,
		Qty:       record.Qty,
		Timestamp: record.Timestamp,
	}
}

func tickerRecordToKraken(record *feed.TickerRecord) krakenmarket.TickerUpdate {
	return krakenmarket.TickerUpdate{
		Symbol:    record.Symbol,
		Ask:       record.Ask,
		AskQty:    record.AskQty,
		Bid:       record.Bid,
		BidQty:    record.BidQty,
		Change:    record.Change,
		ChangePct: record.ChangePct,
		High:      record.High,
		Last:      record.Last,
		Low:       record.Low,
		Volume:    record.Volume,
		VWAP:      record.VWAP,
		Timestamp: record.Timestamp,
	}
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
		signal.Measure(*artifact)
	}

	return nil
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	signal.publishFeatures(scope)

	var measurement *datura.Artifact

	prefix := "features/" + scope

	for inbound := range signal.tree.Seek([]byte(prefix)) {
		processed := datura.Acquire("manifold", datura.APPJSON)

		if processed == nil {
			continue
		}

		payload, payloadErr := inbound.Payload()

		if payloadErr != nil {
			processed.Release()
			continue
		}

		if processed.WithPayload(payload) == nil {
			processed.Release()
			continue
		}

		if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
			_ = processed.WithError(flipErr)
		}

		if datura.Peek[int](processed, "classifier.category") <= 0 {
			processed.Release()
			continue
		}

		if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
			processed.Release()
			continue
		}

		processed.WithRole("measurement")
		processed.WithScope(scope)

		measurement = processed
	}

	return measurement
}

func (signal *Signal) publishFeatures(scope string) {
	artifact := signal.featureArtifact(scope)

	if artifact == nil || signal.tree == nil {
		return
	}

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	if signal == nil || signal.field == nil {
		return nil
	}

	reading, price, _, ok := signal.field.Reading(scope)

	if !ok || !reading.IsFinite() {
		return nil
	}

	artifact := datura.Acquire("manifold-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(
		reading.PressureGradNorm,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
		price,
	))

	return artifact
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

func manifoldFeedError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "non-finite") {
		return nil
	}

	return errnie.Error(err)
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

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.serial != nil {
		signal.serial.Close()
	}

	if signal.field != nil {
		signal.field.Close()
	}

	return err
}
