package market

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/runstats"
)

const level3ControlSubscriberID = "kraken/market:level3-control"

const level3ReadPoll = 50 * time.Millisecond

/*
Level3WebSocket maintains an authenticated Kraken level3 connection and mirrors
public instrument subscriptions onto it for per-order toxicity tracking.
*/
type Level3WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	pool            *qpool.Q
	conn            *websocket.Conn
	connMu          sync.RWMutex
	reconnectPolicy *public.ReconnectPolicy
	broadcasts      *qpool.BroadcastGroup
	publicControl   *qpool.Subscriber
	subscribeReplay []any
	replayMu        sync.RWMutex
}

func NewLevel3WebSocket(ctx context.Context, pool *qpool.Q) *Level3WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	ws := &Level3WebSocket{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		reconnectPolicy: public.NewReconnectPolicyFromConfig(),
		subscribeReplay: make([]any, 0),
	}

	ws.broadcasts = pool.CreateBroadcastGroup("level3", 10*time.Millisecond)
	ws.publicControl = pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond).
		Subscribe(level3ControlSubscriberID, 1024)

	if Level3Available() {
		if err := ws.dialUntilConnected(public.WebSocketL3URL); err != nil {
			errnie.Error(err)
		} else {
			activate.Boot("kraken/market level3 websocket connected")
		}
	}

	return ws
}

func (ws *Level3WebSocket) Tick() error {
	if !Level3Available() {
		<-ws.ctx.Done()

		return ws.ctx.Err()
	}

	for {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case message, ok := <-ws.publicControl.Incoming:
			if !ok {
				return ws.ctx.Err()
			}

			if message == nil {
				continue
			}

			if err := ws.handlePublicControl(message.Value); err != nil {
				return err
			}
		default:
		}

		ws.connMu.RLock()
		connected := ws.conn != nil
		ws.connMu.RUnlock()

		if !connected {
			if err := ws.reconnect(public.WebSocketL3URL); err != nil {
				return err
			}

			continue
		}

		socketMessage, handled, err := ws.readInbound()

		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}

			if ws.ctx.Err() != nil {
				return ws.ctx.Err()
			}

			errnie.Error(err)

			if reconnectErr := ws.reconnect(public.WebSocketL3URL); reconnectErr != nil {
				return reconnectErr
			}

			continue
		}

		if handled {
			continue
		}

		if socketMessage.Channel != "" {
			activate.Once("kraken/market:level3:" + socketMessage.Channel)
		}

		ws.recordInbound(socketMessage)

		ws.broadcasts.Send(&qpool.QValue[any]{
			Type:  socketMessage.Channel,
			Value: socketMessage,
		})
	}
}

func (ws *Level3WebSocket) Close() error {
	ws.cancel()
	ws.dropConnection()

	return nil
}

func (ws *Level3WebSocket) handlePublicControl(value any) error {
	if err := ws.mirrorPublicSubscribe(value); err != nil {
		return err
	}

	return nil
}

func (ws *Level3WebSocket) mirrorPublicSubscribe(value any) error {
	frame, ok := value.(map[string]any)

	if !ok {
		return nil
	}

	method, _ := frame["method"].(string)

	if method != "subscribe" {
		return nil
	}

	params, ok := frame["params"].(map[string]any)

	if !ok {
		return nil
	}

	channel, _ := params["channel"].(string)

	switch channel {
	case public.TickerChannel, public.BookChannel, public.TradesChannel:
	default:
		return nil
	}

	symbols := stringListFromAny(params["symbol"])

	if len(symbols) == 0 {
		return nil
	}

	depth := level3DepthFromViper()

	if channel == public.BookChannel {
		if configured, ok := params["depth"].(float64); ok && configured > 0 {
			depth = normalizeLevel3Depth(int(configured))
		}
	}

	return ws.subscribeLevel3(symbols, depth)
}

func (ws *Level3WebSocket) subscribeLevel3(symbols []string, depth int) error {
	token, err := orderToken(ws.ctx)

	if err != nil {
		return err
	}

	frame := map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  public.Level3Channel,
			"symbol":   symbols,
			"depth":    depth,
			"snapshot": true,
			"token":    token,
		},
	}

	ws.recordSubscribeFrame(frame)

	return ws.writeOutboundFrame(frame, true)
}

func level3DepthFromViper() int {
	depth := viper.GetInt("market.book_depth_levels")

	return normalizeLevel3Depth(depth)
}

func normalizeLevel3Depth(configured int) int {
	if configured <= 0 {
		return 10
	}

	if configured <= 10 {
		return 10
	}

	if configured <= 100 {
		return 100
	}

	return 1000
}

func stringListFromAny(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		symbols := make([]string, 0, len(values))

		for _, value := range values {
			symbol, ok := value.(string)

			if ok && symbol != "" {
				symbols = append(symbols, symbol)
			}
		}

		return symbols
	default:
		return nil
	}
}

func (ws *Level3WebSocket) recordSubscribeFrame(frame map[string]any) {
	ws.replayMu.Lock()
	defer ws.replayMu.Unlock()

	ws.subscribeReplay = append(ws.subscribeReplay, frame)
}

func (ws *Level3WebSocket) replaySubscribeFrames() error {
	ws.replayMu.RLock()
	frames := append([]any(nil), ws.subscribeReplay...)
	ws.replayMu.RUnlock()

	for _, frame := range frames {
		subscribeFrame, ok := frame.(map[string]any)

		if !ok {
			continue
		}

		params, paramsOK := subscribeFrame["params"].(map[string]any)

		if paramsOK {
			token, tokenErr := orderToken(ws.ctx)

			if tokenErr != nil {
				return errnie.Error(tokenErr)
			}

			params["token"] = token
		}

		if err := ws.writeOutboundFrame(subscribeFrame, false); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (ws *Level3WebSocket) writeOutboundFrame(frame map[string]any, record bool) error {
	if record {
		payload, err := sonic.Marshal(frame)

		if err == nil {
			_ = replay.WriteWS(public.Level3Channel, replay.DirectionOut, payload)
		}
	}

	ws.connMu.RLock()
	conn := ws.conn
	ws.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("kraken/market level3 websocket: not connected")
	}

	return conn.WriteJSON(frame)
}

func (ws *Level3WebSocket) recordInbound(message public.SocketMessage) {
	payload, err := sonic.Marshal(message)

	if err != nil {
		return
	}

	_ = replay.WriteWS(public.Level3Channel, replay.DirectionIn, payload)
}

func (ws *Level3WebSocket) readInbound() (public.SocketMessage, bool, error) {
	ws.connMu.RLock()
	conn := ws.conn
	ws.connMu.RUnlock()

	if conn == nil {
		return public.SocketMessage{}, false, fmt.Errorf("kraken/market level3 websocket: not connected")
	}

	if err := conn.SetReadDeadline(time.Now().Add(level3ReadPoll)); err != nil {
		return public.SocketMessage{}, false, err
	}

	_, payload, err := conn.ReadMessage()

	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return public.SocketMessage{}, false, os.ErrDeadlineExceeded
		}

		return public.SocketMessage{}, false, err
	}

	_ = conn.SetReadDeadline(time.Time{})

	var frame map[string]any

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return public.SocketMessage{}, false, err
	}

	if method, _ := frame["method"].(string); method == "pong" {
		return public.SocketMessage{}, true, nil
	}

	var message public.SocketMessage

	if err := sonic.Unmarshal(payload, &message); err != nil {
		return public.SocketMessage{}, false, err
	}

	return message, false, nil
}

func (ws *Level3WebSocket) dropConnection() {
	ws.connMu.Lock()
	defer ws.connMu.Unlock()

	if ws.conn == nil {
		return
	}

	_ = ws.conn.Close()
	ws.conn = nil
}

func (ws *Level3WebSocket) dialUntilConnected(endpoint public.EndpointType) error {
	if endpoint == "" {
		endpoint = public.WebSocketL3URL
	}

	for {
		delay := ws.reconnectPolicy.NextDelay()

		select {
		case <-ws.ctx.Done():
			return errnie.Error(ws.ctx.Err())
		case <-time.After(delay):
		}

		conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

		if err != nil {
			errnie.Error(err)

			continue
		}

		ws.connMu.Lock()
		ws.conn = conn
		ws.connMu.Unlock()

		ws.reconnectPolicy.Reset()
		runstats.WSConnect()
		activate.Once("kraken/market:level3:connected")

		return nil
	}
}

func (ws *Level3WebSocket) reconnect(endpoint public.EndpointType) error {
	if endpoint == "" {
		endpoint = public.WebSocketL3URL
	}

	ws.dropConnection()

	runstats.WSReconnect()
	activate.Boot("kraken/market level3 websocket reconnecting")

	for {
		delay := ws.reconnectPolicy.NextDelay()

		select {
		case <-ws.ctx.Done():
			return errnie.Error(ws.ctx.Err())
		case <-time.After(delay):
		}

		conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

		if err != nil {
			errnie.Error(err)

			continue
		}

		ws.connMu.Lock()
		ws.conn = conn
		ws.connMu.Unlock()

		ws.reconnectPolicy.Reset()
		runstats.WSConnect()
		activate.Boot("kraken/market level3 websocket reconnected")

		if replayErr := ws.replaySubscribeFrames(); replayErr != nil {
			ws.dropConnection()

			errnie.Error(replayErr)

			continue
		}

		return nil
	}
}
