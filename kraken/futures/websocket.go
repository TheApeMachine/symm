package futures

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/wsutil"
	"github.com/theapemachine/symm/observability"
	"github.com/theapemachine/symm/rawbus"
)

const connectMaxAttempts = 12

/*
WebSocket streams Kraken Futures public book feeds into raw book batches.
*/
type WebSocket struct {
	ctx              context.Context
	cancel           context.CancelFunc
	errMu            atomic.Pointer[error]
	pool             *qpool.Q[any]
	conn             *websocket.Conn
	bus              *internal.Bus
	registry         *BookRegistry
	isConnected      atomic.Bool
	needsResubscribe atomic.Bool
	futuresEnabled   bool
	wsPingInterval   time.Duration
	readStarted      atomic.Bool
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	wsCtx, cancel := context.WithCancel(ctx)

	marketConfig, marketErr := config.LoadMarketConfig()

	if marketErr != nil {
		cancel()
		errnie.Error(marketErr)
		return nil
	}

	wsPingInterval := time.Second

	if marketConfig.WSPingInterval > 0 {
		wsPingInterval = marketConfig.WSPingInterval
	}

	futuresSocket := &WebSocket{
		ctx:            wsCtx,
		cancel:         cancel,
		pool:           pool,
		futuresEnabled: marketConfig.FuturesEnabled,
		wsPingInterval: wsPingInterval,
		registry:       NewBookRegistry(),
		bus: internal.NewBus(
			wsCtx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelKrakenFutures},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "kraken:futures:raw"),
				internal.Subscribe(internal.ChannelKrakenFutures, "kraken:futures:bus"),
			},
		),
	}

	errnie.Info("kraken/futures websocket ready")

	return futuresSocket
}

func (ws *WebSocket) setErr(err error) {
	if err == nil {
		ws.errMu.Store(nil)
		return
	}

	stored := err
	ws.errMu.Store(&stored)
}

func (ws *WebSocket) getErr() error {
	err := ws.errMu.Load()

	if err == nil {
		return nil
	}

	return *err
}

func (ws *WebSocket) Connect(attempt uint64) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	connectCtx := wsutil.NonNilContext(ws.ctx)
	backoff := wsutil.NewBackoffFromConfig()
	var lastErr error

	for connectAttempt := attempt; connectAttempt < attempt+connectMaxAttempts; connectAttempt++ {
		if connectCtx.Err() != nil {
			ws.setErr(connectCtx.Err())
			return connectCtx.Err()
		}

		ws.isConnected.Store(false)

		var response *http.Response

		ws.conn, response, lastErr = websocket.DefaultDialer.Dial(string(WebSocketURL), nil)

		if lastErr == nil {
			ws.isConnected.Store(true)
			ws.setErr(nil)
			observability.Shared().RecordWebSocketConnected(
				"kraken/futures",
				string(WebSocketURL),
				time.Now().UTC(),
			)

			return nil
		}

		ws.setErr(lastErr)
		observability.Shared().RecordWebSocketReconnect(
			"kraken/futures",
			string(WebSocketURL),
			lastErr.Error(),
			time.Now().UTC(),
		)

		if response != nil {
			errnie.Error(lastErr, response.StatusCode, response.Status)
		}

		if waitErr := backoff.Wait(connectCtx, connectAttempt); waitErr != nil {
			ws.setErr(waitErr)
			return waitErr
		}
	}

	return lastErr
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)
	ws.needsResubscribe.Store(true)

	if ws.conn == nil {
		return
	}

	errnie.Error(ws.conn.Close())
	ws.conn = nil
}

func (ws *WebSocket) resubscribe() error {
	return rawbus.Send(ws.bus, rawbus.TypeReconnect, struct{}{})
}

func (ws *WebSocket) Tick() error {
	if !ws.futuresEnabled {
		<-ws.ctx.Done()

		return ws.ctx.Err()
	}

	if ws.readStarted.CompareAndSwap(false, true) {
		go ws.readLoop()
	}

	ticker := time.NewTicker(ws.wsPingInterval)
	defer ticker.Stop()

	for {
		for !ws.isConnected.Load() || ws.conn == nil {
			if connectErr := errnie.Error(ws.Connect(0)); connectErr != nil {
				ws.setErr(connectErr)
				continue
			}

			if !ws.needsResubscribe.Load() {
				continue
			}

			if resubErr := errnie.Error(ws.resubscribe()); resubErr != nil {
				ws.setErr(resubErr)
				ws.disconnect()
				continue
			}

			ws.needsResubscribe.Store(false)
		}

		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case <-ticker.C:
			if ws.conn == nil {
				ws.disconnect()
				continue
			}

			if errnie.Error(ws.conn.WriteJSON(PingMessage{Event: "ping"})) != nil {
				ws.disconnect()
				continue
			}
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			continue
		}

		_, payload, readErr := ws.conn.ReadMessage()

		if readErr != nil {
			ws.setErr(readErr)
			ws.disconnect()
			continue
		}

		ws.dispatch(payload)
	}
}

func (ws *WebSocket) readLoop() {
	for {
		message, receiveErr := ws.bus.Receive(internal.ChannelKrakenFutures)

		if errnie.Error(receiveErr) != nil || message == nil {
			break
		}

		for ws.conn == nil || !ws.isConnected.Load() {
			if ws.ctx.Err() != nil {
				return
			}

			time.Sleep(10 * time.Millisecond)
		}

		if errnie.Error(ws.conn.WriteJSON(message.Value)) != nil {
			ws.disconnect()
			continue
		}
	}
}

func (ws *WebSocket) dispatch(payload []byte) {
	frame, err := decodeWireFrame(payload)

	if errnie.Error(err) != nil {
		return
	}

	if frame.Event == "error" {
		errnie.Error(fmt.Errorf("kraken/futures: %s", frame.Message))

		return
	}

	if frame.Event == "subscribed" || frame.Event == "unsubscribed" {
		return
	}

	switch frame.Feed {
	case FeedBookSnapshot:
		snapshot, snapshotErr := parseBookSnapshot(payload)

		if errnie.Error(snapshotErr) != nil {
			return
		}

		update := ws.registry.ApplySnapshot(snapshot)

		if update == nil {
			return
		}

		if sendErr := rawbus.Send(ws.bus, rawbus.TypeBook, &krakenmarket.BookUpdates{update}); sendErr != nil {
			errnie.Error(fmt.Errorf("kraken/futures: publish book snapshot for %q: %w", snapshot.ProductID, sendErr))
		}
	case FeedBookDelta:
		delta, deltaErr := parseBookDelta(payload)

		if errnie.Error(deltaErr) != nil {
			return
		}

		update, ok := ws.registry.ApplyDelta(delta)

		if !ok || update == nil {
			return
		}

		if sendErr := rawbus.Send(ws.bus, rawbus.TypeBook, &krakenmarket.BookUpdates{update}); sendErr != nil {
			errnie.Error(fmt.Errorf("kraken/futures: publish book delta for %q: %w", delta.ProductID, sendErr))
		}
	default:
		return
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	if ws.conn != nil {
		errnie.Error(ws.conn.Close())
	}

	return nil
}
