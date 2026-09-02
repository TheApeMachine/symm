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
FuturesLive is one required futures websocket session. Operational disconnects
replace the venue connection and restore its feeds; protocol and ingestion
failures remain terminal and are reported to the process supervisor.
*/
type FuturesLive struct {
	ctx            context.Context
	cancel         context.CancelFunc
	status         *runtime.Status
	err            error
	client         atomic.Pointer[derivatives.WebSocket]
	ingress        map[string]runtime.Ingress[*types.Envelope]
	simulator      *Simulator
	callbacks      *sync.Map
	capture        CaptureSink
	manifestSink   ManifestSink
	subscriptionMu sync.RWMutex
	subscriptions  map[string][]string
	failureMu      sync.RWMutex
	failure        func(error)
	observer       atomic.Pointer[func(string, time.Duration)]
	connected      atomic.Bool
	released       atomic.Bool
	reconnecting   atomic.Bool
	closing        atomic.Bool
	closeOnce      sync.Once

	// streams owns this session's operational epoch/sequence bookkeeping.
	streams *Streams

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
Error returns the first terminal futures-session failure.
*/
func (futures *FuturesLive) Error() error {
	if futures == nil {
		return nil
	}

	futures.failureMu.RLock()
	defer futures.failureMu.RUnlock()

	return futures.err
}

/*
SetFailure binds this session to its owner. Existing constructor failures are
replayed when API attaches after the futures transport is constructed.
*/
func (futures *FuturesLive) SetFailure(handler func(error)) {
	if futures == nil {
		return
	}

	futures.failureMu.Lock()
	futures.failure = handler
	err := futures.err
	futures.failureMu.Unlock()

	if err != nil && handler != nil {
		handler(err)
	}
}

func (futures *FuturesLive) fail(err error) {
	if futures == nil || err == nil {
		return
	}

	err = errnie.Error(err)

	futures.failureMu.Lock()

	if futures.err != nil {
		futures.failureMu.Unlock()
		return
	}

	futures.err = err
	handler := futures.failure
	futures.failureMu.Unlock()

	futures.status.Transition(runtime.ERROR)
	futures.cancel()

	if handler != nil {
		handler(err)
	}
}

func (futures *FuturesLive) operationalError() error {
	if err := futures.Error(); err != nil {
		return err
	}

	select {
	case <-futures.ctx.Done():
		if err := futures.Error(); err != nil {
			return err
		}

		return futures.ctx.Err()
	default:
		return nil
	}
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
func (futures *FuturesLive) attribute(parsed any) error {
	resolve := futures.resolve.Load()

	if resolve == nil {
		return errnie.Err(
			errnie.NotFound,
			"futures: product resolver is not installed",
			nil,
		)
	}

	switch record := parsed.(type) {
	case *kraken.FuturesTicker:
		symbol, listed := (*resolve)(record.Data.ProductID)

		if !listed {
			return errnie.Err(
				errnie.NotFound,
				"futures: ticker product is absent from the instrument registry",
				nil,
			)
		}

		record.Data.Symbol = symbol

		return nil
	case *kraken.FuturesTrade:
		for index := range record.Data {
			data := &record.Data[index]
			symbol, listed := (*resolve)(data.ProductID)

			if !listed {
				return errnie.Err(
					errnie.NotFound,
					"futures: trade product is absent from the instrument registry",
					nil,
				)
			}

			data.Symbol = symbol
		}

		return nil
	default:
		return errnie.Err(
			errnie.Validation,
			"futures: unsupported parsed feed type",
			nil,
		)
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

	// The SDK reconnect callback retries forever without observing this
	// session's context or restoring its subscriptions. This session owns that
	// lifecycle through the SDK's Connect method instead.
	client.Reconnect = nil
	client.OnDisconnected.Reset()

	ctx, cancel := context.WithCancel(ctx)

	futures := &FuturesLive{
		ctx:           ctx,
		cancel:        cancel,
		status:        runtime.NewStatus(),
		callbacks:     &sync.Map{},
		ingress:       workloads,
		subscriptions: make(map[string][]string),
		streams:       NewStreams(client.URL),
	}
	futures.client.Store(client)

	futures.pinger = NewPinger("futures", func() error {
		client := futures.Client()

		if !futures.connected.Load() {
			return nil
		}

		err := client.WriteMessage(gorillawebsocket.PingMessage, nil)

		if err != nil && client != futures.Client() {
			return nil
		}

		return err
	})

	futures.pinger.OnFailed(func(err error) {
		futures.fail(errnie.Err(
			errnie.IO,
			"futures: keepalive failed",
			err,
		))
	})

	if len(recorders) == 1 {
		futures.capture = recorders[0]

		if manifestSink, ok := recorders[0].(ManifestSink); ok {
			futures.manifestSink = manifestSink
		}
	}

	client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		if futures.operationalError() != nil {
			return
		}

		raw := event.Data.Bytes()

		// Lifecycle acknowledgements carry both event and feed. Event owns the
		// frame identity when present; otherwise a ticker subscription ack would
		// be parsed and attributed as a ticker observation with no product_id.
		feed := futuresFrameIdentity(raw)

		if feed == "" {
			futures.fail(errnie.Err(
				errnie.Validation,
				"futures: frame has no feed or event identity",
				nil,
			))

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
				futures.Client().URL,
				raw,
				time.Now().UTC(),
				streamRef,
			)

			if captureErr != nil {
				futures.fail(errnie.Err(
					errnie.IO,
					fmt.Sprintf("futures: capture failed for %s frame: %s", feed, captureErr.Error()),
					captureErr,
				))
				return
			}
		}

		// An unsubscribe acknowledgement answers a teardown; nothing to dispatch.
		if feed == "unsubscribed" || feed == "unsubscribe" {
			return
		}

		handler, ok := futuresMap[feed]

		if !ok {
			futures.fail(errnie.Err(
				errnie.NotFound,
				"futures: unhandled feed "+feed,
				nil,
			))
			return
		}

		out := handler(raw)

		if feed == "subscribed" || feed == "error" || feed == "alert" {
			errMessage := utils.GetString(raw, "message")

			if errMessage != "" {
				futures.fail(errnie.Err(
					errnie.IO,
					fmt.Sprintf("futures: subscription rejected: %s", errMessage),
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
			// Connected sessions capture but do not feed the pipeline until the
			// complete consumer graph has crossed its READY boundary.
			if futures.Status() != runtime.READY {
				return
			}

			// Futures frames identify themselves by product_id, where every
			// stage downstream keys on the spot symbol, so the frame is
			// attributed before it becomes envelopes.
			if err := futures.attribute(out); err != nil {
				futures.fail(err)
				return
			}

			envelopes, manifests := IngestEnvelopes(
				"futures."+futuresIngressKey(feed), out, captureID,
			)

			for index, envelope := range envelopes {
				// Live trading reads the operational StreamRef; Hindsight's
				// CaptureID records the same fact but is never the source.
				envelope.Stream = streamRef

				if futures.manifestSink != nil {
					if err := futures.manifestSink.WriteManifest(manifests[index]); err != nil {
						futures.fail(errnie.Err(
							errnie.IO,
							"futures: failed to persist envelope manifest",
							err,
						))

						return
					}
				}

				workload, mounted := futures.ingress[futuresIngressKey(feed)]

				if !mounted || workload == nil {
					futures.fail(errnie.Err(
						errnie.NotFound,
						"futures: required ingress is not mounted for "+feed,
						nil,
					))

					return
				}

				if workload.Status() == nil || workload.Status().Current() != runtime.READY {
					futures.fail(errnie.Err(
						errnie.NotAcceptable,
						"futures: ingress is not ready for "+feed,
						nil,
					))

					return
				}

				workload.Push(envelope)
			}
		}
	})

	client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if futures.operationalError() != nil {
			return
		}

		errnie.Info(fmt.Sprintf("futures: connected to %s", futures.Client().URL))

		futures.connected.Store(true)

		if futures.released.Load() {
			futures.status.Transition(runtime.READY)
		} else {
			futures.status.Transition(runtime.BUSY)
		}

		futures.pinger.Start(futures.ctx)

		// Kraken Futures sends nothing at all on an idle socket — unlike spot,
		// which generates heartbeats automatically on any subscription. Its
		// heartbeat is an explicitly subscribable feed, so a quiet session that
		// wants to hear from the venue must ask for it.
		if err := futures.Write(kraken.NewFuturesSubscription("heartbeat", nil)); err != nil {
			futures.fail(errnie.Err(
				errnie.IO,
				"futures: failed to subscribe to heartbeat",
				err,
			))

			return
		}

		if futures.released.Load() {
			if err := futures.restoreSubscriptions(); err != nil {
				futures.fail(err)
			}
		}
	})

	client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		go futures.reconnect(event.Data)
	})

	errnie.Info(fmt.Sprintf("futures: connecting to %s", client.URL))
	futures.status.Transition(runtime.WAITING)

	if err := client.Connect(); err != nil {
		futures.fail(errnie.Err(
			errnie.IO,
			"futures: failed to connect",
			err,
		))
	}

	return futures
}

/*
reconnect reopens the same futures transport after an operational disconnect.
The retry cadence comes from the official SDK client, while this session owns
context cancellation, subscription restoration, and stream epoch boundaries.
*/
func (futures *FuturesLive) reconnect(err error) {
	if futures.closing.Load() || futures.ctx.Err() != nil {
		return
	}

	if !futures.reconnecting.CompareAndSwap(false, true) {
		return
	}

	defer futures.reconnecting.Store(false)

	futures.connected.Store(false)
	futures.pinger.Stop()
	futures.status.Transition(runtime.WAITING)
	futures.streams.Advance()
	client := futures.Client()
	replacement := derivatives.NewWebSocket()
	replacement.REST = client.REST
	replacement.URL = client.URL
	replacement.Reconnect = nil
	replacement.ReconnectWait = client.ReconnectWait
	replacement.Insecure = client.Insecure
	replacement.AuthenticateTimeout = client.AuthenticateTimeout
	replacement.PublicKey = client.PublicKey
	replacement.PrivateKey = client.PrivateKey
	replacement.Challenge = client.Challenge
	replacement.Signature = client.Signature
	replacement.OnAuthenticated = client.OnAuthenticated
	replacement.OnConnected = client.OnConnected
	replacement.OnDisconnected = client.OnDisconnected
	replacement.OnSent = client.OnSent
	replacement.OnReceived = client.OnReceived
	futures.client.Store(replacement)
	errnie.Error(errnie.Err(
		errnie.IO,
		fmt.Sprintf("futures %s disconnected; reconnecting", replacement.URL),
		err,
	))

	for futures.ctx.Err() == nil {
		err = replacement.Connect()

		if err == nil {
			if futures.ctx.Err() != nil {
				_ = replacement.Disconnect()
			}

			return
		}

		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("futures %s reconnect failed", replacement.URL),
			err,
		))

		retry := time.NewTimer(replacement.ReconnectWait)

		select {
		case <-retry.C:
		case <-futures.ctx.Done():
			retry.Stop()

			return
		}
	}
}

/*
restoreSubscriptions replays the feeds accepted by the previous venue session.
It runs only after a replacement connection is established and consumers have
already crossed their readiness boundary.
*/
func (futures *FuturesLive) restoreSubscriptions() error {
	futures.subscriptionMu.RLock()
	defer futures.subscriptionMu.RUnlock()
	client := futures.Client()

	for feed, productIDs := range futures.subscriptions {
		var err error

		switch feed {
		case "ticker":
			err = client.SubTicker(productIDs...)
		case "trade":
			err = client.SubTrade(productIDs...)
		case "book":
			err = client.SubBook(productIDs...)
		}

		if err != nil {
			return errnie.Err(
				errnie.IO,
				"futures: failed to restore "+feed+" subscription",
				err,
			)
		}
	}

	return nil
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

func futuresFrameIdentity(raw []byte) string {
	if event := utils.GetString(raw, "event"); event != "" {
		return event
	}

	return utils.GetString(raw, "feed")
}

func (futures *FuturesLive) Name() string { return "kraken_futures" }

func (futures *FuturesLive) Status() runtime.Stage {
	return futures.status.Current()
}

/*
MarkReady releases this connected session after the complete consumer graph has
been admitted, mirroring Live.MarkReady.
*/
func (futures *FuturesLive) MarkReady() {
	if futures == nil {
		return
	}

	if futures.operationalError() != nil {
		return
	}

	if futures.Status() != runtime.BUSY && futures.Status() != runtime.READY {
		futures.fail(errnie.Err(
			errnie.NotAcceptable,
			"futures: only a connected session can become ready",
			nil,
		))

		return
	}

	if len(futures.ingress) == 0 {
		futures.fail(errnie.Err(
			errnie.NotFound,
			"futures: readiness requires mounted ingress workloads",
			nil,
		))

		return
	}

	for feed, workload := range futures.ingress {
		if workload != nil && workload.Status() != nil &&
			workload.Status().Current() == runtime.READY {
			continue
		}

		futures.fail(errnie.Err(
			errnie.NotAcceptable,
			"futures: cannot become ready before ingress "+feed,
			nil,
		))

		return
	}

	futures.released.Store(true)
	futures.status.Transition(runtime.READY)
}

func (futures *FuturesLive) Client() *derivatives.WebSocket {
	if futures == nil {
		return nil
	}

	return futures.client.Load()
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
	if err := futures.operationalError(); err != nil {
		return err
	}

	if futures.Status() != runtime.READY {
		err := errnie.Err(
			errnie.NotAcceptable,
			"futures: ticker subscription requires a ready session",
			nil,
		)
		futures.fail(err)

		return err
	}

	if err := futures.Client().SubTicker(productIDs...); err != nil {
		err = errnie.Err(
			errnie.IO,
			"futures: failed to subscribe to ticker",
			err,
		)
		futures.fail(err)

		return err
	}

	futures.subscriptionMu.Lock()
	futures.subscriptions["ticker"] = append(futures.subscriptions["ticker"], productIDs...)
	futures.subscriptionMu.Unlock()

	return nil
}

func (futures *FuturesLive) SubFuturesTrades(productIDs []string) error {
	if err := futures.operationalError(); err != nil {
		return err
	}

	if futures.Status() != runtime.READY {
		err := errnie.Err(
			errnie.NotAcceptable,
			"futures: trade subscription requires a ready session",
			nil,
		)
		futures.fail(err)

		return err
	}

	if err := futures.Client().SubTrade(productIDs...); err != nil {
		err = errnie.Err(
			errnie.IO,
			"futures: failed to subscribe to trades",
			err,
		)
		futures.fail(err)

		return err
	}

	futures.subscriptionMu.Lock()
	futures.subscriptions["trade"] = append(futures.subscriptions["trade"], productIDs...)
	futures.subscriptionMu.Unlock()

	return nil
}

func (futures *FuturesLive) SubFuturesBook(productIDs []string) error {
	if err := futures.operationalError(); err != nil {
		return err
	}

	if futures.Status() != runtime.READY {
		err := errnie.Err(
			errnie.NotAcceptable,
			"futures: book subscription requires a ready session",
			nil,
		)
		futures.fail(err)

		return err
	}

	if err := futures.Client().SubBook(productIDs...); err != nil {
		err = errnie.Err(
			errnie.IO,
			"futures: failed to subscribe to book",
			err,
		)
		futures.fail(err)

		return err
	}

	futures.subscriptionMu.Lock()
	futures.subscriptions["book"] = append(futures.subscriptions["book"], productIDs...)
	futures.subscriptionMu.Unlock()

	return nil
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
	if futures == nil || futures.Client() == nil || len(productIDs) == 0 {
		return nil
	}

	if !futures.connected.Load() {
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

	futures.subscriptionMu.Lock()
	futures.subscriptions[feed] = slices.DeleteFunc(
		futures.subscriptions[feed],
		func(productID string) bool {
			return slices.Contains(productIDs, productID)
		},
	)
	futures.subscriptionMu.Unlock()

	return nil
}

func (futures *FuturesLive) Write(params json.Marshaler, callbacks ...Callback[any]) error {
	if err := futures.operationalError(); err != nil {
		return err
	}

	for _, callback := range callbacks {
		futures.callbacks.Store(callback.Channel, callback.Message)
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		err = errnie.Err(
			errnie.Validation,
			"futures: write marshal failed",
			err,
		)
		futures.fail(err)

		return err
	}

	started := time.Now()

	err = futures.Client().WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if futures.simulator != nil {
		futures.simulator.Record(WEBSOCKET, time.Since(started))
	}

	if err != nil {
		err = errnie.Err(
			errnie.IO,
			"futures: write failed",
			err,
		)
		futures.fail(err)
	}

	return err
}

func (futures *FuturesLive) Close() error {
	if futures == nil {
		return nil
	}

	var closeErr error

	futures.closeOnce.Do(func() {
		futures.closing.Store(true)
		futures.cancel()

		if futures.pinger != nil {
			futures.pinger.Stop()
		}

		if futures.connected.Load() && futures.Error() == nil && futures.Client() != nil {
			if err := futures.Client().Disconnect(); err != nil {
				closeErr = errnie.Err(
					errnie.IO,
					"futures: failed to disconnect",
					err,
				)
				futures.fail(closeErr)
			}
		}

		if futures.Error() == nil && futures.status != nil {
			futures.status.Transition(runtime.DONE)
		}
	})

	return closeErr
}
