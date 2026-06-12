package futures

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/rawbus"
)

var futuresSocket *WebSocket
var futuresSocketOnce sync.Once

/*
WebSocket streams Kraken Futures public book feeds into raw book batches.
*/
type WebSocket struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	pool             *qpool.Q[any]
	conn             *websocket.Conn
	bus              *internal.Bus
	registry         *BookRegistry
	isConnected      atomic.Bool
	needsResubscribe atomic.Bool
	futuresEnabled   bool
	wsPingInterval   time.Duration
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	futuresSocketOnce.Do(func() {
		marketConfig, _ := config.LoadMarketConfig()
		wsPingInterval := time.Second

		if marketConfig.WSPingInterval > 0 {
			wsPingInterval = marketConfig.WSPingInterval
		}

		futuresSocket = &WebSocket{
			ctx:            ctx,
			cancel:         cancel,
			pool:           pool,
			futuresEnabled: marketConfig.FuturesEnabled,
			wsPingInterval: wsPingInterval,
			registry:       NewBookRegistry(),
			bus: internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelRaw, internal.ChannelKrakenFutures},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelRaw, "kraken:futures:raw"),
					internal.Subscribe(internal.ChannelKrakenFutures, "kraken:futures:bus"),
				},
			),
		}

		errnie.Info("kraken/futures websocket ready")
	})

	return futuresSocket
}

func (ws *WebSocket) Connect(attempt uint64) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	ws.isConnected.Store(false)

	var response *http.Response

	ws.conn, response, ws.err = websocket.DefaultDialer.Dial(string(WebSocketURL), nil)

	if ws.err != nil {
		if response != nil {
			errnie.Error(ws.err, response.StatusCode, response.Status)
		}

		backoff := uint64(
			math.Round((math.Pow(
				math.Phi, float64(attempt),
			) + math.Pow(
				math.Phi-1, float64(attempt),
			)) / math.Sqrt(5)),
		)

		time.Sleep(time.Duration(backoff) * time.Second)

		return ws.Connect(attempt + 1)
	}

	ws.isConnected.Store(true)

	return nil
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

	ws.read()

	ticker := time.NewTicker(ws.wsPingInterval)
	defer ticker.Stop()

	for {
		for !ws.isConnected.Load() || ws.conn == nil {
			if ws.err = errnie.Error(ws.Connect(0)); ws.err != nil {
				continue
			}

			if !ws.needsResubscribe.Load() {
				continue
			}

			if ws.err = errnie.Error(ws.resubscribe()); ws.err != nil {
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
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			continue
		}

		_, payload, readErr := ws.conn.ReadMessage()

		if readErr != nil {
			ws.err = readErr
			ws.disconnect()
			continue
		}

		ws.dispatch(payload)
	}
}

func (ws *WebSocket) read() {
	go func() {
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
	}()
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

		rawbus.Send(ws.bus, rawbus.TypeBook, &krakenmarket.BookUpdates{update})
	case FeedBookDelta:
		delta, deltaErr := parseBookDelta(payload)

		if errnie.Error(deltaErr) != nil {
			return
		}

		update, ok := ws.registry.ApplyDelta(delta)

		if !ok || update == nil {
			return
		}

		rawbus.Send(ws.bus, rawbus.TypeBook, &krakenmarket.BookUpdates{update})
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
