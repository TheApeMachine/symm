package public

import (
	"container/ring"
	"context"
	"encoding/json"
	"io"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/focus"
)

var socket *WebSocket
var socketOnce sync.Once
var outboundOnce sync.Once

const publicOutboundSubscriberID = "kraken:public:websocket"

const (
	publicRawSubscriberID    = "kraken/public:raw"
	publicLevel3SubscriberID = "kraken/public:level3"
)

type WebSocketClient interface {
	Connect(EndpointType, string, uint64) error
	Tick() error
	Close() error
}

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	conn        *websocket.Conn
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	recorder    io.Writer
	streams     *focus.Set
	latencies   *ring.Ring
}

func NewWebSocket(ctx context.Context, pool *qpool.Q, streams *focus.Set) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:         ctx,
			cancel:      cancel,
			pool:        pool,
			broadcasts:  make(map[string]*qpool.BroadcastGroup),
			subscribers: make(map[string]*qpool.Subscriber),
			latencies:   ring.New(64),
		}

		for _, channel := range []string{"raw", "level3", "ui", "kraken:public"} {
			socket.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
			socket.subscribers[channel] = socket.broadcasts[channel].Subscribe(channel, 1024)
		}

		socket.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel":  "instrument",
				"snapshot": true,
			},
		}})

		activate.Boot("kraken/public websocket ready")
	})

	return socket
}

func (ws *WebSocket) Connect(
	endpoint EndpointType, channel string, n uint64,
) error {
	// Error, retry.
	ws.conn, _, ws.err = websocket.DefaultDialer.Dial(string(endpoint), nil)

	if ws.err != nil {
		errnie.Error(ws.err)

		// Backoff delay time by using Fibonacci sequence.
		n = uint64(
			math.Round((math.Pow(
				math.Phi, float64(n),
			) + math.Pow(
				math.Phi-1, float64(n),
			)) / math.Sqrt(5)),
		)

		// Wait for the next retry.
		time.Sleep(time.Duration(n) * time.Second)
	}

	return nil
}

var sockMsgPool = sync.Pool{
	New: func() any {
		return make(map[string]any)
	},
}

func (ws *WebSocket) Tick() (err error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case message, ok := <-ws.subscribers["kraken:public"].Incoming:
			if !ok || message == nil {
				return ws.ctx.Err()
			}
		case <-ticker.C:
			if err = ws.conn.WriteJSON(map[string]any{
				"method": "ping",
			}); err != nil {
				ws.handleError(errnie.Error(err))
				continue
			}
		default:
		}

		message := sockMsgPool.Get().(map[string]any)
		defer sockMsgPool.Put(message)

		if err = ws.conn.ReadJSON(&message); err != nil {
			ws.handleError(errnie.Error(err))
			continue
		}

		if ch := ws.broadcasts["raw"]; ch != nil {
			ch.Send(&qpool.QValue[any]{
				Type:  message["type"].(string),
				Value: message,
			})
		}

		if message["type"].(string) == "pong" {
			ws.latencies.Value = time.Since(message["time_in"].(time.Time))
			ws.latencies.Next()
		}

		if message["type"].(string) == "ohlc" {
			ws.publishOhlc(message)
		}
	}
}

func (ws *WebSocket) handleError(err error) {
	switch err {
	case websocket.ErrCloseSent:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	case io.ErrUnexpectedEOF:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	case websocket.ErrNilConn:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	case websocket.ErrNilNetConn:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	case websocket.ErrReadLimit:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	case websocket.ErrBadHandshake:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	default:
		ws.Close()
		ws.Connect(WebSocketURL, "kraken:public", 0)
		return
	}
}

func (ws *WebSocket) publishOhlc(message map[string]any) {
	var candles []map[string]any

	if err := sonic.Unmarshal(message["data"].(json.RawMessage), &candles); err != nil {
		errnie.Error(err)
		return
	}

	for _, candle := range candles {
		if slices.Contains(ws.streams.Snapshot(), candle["symbol"].(string)) {
			ws.broadcasts["ui:charts"].Send(&qpool.QValue[any]{
				Type:  message["type"].(string),
				Value: candle,
			})
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.conn.Close()
	ws.cancel()
	return nil
}
