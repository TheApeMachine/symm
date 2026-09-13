package websocket

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/utils"
	"golang.design/x/lockfree/lf"
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
	*runtime.System
	client         atomic.Pointer[derivatives.WebSocket]
	queue          *lf.Queue[map[string]any]
	simulator      *Simulator
	callbacks      *sync.Map
	subscriptionMu sync.RWMutex
	subscriptions  map[string][]string

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

func (futures *FuturesLive) fail(err error) {
	if futures == nil || err == nil {
		return
	}

	futures.Fail(err)
}

func (futures *FuturesLive) operationalError() error {
	if err := futures.Error(); err != nil {
		return err
	}

	select {
	case <-futures.Context().Done():
		if err := futures.Error(); err != nil {
			return err
		}

		return futures.Context().Err()
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
) *FuturesLive {
	return NewFuturesWithClient(ctx, endpoint, nil)
}

/*
NewFuturesWithClient opens a futures websocket session using an injected
derivatives.WebSocket client instance, mirroring NewWithClient.
*/
func NewFuturesWithClient(
	ctx context.Context,
	endpoint string,
	client *derivatives.WebSocket,
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

	futures := &FuturesLive{
		System:        runtime.NewSystem(ctx, "websocket:futures"),
		callbacks:     &sync.Map{},
		queue:         lf.NewQueue[map[string]any](),
		subscriptions: make(map[string][]string),
		streams:       NewStreams(client.URL),
	}
	futures.client.Store(client)

	futures.pinger = NewPinger("futures", func() error {
		client := futures.Client()

		if futures.Status() != runtime.READY && futures.Status() != runtime.BUSY {
			return nil
		}

		err := client.WriteMessage(gorillawebsocket.PingMessage, nil)

		if err != nil && client != futures.Client() {
			return nil
		}

		return err
	})

	futures.pinger.OnFailed(func(err error) {
		go futures.reconnect(errnie.Err(
			errnie.IO,
			"futures: keepalive failed",
			err,
		))
	})

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

			// One queue row per venue record; conversion to measurements
			// happens in Step, exactly like the spot transport.
			frame, err := event.Data.Map()

			if err != nil {
				futures.fail(errnie.Err(
					errnie.Validation,
					"futures: failed to map "+feed+" frame",
					err,
				))

				return
			}

			rows, rowsOk := frame["data"].([]any)

			if !rowsOk {
				return
			}

			channel := "futures." + futuresKey(feed)

			for _, entry := range rows {
				row, rowOk := entry.(map[string]any)

				if !rowOk {
					continue
				}

				row["channel"] = channel
				futures.queue.Enqueue(row)
			}
		}
	})

	client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if futures.operationalError() != nil {
			return
		}

		errnie.Info(fmt.Sprintf("futures: connected to %s", futures.Client().URL))

		if futures.Status() == runtime.READY {
			futures.Transition(runtime.READY)
		} else {
			futures.Transition(runtime.BUSY)
		}

		futures.pinger.Start(futures.Context())

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

		if futures.Status() == runtime.READY {
			if err := futures.restoreSubscriptions(); err != nil {
				futures.fail(err)
			}
		}
	})

	client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		go futures.reconnect(event.Data)
	})

	errnie.Info(fmt.Sprintf("futures: connecting to %s", client.URL))
	futures.Transition(runtime.WAITING)

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
	if futures.Status() == runtime.DONE || futures.Context().Err() != nil {
		return
	}

	futures.pinger.Stop()
	futures.Transition(runtime.WAITING)
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

	for futures.Context().Err() == nil {
		err = replacement.Connect()

		if err == nil {
			if futures.Context().Err() != nil {
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
		case <-futures.Context().Done():
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
futuresKey maps a futures feed onto the ingress workload that carries it.
The venue emits a snapshot feed and an incremental feed for the same stream, and
both belong on the same workload.
*/
func futuresKey(feed string) string {
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

func (futures *FuturesLive) Client() *derivatives.WebSocket {
	if futures == nil {
		return nil
	}

	return futures.client.Load()
}

func (futures *FuturesLive) MarkReady() {
	if futures == nil || futures.operationalError() != nil {
		return
	}

	futures.Transition(runtime.READY)

	if err := futures.restoreSubscriptions(); err != nil {
		futures.fail(err)
	}
}

func (futures *FuturesLive) SubFuturesTicker(productIDs []string) error {
	if err := futures.operationalError(); err != nil {
		return err
	}

	if futures.Status() != runtime.BUSY && futures.Status() != runtime.READY {
		err := errnie.Err(
			errnie.NotAcceptable,
			"futures: ticker subscription requires a connected session",
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

	if futures.Status() != runtime.BUSY && futures.Status() != runtime.READY {
		err := errnie.Err(
			errnie.NotAcceptable,
			"futures: trade subscription requires a connected session",
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

	if futures.Status() != runtime.BUSY && futures.Status() != runtime.READY {
		err := errnie.Err(
			errnie.NotAcceptable,
			"futures: book subscription requires a connected session",
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

	if futures.Status() != runtime.READY && futures.Status() != runtime.BUSY {
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

	if futures.Status() == runtime.DONE {
		return nil
	}

	futures.Transition(runtime.DONE)

	if futures.pinger != nil {
		futures.pinger.Stop()
	}

	var closeErr error

	if client := futures.Client(); client != nil {
		if err := client.Disconnect(); err != nil {
			closeErr = errnie.Err(
				errnie.IO,
				"futures: failed to disconnect",
				err,
			)
			futures.fail(closeErr)
		}
	}

	return errors.Join(closeErr, futures.System.Close())
}
