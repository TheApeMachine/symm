package public

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
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
	publicSubscribePace      = 50 * time.Millisecond
)

type WebSocketClient interface {
	Connect(endpoint EndpointType, channel string) error
	Tick() error
	Close() error
}

type SocketMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

type WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q
	conn            *websocket.Conn
	connMu          sync.RWMutex
	reconnectPolicy *ReconnectPolicy
	subscribeReplay []any
	replayMu        sync.RWMutex
	broadcasts      map[string]*qpool.BroadcastGroup
	subscribers     map[string]*qpool.Subscriber
	recorder        io.Writer
	pairs           []string
	ui              *qpool.BroadcastGroup
	streams         *focus.Set
	ohlcSubscribed  map[string]struct{}
	lastSubscribe   time.Time
	subscribeMu     sync.Mutex
}

func NewWebSocket(ctx context.Context, pool *qpool.Q, streams *focus.Set) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:             ctx,
			cancel:          cancel,
			pool:            pool,
			reconnectPolicy: NewReconnectPolicyFromConfig(),
			broadcasts:      make(map[string]*qpool.BroadcastGroup),
			subscribers:     make(map[string]*qpool.Subscriber),
			ohlcSubscribed:  make(map[string]struct{}),
			subscribeReplay: make([]any, 0),
		}
	})

	socket.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	socket.bindChartStreams(streams)

	for _, channel := range []string{
		"raw", "level3",
	} {
		socket.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)

		subscriberID := publicRawSubscriberID

		if channel == "level3" {
			subscriberID = publicLevel3SubscriberID
		}

		socket.subscribers[channel] = socket.broadcasts[channel].Subscribe(subscriberID, 1024)
	}

	socket.broadcasts["kraken:public"] = pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
	socket.subscribers["kraken:public"] = socket.broadcasts["kraken:public"].Subscribe(
		publicOutboundSubscriberID, 8192,
	)

	outboundOnce.Do(func() {
		go socket.runOutbound()
	})

	if viper.GetViper().Get("trading.model") == "record" {
		recorder, err := os.Create(viper.GetViper().GetString("trading.record.file"))

		if err != nil {
			errnie.Error(err)
		}

		socket.recorder = bufio.NewWriter(recorder)
	}

	if err := socket.dialUntilConnected(ctx, WebSocketURL); err != nil {
		errnie.Error(err)
	} else {
		activate.Boot("kraken/public websocket connected")
	}

	errnie.Debug("kraken.public.websocket.NewWebSocket", "subscribing to", "instrument")

	socket.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "instrument",
			"snapshot": true,
		},
	}})

	activate.Boot("kraken/public websocket ready")

	return socket
}

func (ws *WebSocket) Connect(endpoint EndpointType, channel string) error {
	errnie.Debug("kraken.public.websocket.Connect", endpoint, channel)

	ws.connMu.RLock()
	connected := ws.conn != nil
	ws.connMu.RUnlock()

	if connected {
		return nil
	}

	return ws.dialUntilConnected(ws.ctx, endpoint)
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		default:
		}

		ws.connMu.RLock()
		connected := ws.conn != nil
		ws.connMu.RUnlock()

		if !connected {
			if err := ws.reconnect(WebSocketURL); err != nil {
				return errnie.Error(err)
			}

			continue
		}

		var message SocketMessage

		if err := ws.readMessage(&message); err != nil {
			if ws.ctx.Err() != nil {
				return errnie.Error(ws.ctx.Err())
			}

			errnie.Error(err)

			if reconnectErr := ws.reconnect(WebSocketURL); reconnectErr != nil {
				return errnie.Error(reconnectErr)
			}

			continue
		}

		if message.Channel != "" {
			activate.Once("kraken/public:channel:" + message.Channel)
		}

		if ch := ws.broadcasts["raw"]; ch != nil {
			ch.Send(&qpool.QValue[any]{
				Type:  message.Channel,
				Value: message,
			})
		}

		if message.Channel == CandlesChannel {
			ws.applyOhlc(message)
		}
	}
}

func (ws *WebSocket) runOutbound() {
	inbound := ws.subscribers["kraken:public"].Incoming

	for {
		select {
		case <-ws.ctx.Done():
			return
		case message, ok := <-inbound:
			if !ok {
				return
			}

			if message == nil {
				continue
			}

			ws.connMu.RLock()
			connected := ws.conn != nil
			ws.connMu.RUnlock()

			if !connected {
				if err := ws.reconnect(WebSocketURL); err != nil {
					return
				}
			}

			for {
				if err := ws.writeOutbound(message.Value); err != nil {
					if ws.ctx.Err() != nil {
						return
					}

					errnie.Error(err)

					if reconnectErr := ws.reconnect(WebSocketURL); reconnectErr != nil {
						return
					}

					continue
				}

				break
			}
		}
	}
}

func (ws *WebSocket) writeOutbound(value any) error {
	return ws.writeOutboundFrame(value, true)
}

func (ws *WebSocket) writeOutboundFrame(value any, record bool) error {
	if record {
		if frame, ok := value.(map[string]any); ok {
			if method, _ := frame["method"].(string); method == "subscribe" {
				ws.recordSubscribeFrame(value)
			}
		}
	}

	if frame, ok := value.(map[string]any); ok {
		if method, _ := frame["method"].(string); method == "subscribe" {
			pace := viper.GetDuration("market.subscribe_pace")

			if pace <= 0 {
				pace = publicSubscribePace
			}

			ws.subscribeMu.Lock()

			if since := time.Since(ws.lastSubscribe); since < pace {
				time.Sleep(pace - since)
			}

			ws.lastSubscribe = time.Now()
			ws.subscribeMu.Unlock()
		}
	}

	ws.connMu.RLock()
	conn := ws.conn
	ws.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("kraken public websocket: not connected")
	}

	return conn.WriteJSON(value)
}

func (ws *WebSocket) bindChartStreams(streams *focus.Set) {
	if streams == nil {
		return
	}

	ws.streams = streams
	streams.SetStreamNotifier(ws.onStreamChange)
	ws.ensureOhlcSubscription(focus.AnchorSymbol())

	for _, symbol := range streams.Snapshot() {
		ws.ensureOhlcSubscription(symbol)
	}
}

func (ws *WebSocket) onStreamChange(symbol string, added bool) {
	if !added {
		return
	}

	ws.ensureOhlcSubscription(symbol)
}

func (ws *WebSocket) ensureOhlcSubscription(symbol string) {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return
	}

	if _, known := ws.ohlcSubscribed[symbol]; known {
		return
	}

	ws.ohlcSubscribed[symbol] = struct{}{}

	outbound := ws.broadcasts["kraken:public"]

	if outbound == nil {
		return
	}

	outbound.Send(&qpool.QValue[any]{
		Value: OhlcSubscribeFrame(symbol),
	})
}

// applyOhlc publishes Kraken v2 ohlc rows as candle_bar frames for chart-stream
// symbols only. Updates share interval_begin so the frontend updates bars in place.
func (ws *WebSocket) applyOhlc(msg SocketMessage) {
	if ws.ui == nil {
		return
	}

	candles, err := DecodeOhlc(&msg)

	if err != nil {
		errnie.Error(err)
		return
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	for _, candle := range candles {
		if candle.Symbol == "" {
			continue
		}

		if ws.streams != nil && !ws.streams.Streams(candle.Symbol) {
			continue
		}

		sec, secErr := CandleIntervalSec(candle)

		if secErr != nil {
			errnie.Error(secErr)
			continue
		}

		ws.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":          "candle_bar",
			"ts":             nowStr,
			"symbol":         candle.Symbol,
			"sec":            sec,
			"interval_begin": candle.IntervalBegin,
			"interval":       candle.Interval,
			"open":           candle.Open,
			"high":           candle.High,
			"low":            candle.Low,
			"close":          candle.Close,
			"volume":         candle.Volume,
			"trades":         candle.Trades,
			"vwap":           candle.VWAP,
		}})
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	ws.dropConnection()

	return nil
}
