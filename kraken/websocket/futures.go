package websocket

import (
	"context"
	"errors"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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
	status        atomic.Pointer[types.Status]
	ctx           context.Context
	cancel        context.CancelFunc
	endpoint      string
	thesis        atomic.Pointer[types.Thesis]
	bus           atomic.Pointer[runtime.Workspace]
	capture       CaptureSink
	conn          *gorillawebsocket.Conn
	connMu        sync.Mutex
	subsMu        sync.RWMutex
	subscriptions map[string]map[string]struct{}
	failureMu     sync.RWMutex
	failure       func(error)
	observer      atomic.Pointer[func(string, time.Duration)]
}

/*
NewFutures constructs a new Kraken Futures WebSocket connection.
*/
func NewFutures(
	ctx context.Context,
	bus *runtime.Workspace,
	endpoint string,
) *FuturesLive {
	if endpoint == "" {
		endpoint = FuturesWebSocketURL
	}

	childCtx, cancel := context.WithCancel(ctx)

	futures := &FuturesLive{
		ctx:           childCtx,
		cancel:        cancel,
		endpoint:      endpoint,
		subscriptions: make(map[string]map[string]struct{}),
	}

	initialStatus := types.INITIALIZING
	futures.status.Store(&initialStatus)

	futures.SetBus(bus)

	if bus == nil {
		return futures
	}

	if shared, _ := bus.Shared("thesis", ""); shared != nil {
		if thesis, ok := shared.(*types.Thesis); ok {
			futures.thesis.Store(thesis)
		}
	}

	if shared, _ := bus.Shared("recorder", ""); shared != nil {
		if capture, ok := shared.(CaptureSink); ok {
			futures.capture = capture
		}
	}

	return futures
}

func (futures *FuturesLive) Name() string { return "kraken_futures" }

func (futures *FuturesLive) Error() error {
	futures.failureMu.RLock()
	defer futures.failureMu.RUnlock()
	return nil
}

func (futures *FuturesLive) Status() types.Status {
	st := futures.status.Load()

	if st == nil {
		return types.PENDING
	}

	return *st
}

func (futures *FuturesLive) SetBus(bus *runtime.Workspace) {
	if futures == nil || bus == nil {
		return
	}

	futures.bus.Store(bus)
}

func (futures *FuturesLive) SetThesis(thesis *types.Thesis) {
	if futures == nil || thesis == nil {
		return
	}

	futures.thesis.Store(thesis)
}

func (futures *FuturesLive) SetFailureHandler(handler func(error)) {
	if futures == nil {
		return
	}

	futures.failureMu.Lock()
	futures.failure = handler
	futures.failureMu.Unlock()
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

	readyStatus := types.READY
	futures.status.Store(&readyStatus)

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

	if futures.capture != nil {
		_ = futures.capture.Capture("futures", raw, time.Now().UTC())
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
	ticker := kraken.NewFuturesTicker(raw)

	if ticker == nil || ticker.Data.ProductID == "" {
		return
	}

	thesis := futures.thesis.Load()

	if thesis == nil {
		return
	}

	spotSymbol := kraken.FuturesProductIDToSpot(ticker.Data.ProductID)

	if spotSymbol == "" {
		return
	}

	ticker.Data.Symbol = spotSymbol

	if thesis.Symbol(spotSymbol).AcceptFuturesTicker(ticker.Data.Timestamp) {
		if bus := futures.bus.Load(); bus != nil {
			bus.Publish(types.ChannelFuturesTickers, ticker.Data)
		}
	}
}

func (futures *FuturesLive) dispatchTrades(raw []byte) {
	trades := kraken.NewFuturesTrade(raw)

	if trades == nil || len(trades.Data) == 0 {
		return
	}

	thesis := futures.thesis.Load()

	if thesis == nil {
		return
	}

	for index := range trades.Data {
		trade := trades.Data[index]
		spotSymbol := kraken.FuturesProductIDToSpot(trade.ProductID)

		if spotSymbol == "" {
			continue
		}

		trade.Symbol = spotSymbol

		if thesis.Symbol(spotSymbol).AcceptFuturesTrade(trade.Timestamp) {
			if bus := futures.bus.Load(); bus != nil {
				bus.Publish(types.ChannelFuturesTrades, trade)
			}
		}
	}
}

func (futures *FuturesLive) dispatchBook(raw []byte) {
	book := kraken.NewFuturesBook(raw)

	if book == nil || book.Data.ProductID == "" {
		return
	}

	thesis := futures.thesis.Load()

	if thesis == nil {
		return
	}

	spotSymbol := kraken.FuturesProductIDToSpot(book.Data.ProductID)

	if spotSymbol == "" {
		return
	}

	book.Data.Symbol = spotSymbol

	if thesis.Symbol(spotSymbol).AcceptFuturesBook(book.Data.Timestamp) {
		if bus := futures.bus.Load(); bus != nil {
			bus.Publish(types.ChannelFuturesBooks, book.Data)
		}
	}
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
