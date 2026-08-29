package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
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
}

/*
NewFutures constructs a new Kraken Futures WebSocket connection. ingress is
keyed by feed name ("ticker"/"trade"), mirroring Live's ingress map, and is
where DispatchFrame pushes the envelope it builds from each parsed frame.
*/
func NewFutures(
	ctx context.Context,
	endpoint string,
	ingress map[string]*runtime.Workload[*types.Envelope],
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

	return futures
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

		futures.DispatchFrame(payload)
	}
}

/*
DispatchFrame decodes a raw wire payload from Kraken Futures and routes it.
*/
func (futures *FuturesLive) DispatchFrame(raw []byte) {
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
		futures.dispatchTicker(raw)
	case "trade", "trade_snapshot":
		futures.dispatchTrades(raw)
	case "book", "book_snapshot":
		futures.dispatchBook(raw)
	}
}

func (futures *FuturesLive) dispatchTicker(raw []byte) {
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
	workload.Push(envelope)
}

func (futures *FuturesLive) dispatchTrades(raw []byte) {
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
		workload.Push(envelope)
	}
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
