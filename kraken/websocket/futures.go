package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/derivatives"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
futuresMap parses one raw futures frame per feed, mirroring entityMap. Kraken
Futures names its streams "feed" where spot names them "channel", and answers
lifecycle requests on "event", so the keys are the futures wire's own names.
*/
var futuresMap = map[string]func([]byte) any{
	"ticker":           func(buf []byte) any { return kraken.NewFuturesTicker(buf) },
	"ticker_lite":      func(buf []byte) any { return kraken.NewFuturesTicker(buf) },
	"trade":            func(buf []byte) any { return kraken.NewFuturesTrade(buf) },
	"trade_snapshot":   func(buf []byte) any { return kraken.NewFuturesTrade(buf) },
	"book":             func(buf []byte) any { return kraken.NewFuturesBook(buf) },
	"book_snapshot":    func(buf []byte) any { return kraken.NewFuturesBook(buf) },
	"heartbeat":        func(buf []byte) any { return true },
	"info":             func(buf []byte) any { return true },
	"subscribed":       func(buf []byte) any { return true },
	"unsubscribed":     func(buf []byte) any { return true },
	"alert":            func(buf []byte) any { return true },
	"challenge":        func(buf []byte) any { return true },
	"error":            func(buf []byte) any { return true },
	"subscribe":        func(buf []byte) any { return true },
	"unsubscribe":      func(buf []byte) any { return true },
	"product_snapshot": func(buf []byte) any { return true },
}

/*
FuturesLive is one futures websocket session: SDK client, feed fan-out, and
Sub* resubscribe after the SDK reconnects. It mirrors Live exactly — the same
SDK transport, callback seams, keepalive, and reconnect escalation — because
derivatives.WebSocket and spot.WebSocket embed the same kraken.WebSocket.
*/
type FuturesLive struct {
	ctx            context.Context
	cancel         context.CancelFunc
	status         *runtime.Status
	err            error
	client         *derivatives.WebSocket
	ingress        map[string]runtime.Ingress[*types.Envelope]
	simulator      *Simulator
	symbols        []string
	callbacks      *sync.Map
	capture        CaptureSink
	manifestSink   ManifestSink
	captureName    string
	reconnect      func()
	observer       atomic.Pointer[func(string, time.Duration)]
	connectedCount atomic.Int32

	// streams owns this session's operational epoch/sequence bookkeeping.
	streams *Streams

	// recovery decides when this session's reconnects have stopped being worth
	// trying and asks the process owner to reboot.
	recovery *Recovery

	// pinger owns this session's keepalive loop.
	pinger *Pinger

	// resolve maps an inbound product identifier to the spot symbol carrying
	// it. Futures frames identify themselves by product_id alone, and every
	// stage downstream keys on the spot symbol, so the frame is attributed
	// here. The instrument registry owns the mapping and installs it.
	resolve atomic.Pointer[func(string) (string, bool)]
}

/*
Capture returns the underlying capture sink attached to the futures connection.
*/
func (futures *FuturesLive) Capture() CaptureSink {
	if futures == nil {
		return nil
	}

	return futures.capture
}

/*
SetReconnect installs the callback invoked when this session's transport
reconnects (a second or later connect within one process lifetime).
*/
func (futures *FuturesLive) SetReconnect(handler func()) {
	if futures == nil {
		return
	}

	futures.reconnect = handler
}

/*
SetUnrecoverable installs the callback invoked once when this session exhausts
its reconnect budget — the transport keeps dialing but the venue drops the
session immediately, which reconnecting cannot resolve.
*/
func (futures *FuturesLive) SetUnrecoverable(handler func(reason string)) {
	if futures == nil {
		return
	}

	futures.recovery.OnUnrecoverable(handler)
}

/*
SetResolver installs the product-identifier to spot-symbol mapping this session
stamps onto every inbound frame. The instrument registry owns the market
universe, so it owns this mapping too; the transport only applies it.
*/
func (futures *FuturesLive) SetResolver(resolve func(string) (string, bool)) {
	if futures == nil {
		return
	}

	futures.resolve.Store(&resolve)
}

/*
attribute stamps the spot symbol onto every record parsed from one futures
frame. A frame for a product the venue does not list, or one arriving before the
mapping is installed, has no symbol to attribute to and is dropped: every stage
downstream keys on the spot symbol, so an unattributed frame is not routable.
*/
func (futures *FuturesLive) attribute(parsed any) bool {
	resolve := futures.resolve.Load()

	if resolve == nil {
		return false
	}

	switch record := parsed.(type) {
	case *kraken.FuturesTicker:
		symbol, listed := (*resolve)(record.Data.ProductID)

		if !listed {
			return false
		}

		record.Data.Symbol = symbol

		return true
	case *kraken.FuturesTrade:
		attributed := record.Data[:0]

		for _, data := range record.Data {
			symbol, listed := (*resolve)(data.ProductID)

			if !listed {
				continue
			}

			data.Symbol = symbol
			attributed = append(attributed, data)
		}

		record.Data = attributed

		return len(record.Data) > 0
	default:
		return false
	}
}

/*
NewFutures opens a futures websocket session and wires SDK callbacks in the
constructor, mirroring New.
*/
func NewFutures(
	ctx context.Context,
	endpoint string,
	workloads map[string]runtime.Ingress[*types.Envelope],
	recorders ...CaptureSink,
) *FuturesLive {
	return NewFuturesWithClient(ctx, endpoint, workloads, nil, recorders...)
}

/*
NewFuturesWithClient opens a futures websocket session using an injected
derivatives.WebSocket client instance, mirroring NewWithClient.
*/
func NewFuturesWithClient(
	ctx context.Context,
	endpoint string,
	workloads map[string]runtime.Ingress[*types.Envelope],
	client *derivatives.WebSocket,
	recorders ...CaptureSink,
) *FuturesLive {
	if endpoint == "" {
		endpoint = system.Cfg.WebSocket.Endpoints.Futures
	}

	if client == nil {
		client = derivatives.NewWebSocket()
		client.URL = endpoint
	}

	ctx, cancel := context.WithCancel(ctx)

	futures := &FuturesLive{
		ctx:         ctx,
		cancel:      cancel,
		status:      runtime.NewStatus(),
		client:      client,
		callbacks:   &sync.Map{},
		ingress:     workloads,
		captureName: "futures",
		streams:     NewStreams(client.URL),
		recovery:    NewRecovery("futures"),
	}

	futures.pinger = NewPinger("futures", func() error {
		if !futures.client.IsActive() {
			return nil
		}

		return futures.client.WriteMessage(gorillawebsocket.PingMessage, nil)
	})

	// A failed keepalive is the only evidence of a half-open socket: the SDK
	// reports a disconnect from its read loop, which stays blocked on a peer
	// that will never answer, so the session would otherwise keep writing into
	// a dead socket for the rest of the process lifetime.
	futures.pinger.OnFailed(func(error) {
		futures.reopen()
	})

	if len(recorders) == 1 {
		futures.capture = recorders[0]

		if manifestSink, ok := recorders[0].(ManifestSink); ok {
			futures.manifestSink = manifestSink
		}
	}

	futures.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		// Real inbound traffic, not a successful dial, is what proves the venue
		// accepted this session, so the dead-reconnect count clears here rather
		// than in OnConnected.
		futures.recovery.Delivered()

		raw := event.Data.Bytes()

		// Kraken Futures names its data streams "feed" and its lifecycle
		// acknowledgements "event", where spot uses "channel" and "method".
		feed := utils.GetString(raw, "feed")

		if feed == "" {
			feed = utils.GetString(raw, "event")
		}

		if feed == "" {
			return
		}

		// Every futures frame reaches the same capture sink as the spot stream,
		// so the events store sees the whole system. Capture mints the
		// Hindsight capture identity, persists the raw frame with it, and
		// returns it so every envelope parsed from this frame carries the exact
		// same origin. The transport mints the operational StreamRef first —
		// independent of capture — so the same epoch/sequence fact exists with
		// Hindsight both on and off.
		streamRef := futures.streams.Next(feed)

		var captureID hindsight.CaptureIdentity

		if futures.capture != nil {
			var captureErr error

			captureID, captureErr = futures.capture.Capture(
				feed,
				futures.client.URL,
				raw,
				time.Now().UTC(),
				streamRef,
			)

			if captureErr != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("futures: capture failed for %s frame: %s", feed, captureErr.Error()),
					captureErr,
				))

				futures.status.Transition(runtime.ERROR)
				return
			}
		}

		// An unsubscribe acknowledgement answers a teardown; nothing to dispatch.
		if feed == "unsubscribed" || feed == "unsubscribe" {
			return
		}

		handler, ok := futuresMap[feed]

		if !ok {
			futures.err = errnie.Error(errnie.Err(
				errnie.NotFound,
				"futures: unhandled feed "+feed,
				nil,
			))

			futures.status.Transition(runtime.ERROR)
			return
		}

		out := handler(raw)

		if feed == "subscribed" || feed == "error" || feed == "alert" {
			errMessage := utils.GetString(raw, "message")

			if errMessage != "" {
				// A rejected subscription is a per-request acknowledgement, not
				// a transport failure, exactly as on the spot session. It must
				// not poison the session lifecycle status.
				futures.err = errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[futures] subscription rejected: %s", errMessage),
					nil,
				))

				return
			}
		}

		// Dispatch one-shot callbacks.
		if cb, ok := futures.callbacks.LoadAndDelete(feed); ok {
			if msgChan, ok := cb.(chan any); ok {
				msgChan <- out
			}
		}

		switch feed {
		case "ticker", "ticker_lite", "trade", "trade_snapshot":
			// This session does not feed the pipeline until the subscription
			// authority has marked it READY, which happens once the WHOLE
			// universe is subscribed. The frame is still captured above, so
			// Hindsight sees the complete stream.
			if futures.Status() != runtime.READY {
				return
			}

			// Futures frames identify themselves by product_id, where every
			// stage downstream keys on the spot symbol, so the frame is
			// attributed before it becomes envelopes.
			if !futures.attribute(out) {
				return
			}

			envelopes, manifests := IngestEnvelopes(
				"futures."+futuresIngressKey(feed), out, captureID,
			)

			for index, envelope := range envelopes {
				// Live trading reads the operational StreamRef; Hindsight's
				// CaptureID records the same fact but is never the source.
				envelope.Stream = streamRef

				workload, mounted := futures.ingress[futuresIngressKey(feed)]

				if !mounted {
					continue
				}

				workload.Push(envelope)

				if futures.manifestSink != nil {
					_ = futures.manifestSink.WriteManifest(manifests[index])
				}
			}
		}
	})

	futures.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		errnie.Info(fmt.Sprintf("futures: connected to %s", futures.client.URL))

		futures.pinger.Start(futures.ctx)

		// Kraken Futures sends nothing at all on an idle socket — unlike spot,
		// which generates heartbeats automatically on any subscription. Its
		// heartbeat is an explicitly subscribable feed, so a quiet session that
		// wants to hear from the venue must ask for it. Subscribed on every
		// connect, since a reconnect starts a new venue session.
		if err := futures.Write(kraken.NewFuturesSubscription("heartbeat", nil)); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"futures: failed to subscribe to heartbeat",
				err,
			))
		}

		count := futures.connectedCount.Add(1)

		// A second (or later) connect in this process lifetime is a reconnect:
		// the same subscription universe must be soft-rebooted through the one
		// subscription authority and the operational stream epochs advanced,
		// rather than a second subscription path being invented here.
		if count > 1 && futures.reconnect != nil {
			futures.streams.Reconnected()
			futures.reconnect()
		}

		// Connected is not ready, for the same reason as the spot session: the
		// universe is subscribed in paced batches after the socket comes up.
		futures.status.Transition(runtime.BUSY)
	})

	futures.client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		if gorillawebsocket.IsCloseError(
			event.Data,
			gorillawebsocket.CloseNormalClosure,
		) {
			return
		}

		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			fmt.Sprintf("futures %s disconnected: %s - %s", endpoint, event.Data.Error(), event.Data),
			event.Data,
		))

		futures.status.Transition(runtime.WAITING)

		futures.recovery.Dropped(endpoint)
	})

	errnie.Info(fmt.Sprintf("futures: connecting to %s", futures.client.URL))
	futures.status.Transition(runtime.WAITING)

	if err := futures.client.Connect(); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"futures: failed to connect",
			err,
		))
	}

	return futures
}

/*
futuresIngressKey maps a futures feed onto the ingress workload that carries it.
The venue emits a snapshot feed and an incremental feed for the same stream, and
both belong on the same workload.
*/
func futuresIngressKey(feed string) string {
	switch feed {
	case "ticker", "ticker_lite":
		return "ticker"
	case "trade", "trade_snapshot":
		return "trade"
	default:
		return feed
	}
}

func (futures *FuturesLive) Name() string { return "kraken_futures" }

func (futures *FuturesLive) Error() error {
	return futures.err
}

func (futures *FuturesLive) Status() runtime.Stage {
	return futures.status.Current()
}

/*
MarkReady is called by the subscription authority once the WHOLE market universe
is subscribed, mirroring Live.MarkReady.
*/
func (futures *FuturesLive) MarkReady() {
	if futures == nil {
		return
	}

	futures.status.Transition(runtime.READY)
}

func (futures *FuturesLive) Client() *derivatives.WebSocket {
	if futures == nil {
		return nil
	}

	return futures.client
}

/*
SetObserver installs the latency observer this session reports transport timings
to, mirroring the spot session's observer seam.
*/
func (futures *FuturesLive) SetObserver(observer func(string, time.Duration)) {
	if futures == nil {
		return
	}

	futures.observer.Store(&observer)
}

func (futures *FuturesLive) SubFuturesTicker(productIDs []string) error {
	futures.symbols = append(futures.symbols, productIDs...)
	return errnie.Error(futures.client.SubTicker(productIDs...))
}

func (futures *FuturesLive) SubFuturesTrades(productIDs []string) error {
	return errnie.Error(futures.client.SubTrade(productIDs...))
}

func (futures *FuturesLive) SubFuturesBook(productIDs []string) error {
	return errnie.Error(futures.client.SubBook(productIDs...))
}

/*
UnsubFuturesTicker, UnsubFuturesTrades and UnsubFuturesBook withdraw the futures
feeds for the given product IDs. They mirror the Sub* seam so the instrument —
which owns the market universe — drives teardown through the same batched path
it drives setup, rather than the transport keeping a second copy of the universe.
*/
func (futures *FuturesLive) UnsubFuturesTicker(productIDs []string) error {
	return futures.unsubscribe("ticker", productIDs)
}

func (futures *FuturesLive) UnsubFuturesTrades(productIDs []string) error {
	return futures.unsubscribe("trade", productIDs)
}

func (futures *FuturesLive) UnsubFuturesBook(productIDs []string) error {
	return futures.unsubscribe("book", productIDs)
}

/*
unsubscribe writes the batched unsubscribe requests for one feed. A session
whose socket is already gone has nothing to withdraw, which is not an error:
the venue drops the subscriptions along with the connection.
*/
func (futures *FuturesLive) unsubscribe(feed string, productIDs []string) error {
	if futures == nil || futures.client == nil || len(productIDs) == 0 {
		return nil
	}

	if !futures.client.IsActive() {
		return nil
	}

	for group := range slices.Chunk(productIDs, 100) {
		if err := futures.Write(kraken.NewFuturesUnsubscription(feed, group)); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("futures: unsubscribe %s failed", feed),
				err,
			))
		}
	}

	return nil
}

func (futures *FuturesLive) Write(params json.Marshaler, callbacks ...Callback[any]) error {
	for _, callback := range callbacks {
		futures.callbacks.Store(callback.Channel, callback.Message)
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"futures: write marshal failed",
			err,
		))
	}

	started := time.Now()

	err = futures.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if futures.simulator != nil {
		futures.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(err)
}

/*
reopen tears down a socket the session can no longer write to and dials again.

Disconnect is what makes the SDK's blocked read loop return, which raises
OnDisconnected and clears IsActive; Connect then re-arms the SDK's own reconnect
handling for the new socket. Both are needed: Disconnect alone leaves the
session down permanently, because it also disables the SDK's auto-reconnect.
*/
func (futures *FuturesLive) reopen() {
	select {
	case <-futures.ctx.Done():
		return
	default:
	}

	errnie.Error(futures.client.Disconnect())

	if err := futures.client.Connect(); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"futures: failed to reopen after keepalive failure",
			err,
		))
	}
}

func (futures *FuturesLive) Close() error {
	futures.pinger.Stop()
	futures.cancel()

	if futures.client.IsActive() {
		errnie.Error(futures.client.Disconnect())
	}

	return nil
}
