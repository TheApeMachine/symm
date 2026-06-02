package public

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/runstats"
)

const defaultReconnectInitial = time.Second

const defaultReconnectMax = 2 * time.Minute

const defaultReconnectMultiplier = 2.0

/*
ReconnectPolicy spaces dial attempts with exponential backoff capped at max.
*/
type ReconnectPolicy struct {
	mu      sync.Mutex
	initial time.Duration
	max     time.Duration
	factor  float64
	attempt uint64
}

/*
NewReconnectPolicyFromConfig loads backoff timings from market.ws_reconnect_*.
*/
func NewReconnectPolicyFromConfig() *ReconnectPolicy {
	initial := viper.GetDuration("market.ws_reconnect_initial")

	if initial <= 0 {
		initial = defaultReconnectInitial
	}

	maximum := viper.GetDuration("market.ws_reconnect_max")

	if maximum <= 0 {
		maximum = defaultReconnectMax
	}

	factor := viper.GetFloat64("market.ws_reconnect_multiplier")

	if factor < 1 {
		factor = defaultReconnectMultiplier
	}

	return &ReconnectPolicy{
		initial: initial,
		max:     maximum,
		factor:  factor,
	}
}

/*
NextDelay returns the wait before the next dial attempt and advances the attempt counter.
*/
func (policy *ReconnectPolicy) NextDelay() time.Duration {
	policy.mu.Lock()
	defer policy.mu.Unlock()

	delay := policy.initial

	for step := uint64(0); step < policy.attempt; step++ {
		scaled := time.Duration(float64(delay) * policy.factor)

		if scaled <= delay || scaled > policy.max {
			delay = policy.max

			break
		}

		delay = scaled
	}

	policy.attempt++

	return delay
}

/*
Reset clears the attempt counter after a successful connection.
*/
func (policy *ReconnectPolicy) Reset() {
	policy.mu.Lock()
	defer policy.mu.Unlock()

	policy.attempt = 0
}

func cloneSubscribeFrame(value any) (map[string]any, bool) {
	frame, ok := value.(map[string]any)

	if !ok {
		return nil, false
	}

	method, _ := frame["method"].(string)

	if method != "subscribe" {
		return nil, false
	}

	raw, err := sonic.Marshal(frame)

	if err != nil {
		return nil, false
	}

	var cloned map[string]any

	if err := sonic.Unmarshal(raw, &cloned); err != nil {
		return nil, false
	}

	return cloned, true
}

func (ws *WebSocket) recordSubscribeFrame(value any) {
	cloned, ok := cloneSubscribeFrame(value)

	if !ok {
		return
	}

	ws.replayMu.Lock()
	defer ws.replayMu.Unlock()

	ws.subscribeReplay = append(ws.subscribeReplay, cloned)
}

func (ws *WebSocket) replaySubscribeFrames() error {
	ws.replayMu.RLock()
	frames := append([]any(nil), ws.subscribeReplay...)
	ws.replayMu.RUnlock()

	for _, frame := range frames {
		if err := ws.writeOutboundFrame(frame, false); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
reconnect closes the current socket, backs off, redials, and replays subscribe frames.
*/
func (ws *WebSocket) reconnect(endpoint EndpointType) error {
	if endpoint == "" {
		endpoint = WebSocketURL
	}

	ws.dropConnection()

	runstats.WSReconnect()
	activate.Boot("kraken/public websocket reconnecting")

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
		activate.Boot("kraken/public websocket reconnected")

		if replayErr := ws.replaySubscribeFrames(); replayErr != nil {
			ws.dropConnection()

			errnie.Error(replayErr)

			continue
		}

		return nil
	}
}

func (ws *WebSocket) dropConnection() {
	ws.connMu.Lock()
	defer ws.connMu.Unlock()

	if ws.conn == nil {
		return
	}

	_ = ws.conn.Close()
	ws.conn = nil
}

func (ws *WebSocket) dialUntilConnected(ctx context.Context, endpoint EndpointType) error {
	if endpoint == "" {
		endpoint = WebSocketURL
	}

	for {
		delay := ws.reconnectPolicy.NextDelay()

		select {
		case <-ctx.Done():
			return errnie.Error(ctx.Err())
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
		activate.Once("kraken/public:connected:" + string(endpoint))

		return nil
	}
}

func (ws *WebSocket) readInbound(message *SocketMessage) (handled bool, err error) {
	ws.connMu.RLock()
	conn := ws.conn
	ws.connMu.RUnlock()

	if conn == nil {
		return false, fmt.Errorf("kraken public websocket: not connected")
	}

	_, payload, err := conn.ReadMessage()

	if err != nil {
		return false, err
	}

	var frame map[string]any

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return false, err
	}

	if method, _ := frame["method"].(string); method == "pong" {
		if ws.latencyProbe != nil {
			ws.latencyProbe.observePong(frame)
		}

		return true, nil
	}

	if err := sonic.Unmarshal(payload, message); err != nil {
		return false, err
	}

	return false, nil
}

func (ws *WebSocket) readMessage(message *SocketMessage) error {
	_, err := ws.readInbound(message)

	return err
}
