package public

import (
	"container/ring"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/focus"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocketClient interface {
	Connect(EndpointType, string, uint64) error
	Tick() error
	Close() error
}

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	conns       []*websocket.Conn
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.BroadcastConsumer
	recorder    io.Writer
	streams     *focus.Set
	latencies   *ring.Ring
	isConnected bool
	reconnectAttempt uint64
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	streams *focus.Set,
	conns ...*websocket.Conn,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	if len(conns) == 0 {
		conns = make([]*websocket.Conn, 1)
	}

	if streams == nil {
		streams = focus.NewSet()
	}

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:         ctx,
			cancel:      cancel,
			pool:        pool,
			broadcasts:  make(map[string]*qpool.BroadcastGroup),
			subscribers: make(map[string]*qpool.BroadcastConsumer),
			latencies:   newLatencyRing(64),
			conns:       conns,
			streams:     streams,
		}

		for _, channel := range []string{"raw", "level3", "kraken:public"} {
			socket.broadcasts[channel] = bus.Group(pool, channel, 10*time.Millisecond)
			socket.subscribers[channel] = socket.broadcasts[channel].Subscribe(channel, 1024)
		}

		socket.broadcasts["ui"] = bus.Group(pool, "ui", 10*time.Millisecond)

		socket.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel":  "instrument",
				"snapshot": true,
			},
		}})

		errnie.Info("kraken/public websocket ready", "kraken/public websocket ready")

		socket.streams.Add(focus.AnchorSymbol())
	})

	return socket
}

func newLatencyRing(size int) *ring.Ring {
	latencies := ring.New(size)
	cursor := latencies

	for index := 0; index < size; index++ {
		cursor.Value = time.Duration(0)
		cursor = cursor.Next()
	}

	return latencies
}

/*
AppendConn registers an authenticated WebSocket on the process public socket.
*/
func AppendConn(conn *websocket.Conn) {
	if conn == nil || socket == nil {
		return
	}

	socket.conns = append(socket.conns, conn)
}

func (ws *WebSocket) Connect(
	endpoint EndpointType, channel string, n uint64,
) error {
	delay := reconnectBackoff(n)

	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case <-timer.C:
		}
	}

	if err := ws.tryConnect(endpoint); err != nil {
		return ws.Connect(endpoint, channel, n+1)
	}

	return nil
}

func reconnectBackoff(attempt uint64) time.Duration {
	if attempt == 0 {
		return 0
	}

	seconds := uint64(
		math.Round((math.Pow(
			math.Phi, float64(attempt),
		) + math.Pow(
			math.Phi-1, float64(attempt),
		)) / math.Sqrt(5)),
	)

	const maxSeconds = 30

	if seconds > maxSeconds {
		seconds = maxSeconds
	}

	return time.Duration(seconds) * time.Second
}

func (ws *WebSocket) tryConnect(endpoint EndpointType) error {
	conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

	if err != nil {
		ws.err = err

		return err
	}

	if ws.conns[0] != nil {
		_ = ws.conns[0].Close()
	}

	ws.conns[0] = conn
	ws.reconnectAttempt = 0
	ws.afterConnect()

	return nil
}

func (ws *WebSocket) markDisconnected() {
	ws.isConnected = false

	if ws.conns[0] != nil {
		_ = ws.conns[0].Close()
		ws.conns[0] = nil
	}
}

func (ws *WebSocket) ensureConnected() error {
	if ws.isConnected && ws.conns[0] != nil {
		return nil
	}

	delay := reconnectBackoff(ws.reconnectAttempt)

	if delay > 0 {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case <-time.After(delay):
		}
	}

	if err := ws.tryConnect(WebSocketURL); err != nil {
		ws.reconnectAttempt++

		return err
	}

	return nil
}

func (ws *WebSocket) afterConnect() {
	ws.isConnected = true

	if outbound := ws.broadcasts["kraken:public"]; outbound != nil {
		outbound.Send(&qpool.QValue[any]{Value: map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel":  "instrument",
				"snapshot": true,
			},
		}})
	}

	errnie.Info("kraken/public websocket connected")
	notifyReconnect()
}

type SocketMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Errors  []string        `json:"errors"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	TimeIn  time.Time       `json:"time_in"`
	TimeOut time.Time       `json:"time_out"`
}

var sockMsgPool = sync.Pool{
	New: func() any {
		return &SocketMessage{}
	},
}

func (ws *WebSocket) Tick() (err error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	outbound := ws.subscribers["kraken:public"]

	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case <-ticker.C:
			if ws.conns[0] == nil {
				continue
			}

			if err = ws.conns[0].WriteJSON(map[string]any{
				"method": "ping",
			}); err != nil {
				ws.handleError(errnie.Error(err))
				continue
			}

			if time.Now().Unix()%64 == 0 {
				ws.recordLatency()
			}
		default:
			if message := outbound.Poll(); message != nil {
				if ws.conns[0] != nil {
					errnie.Error(ws.conns[0].WriteJSON(message.Value))
				}

				continue
			}
		}

		if !ws.isConnected || ws.conns[0] == nil {
			if err := ws.ensureConnected(); err != nil {
				errnie.Error(err)
			}

			continue
		}

		if err = ws.readFrame(); err != nil {
			ws.handleError(errnie.Error(err))
		}
	}
}

func (ws *WebSocket) readFrame() (err error) {
	message := sockMsgPool.Get().(*SocketMessage)
	defer sockMsgPool.Put(message)

	*message = SocketMessage{}

	if err = ws.conns[0].ReadJSON(message); err != nil {
		return err
	}

	if message.Channel == "heartbeat" {
		ws.isConnected = true
		return nil
	}

	if message.Channel != "" {
		if ch := ws.broadcasts["raw"]; ch != nil {
			ch.Send(&qpool.QValue[any]{
				Type: message.Channel,
				Value: map[string]any{
					"channel": message.Channel,
					"type":    message.Type,
					"data":    append(json.RawMessage(nil), message.Data...),
				},
			})
		}
	}

	if message.Type == "pong" {
		ws.latencies.Value = time.Since(message.TimeIn)
		ws.latencies = ws.latencies.Next()
	}

	if message.Channel == "ohlc" {
		ws.publishOhlc(message.Data)
	}

	return nil
}

func (ws *WebSocket) handleError(err error) {
	errnie.Error(err)
	ws.markDisconnected()
}

func (ws *WebSocket) recordLatency() {
	// Write the latency ring to the file.
	latencyFile, err := os.OpenFile("runs/network_latency.json", os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		errnie.Error(err)
		return
	}

	defer latencyFile.Close()

	ws.latencies.Do(func(value any) {
		duration, _ := value.(time.Duration)
		fmt.Fprintf(latencyFile, "%d\n", duration)
	})
}

func (ws *WebSocket) publishOhlc(message json.RawMessage) {
	var candles []map[string]any

	if err := sonic.Unmarshal(message, &candles); err != nil {
		errnie.Error(err)
		return
	}

	ui := ws.broadcasts["ui"]

	for _, candle := range candles {
		sym, _ := candle["symbol"].(string)

		if slices.Contains(ws.streams.Snapshot(), sym) && ui != nil {
			ui.Send(&qpool.QValue[any]{
				Type:  "ohlc",
				Value: candle,
			})
		}
	}
}

func (ws *WebSocket) Close() error {
	if len(ws.conns) > 0 && ws.conns[0] != nil {
		_ = ws.conns[0].Close()
	}

	ws.cancel()
	return nil
}
