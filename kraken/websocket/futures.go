package websocket

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	FuturesWebSocketURL = "wss://futures.kraken.com/ws/v1"
	pingInterval        = 30 * time.Second
)

/*
FuturesLive manages a real-time WebSocket session connected to Kraken Futures.
It ingests tickers (OI, basis, index, funding), trades (aggressor flow,
liquidations), and order books, routing them to the thesis symbol stream.
*/
type FuturesLive struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	status        *runtime.Status
	endpoint      string
	conn          *gorillawebsocket.Conn
	connMu        sync.Mutex
	subsMu        sync.RWMutex
	subscriptions map[string]map[string]struct{}
	observer      atomic.Pointer[func(string, time.Duration)]
	ingress       map[string]*runtime.Workload[*types.Envelope]
	capture       CaptureSink
	manifestSink  ManifestSink
	reconnect     func()
	connectCount  atomic.Int32

	// opStreams owns the operational per-stream epoch/sequence bookkeeping this
	// transport session maintains independent of Hindsight, mirroring Live.
	opMu      sync.Mutex
	opStreams map[hindsight.Stream]streamSpan
}

/*
NewFutures constructs a new Kraken Futures WebSocket connection. ingress is
keyed by feed name ("ticker"/"trade"), mirroring Live's ingress map, and is
where DispatchFrame pushes the envelope it builds from each parsed frame.
recorders, when present, receives every untouched transport payload exactly as
Live's recorder does, so futures frames reach the same capture sink as spot.
*/
func NewFutures(
	ctx context.Context,
	endpoint string,
	ingress map[string]*runtime.Workload[*types.Envelope],
	recorders ...CaptureSink,
) *FuturesLive {
	if endpoint == "" {
		endpoint = FuturesWebSocketURL
	}

	childCtx, cancel := context.WithCancel(ctx)

	futures := &FuturesLive{
		ctx:           childCtx,
		cancel:        cancel,
		status:        runtime.NewStatus(),
		endpoint:      endpoint,
		subscriptions: make(map[string]map[string]struct{}),
		ingress:       ingress,
	}

	if len(recorders) == 1 {
		futures.capture = recorders[0]

		if manifestSink, ok := recorders[0].(ManifestSink); ok {
			futures.manifestSink = manifestSink
		}
	}

	return futures
}

/*
SetReconnect installs the callback invoked when this futures session's transport
reconnects. The futures transport no longer fires it — dialAndServe reconnects
stream-isolated via futures.resubscribe only — but the seam remains for callers
that still wire it externally.
*/
func (futures *FuturesLive) SetReconnect(handler func()) {
	if futures == nil {
		return
	}

	futures.reconnect = handler
}

func (futures *FuturesLive) Name() string { return "kraken_futures" }

func (futures *FuturesLive) Error() error {
	return futures.err
}

func (futures *FuturesLive) Status() runtime.Stage {
	return futures.status.Current()
}

/*
SubFuturesTicker registers a subscription for real-time futures tickers.
*/
func (futures *FuturesLive) SubFuturesTicker(productIDs []string) error {
	return futures.subscribe("ticker", productIDs)
}

/*
SubFuturesTrades registers a subscription for real-time futures executions.
*/
func (futures *FuturesLive) SubFuturesTrades(productIDs []string) error {
	return futures.subscribe("trade", productIDs)
}

/*
SubFuturesBook registers a subscription for real-time futures order books.
*/
func (futures *FuturesLive) SubFuturesBook(productIDs []string) error {
	return futures.subscribe("book", productIDs)
}

func (futures *FuturesLive) subscribe(feed string, productIDs []string) error {
	if len(productIDs) == 0 {
		return nil
	}

	futures.subsMu.Lock()

	if _, exists := futures.subscriptions[feed]; !exists {
		futures.subscriptions[feed] = make(map[string]struct{})
	}

	for _, pid := range productIDs {
		futures.subscriptions[feed][pid] = struct{}{}
	}

	futures.subsMu.Unlock()

	return futures.writeSubscription(feed, productIDs)
}

func (futures *FuturesLive) writeSubscription(feed string, productIDs []string) error {
	futures.connMu.Lock()
	conn := futures.conn
	futures.connMu.Unlock()

	if conn == nil {
		return nil
	}

	sub := kraken.NewFuturesSubscription(feed, productIDs)

	if err := conn.WriteJSON(sub); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"futures: failed to write subscription",
			err,
		))
	}

	return nil
}

/*
Run connects to the WebSocket and enters the read/ping event loop.
*/
func (futures *FuturesLive) Run() error {
	for {
		select {
		case <-futures.ctx.Done():
			return futures.ctx.Err()
		default:
		}

		if err := futures.dialAndServe(); err != nil {
			if errors.Is(err, context.Canceled) || futures.ctx.Err() != nil {
				return nil
			}

			errnie.Warn(fmt.Sprintf("futures: websocket disconnected: %v, reconnecting in 2s", err))
			time.Sleep(2 * time.Second)
		}
	}
}

func (futures *FuturesLive) dialAndServe() error {
	dialer := gorillawebsocket.DefaultDialer
	conn, _, err := dialer.DialContext(futures.ctx, futures.endpoint, http.Header{})

	if err != nil {
		return err
	}

	futures.connMu.Lock()
	futures.conn = conn
	futures.connMu.Unlock()

	futures.status.Transition(runtime.READY)

	// A second (or later) dial in this process lifetime is a reconnect: advance
	// the operational stream epoch — a transport fact, independent of Hindsight
	// — and resubscribe to the active futures products only. The futures stream
	// must never soft-reboot the global spot subscription authority.
	if futures.connectCount.Add(1) > 1 {
		futures.reconnectStreams()
	}

	futures.resubscribe()

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	readDone := make(chan error, 1)

	go futures.readLoop(conn, readDone)

	for {
		select {
		case <-futures.ctx.Done():
			_ = conn.Close()
			return futures.ctx.Err()
		case err := <-readDone:
			_ = conn.Close()
			return err
		case <-pingTicker.C:
			if err := conn.WriteJSON(map[string]string{"event": "ping"}); err != nil {
				_ = conn.Close()
				return err
			}
		}
	}
}

func (futures *FuturesLive) resubscribe() {
	futures.subsMu.RLock()
	defer futures.subsMu.RUnlock()

	for feed, pidsMap := range futures.subscriptions {
		pids := make([]string, 0, len(pidsMap))

		for pid := range pidsMap {
			pids = append(pids, pid)
		}

		if len(pids) > 0 {
			_ = futures.writeSubscription(feed, pids)
		}
	}
}

func (futures *FuturesLive) readLoop(conn *gorillawebsocket.Conn, done chan<- error) {
	for {
		messageType, payload, err := conn.ReadMessage()

		if err != nil {
			done <- err
			return
		}

		if messageType != gorillawebsocket.TextMessage && messageType != gorillawebsocket.BinaryMessage {
			continue
		}

		// The capture identity is local to THIS frame, computed here and passed
		// into DispatchFrame — never stored as ambient mutable state that a
		// failed capture could leave holding a previous frame's identity.
		// The operational StreamRef is minted first, independent of capture.
		kind := frameKind(payload)
		streamRef := futures.nextStreamRef(kind)

		var captureID hindsight.CaptureIdentity

		if futures.capture != nil {
			// The reader reuses the payload buffer for the next frame, so the
			// capture owns its own copy of the exact bytes. The frame's feed
			// (falling back to its event) is recorded as the kind instead of a
			// blanket websocket_frame tag, mirroring the spot stream. Capture
			// mints the Hindsight identity, persists the frame with it, and
			// returns it so envelopes parsed from this frame carry the origin.
			identity, captureErr := futures.capture.Capture(
				kind,
				futures.endpoint,
				bytes.Clone(payload),
				time.Now().UTC(),
				streamRef,
			)

			if captureErr != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("futures: capture failed for %s frame: %s", kind, captureErr.Error()),
					captureErr,
				))

				futures.status.Transition(runtime.ERROR)
				continue
			}

			captureID = identity
		}

		futures.DispatchFrame(payload, captureID, streamRef)
	}
}

/*
frameKind names the origin of a raw futures frame for the capture sink: the
feed when present ("ticker"/"trade"/"book"), falling back to the control event
("pong"/"heartbeat"/"subscribed"/"info") for frames that carry no feed.
*/
func frameKind(raw []byte) string {
	if feed := utils.GetString(raw, "feed"); feed != "" {
		return feed
	}

	if event := utils.GetString(raw, "event"); event != "" {
		return event
	}

	return "unknown"
}

/*
nextStreamRef mints the operational StreamRef for one inbound futures frame on
the given feed. It mirrors Live's framing: transport-owned epoch/sequence,
independent of Hindsight.
*/
func (futures *FuturesLive) nextStreamRef(feed string) hindsight.StreamRef {
	if futures == nil {
		return hindsight.StreamRef{}
	}

	stream := hindsight.Stream(futures.endpoint + ":" + feed)

	futures.opMu.Lock()
	defer futures.opMu.Unlock()

	if futures.opStreams == nil {
		futures.opStreams = make(map[hindsight.Stream]streamSpan)
	}

	span := futures.opStreams[stream]

	if span.epoch == 0 {
		span.epoch = 1
	}

	span.sequence++
	futures.opStreams[stream] = span

	return hindsight.StreamRef{
		Stream:   stream,
		Epoch:    span.epoch,
		Sequence: span.sequence,
	}
}

/*
reconnectStreams bumps the operational epoch for every stream this futures
session has seen and resets each stream's per-epoch sequence, independent of any
capture sink.
*/
func (futures *FuturesLive) reconnectStreams() {
	if futures == nil {
		return
	}

	futures.opMu.Lock()
	defer futures.opMu.Unlock()

	for stream, span := range futures.opStreams {
		span.epoch++
		span.sequence = 0
		futures.opStreams[stream] = span
	}
}

/*
DispatchFrame decodes a raw wire payload from Kraken Futures and routes it. The
captureID is the identity minted for THIS frame and ref the transport-minted
operational StreamRef, both passed in explicitly so a frame can never inherit
another frame's identity.
*/
func (futures *FuturesLive) DispatchFrame(raw []byte, captureID hindsight.CaptureIdentity, ref hindsight.StreamRef) {
	if len(raw) == 0 {
		return
	}

	event := utils.GetString(raw, "event")

	if event == "pong" || event == "heartbeat" || event == "subscribed" {
		return
	}

	feed := utils.GetString(raw, "feed")

	switch feed {
	case "ticker":
		futures.dispatchTicker(raw, captureID, ref)
	case "trade", "trade_snapshot":
		futures.dispatchTrades(raw, captureID, ref)
	case "book", "book_snapshot":
		futures.dispatchBook(raw)
	}
}

func (futures *FuturesLive) dispatchTicker(raw []byte, captureID hindsight.CaptureIdentity, ref hindsight.StreamRef) {
	workload := futures.ingress["ticker"]

	if workload == nil {
		return
	}

	ticker := kraken.NewFuturesTicker(raw)

	if ticker == nil || ticker.Data.ProductID == "" {
		return
	}

	spotSymbol := kraken.FuturesProductIDToSpot(ticker.Data.ProductID)

	if spotSymbol == "" {
		return
	}

	ticker.Data.Symbol = spotSymbol

	envelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
	envelope.FuturesTickerData = ticker.Data
	envelope.CaptureID = captureID
	envelope.CaptureOrdinal = 0
	envelope.Stream = ref
	workload.Push(envelope)

	futures.writeManifest("futures.ticker", ticker.Data.Symbol, 0, captureID)
}

func (futures *FuturesLive) dispatchTrades(raw []byte, captureID hindsight.CaptureIdentity, ref hindsight.StreamRef) {
	workload := futures.ingress["trade"]

	if workload == nil {
		return
	}

	trades := kraken.NewFuturesTrade(raw)

	if trades == nil || len(trades.Data) == 0 {
		return
	}

	for index := range trades.Data {
		trade := trades.Data[index]
		spotSymbol := kraken.FuturesProductIDToSpot(trade.ProductID)

		if spotSymbol == "" {
			continue
		}

		trade.Symbol = spotSymbol

		envelope := types.NewEnvelope(types.EnvelopeFuturesTrade)
		envelope.FuturesTradeData = trade
		envelope.CaptureID = captureID
		envelope.CaptureOrdinal = uint64(index)
		envelope.Stream = ref
		workload.Push(envelope)

		futures.writeManifest("futures.trade", trade.Symbol, uint64(index), captureID)
	}
}

/*
writeManifest persists the EnvelopeManifest for one futures envelope, keyed by
the same CaptureIdentity and ordinal the envelope carries.
*/
func (futures *FuturesLive) writeManifest(workload, symbol string, ordinal uint64, captureID hindsight.CaptureIdentity) {
	if futures.manifestSink == nil {
		return
	}

	_ = futures.manifestSink.WriteManifest(hindsight.EnvelopeManifest{
		Envelope: hindsight.EnvelopeRef{
			Origin:  captureID,
			Ordinal: ordinal,
		},
		Workload:   workload,
		DomainKind: workload,
		Symbol:     symbol,
	})
}

func (futures *FuturesLive) dispatchBook(raw []byte) {
	book := kraken.NewFuturesBook(raw)

	if book == nil || book.Data.ProductID == "" {
		return
	}

	spotSymbol := kraken.FuturesProductIDToSpot(book.Data.ProductID)

	if spotSymbol == "" {
		return
	}

	book.Data.Symbol = spotSymbol
}

func (futures *FuturesLive) Close() error {
	if futures.cancel != nil {
		futures.cancel()
	}

	futures.connMu.Lock()

	if futures.conn != nil {
		_ = futures.conn.Close()
		futures.conn = nil
	}

	futures.connMu.Unlock()
	return nil
}
