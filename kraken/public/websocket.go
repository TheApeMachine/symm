package public

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
)

var socket *WebSocket
var socketOnce sync.Once

const publicOutboundSubscriberID = "kraken:public:websocket"

const (
	publicRawSubscriberID    = "kraken/public:raw"
	publicLevel3SubscriberID = "kraken/public:level3"
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

// ohlcBar tracks the running OHLC state within a minute for one symbol.
type ohlcBar struct {
	open, high, low, close float64
	intervalSec            int64
}

// tickerFrame is the minimal subset of a Kraken v2 ticker data row needed for OHLC.
type tickerFrame struct {
	Symbol string  `json:"symbol"`
	Last   float64 `json:"last"`
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
	pairs       []string
	ui          *qpool.BroadcastGroup
	candles     map[string]*ohlcBar
	watchSet    map[string]struct{}
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:         ctx,
			cancel:      cancel,
			pool:        pool,
			broadcasts:  make(map[string]*qpool.BroadcastGroup),
			subscribers: make(map[string]*qpool.Subscriber),
			candles:     make(map[string]*ohlcBar),
			watchSet:    buildWatchSet(viper.GetStringSlice("market.symbols")),
		}
	})

	socket.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	for _, channel := range []string{
		"raw", "level3",
	} {
		socket.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)

		subscriberID := publicRawSubscriberID

		if channel == "level3" {
			subscriberID = publicLevel3SubscriberID
		}

		socket.subscribers[channel] = socket.broadcasts[channel].Subscribe(subscriberID, 128)
	}

	socket.broadcasts["kraken:public"] = pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
	socket.subscribers["kraken:public"] = socket.broadcasts["kraken:public"].Subscribe(
		publicOutboundSubscriberID, 128,
	)

	if viper.GetViper().Get("trading.model") == "record" {
		recorder, err := os.Create(viper.GetViper().GetString("trading.record.file"))

		if err != nil {
			errnie.Error(err)
		}

		socket.recorder = bufio.NewWriter(recorder)
	}

	if err := socket.Connect(WebSocketURL, "kraken:public"); err != nil {
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

	if endpoint == "" {
		endpoint = WebSocketURL
	}

	if ws.conn, _, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), nil,
	); ws.err != nil {
		return ws.err
	}

	activate.Once("kraken/public:connected:" + string(endpoint))

	return nil
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case message, ok := <-ws.subscribers["kraken:public"].Incoming:
			if !ok {
				errnie.Debug("kraken.public.websocket.Tick", "no ok")
				return nil
			}

			if message == nil {
				errnie.Debug("kraken.public.websocket.Tick", "nil message")
				continue
			}

			if ws.conn == nil {
				return fmt.Errorf("kraken public websocket: not connected")
			}

			errnie.Error(ws.conn.WriteJSON(message.Value))
		default:
		}

		if ws.conn == nil {
			return fmt.Errorf("kraken public websocket: not connected")
		}

		var message SocketMessage

		if err := ws.conn.ReadJSON(&message); err != nil {
			return errnie.Error(err)
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

		if message.Channel == TickerChannel {
			ws.applyTickers(message)
		}
	}
}

// applyTickers builds per-symbol 1-minute OHLC bars from ticker last-prices and
// publishes candle_bar events to the "ui" broadcast. Only symbols listed in
// market.symbols are published; an empty list passes all symbols.
func (ws *WebSocket) applyTickers(msg SocketMessage) {
	if ws.ui == nil {
		return
	}

	var frames []tickerFrame
	if err := json.Unmarshal(msg.Data, &frames); err != nil {
		return
	}

	now := time.Now()
	nowStr := now.UTC().Format(time.RFC3339Nano)
	floor := (now.Unix() / 60) * 60

	for _, t := range frames {
		if t.Last <= 0 || t.Symbol == "" {
			continue
		}
		if len(ws.watchSet) > 0 {
			if _, ok := ws.watchSet[t.Symbol]; !ok {
				continue
			}
		}

		bar, exists := ws.candles[t.Symbol]
		if !exists || bar.intervalSec != floor {
			ws.candles[t.Symbol] = &ohlcBar{
				open: t.Last, high: t.Last, low: t.Last, close: t.Last,
				intervalSec: floor,
			}
			bar = ws.candles[t.Symbol]
		} else {
			bar.close = t.Last
			if t.Last > bar.high {
				bar.high = t.Last
			}
			if t.Last < bar.low {
				bar.low = t.Last
			}
		}

		ws.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "candle_bar",
			"ts":     nowStr,
			"symbol": t.Symbol,
			"sec":    bar.intervalSec,
			"open":   bar.open,
			"high":   bar.high,
			"low":    bar.low,
			"close":  bar.close,
			"volume": 0.0,
		}})
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	if ws.conn == nil {
		return nil
	}

	return errnie.Error(ws.conn.Close())
}

func buildWatchSet(symbols []string) map[string]struct{} {
	watchSet := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		watchSet[symbol] = struct{}{}
	}

	return watchSet
}
