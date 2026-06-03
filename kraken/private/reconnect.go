package private

import (
	"fmt"
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/runstats"
)

func cloneOutboundFrame(value any) (any, bool) {
	raw, err := sonic.Marshal(value)

	if err != nil {
		return nil, false
	}

	var cloned any

	if err := sonic.Unmarshal(raw, &cloned); err != nil {
		return nil, false
	}

	return cloned, true
}

func (ws *WebSocket) recordOutboundFrame(value any) {
	cloned, ok := cloneOutboundFrame(value)

	if !ok {
		return
	}

	ws.replayMu.Lock()
	defer ws.replayMu.Unlock()

	ws.outboundReplay = append(ws.outboundReplay, cloned)
}

func (ws *WebSocket) replayOutboundFrames() error {
	ws.replayMu.RLock()
	frames := append([]any(nil), ws.outboundReplay...)
	ws.replayMu.RUnlock()

	for _, frame := range frames {
		if err := ws.writeOutboundFrame(frame, false); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (ws *WebSocket) dropConnection() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.conn == nil {
		return
	}

	_ = ws.conn.Close()
	ws.conn = nil
}

func (ws *WebSocket) dialUntilConnected(endpoint public.EndpointType) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	for attempt := uint64(0); ; attempt++ {
		select {
		case <-ws.ctx.Done():
			return errnie.Error(ws.ctx.Err())
		case <-time.After(dialBackoff(attempt)):
		}

		conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

		if err != nil {
			errnie.Error(err)

			continue
		}

		ws.mu.Lock()
		ws.conn = conn
		ws.mu.Unlock()

		runstats.WSConnect()
		activate.Once("kraken/private:connected:" + string(endpoint))

		return nil
	}
}

func (ws *WebSocket) reconnect(endpoint public.EndpointType) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	ws.dropConnection()

	runstats.WSReconnect()
	activate.Boot("kraken/private websocket reconnecting")

	for attempt := uint64(0); ; attempt++ {
		select {
		case <-ws.ctx.Done():
			return errnie.Error(ws.ctx.Err())
		case <-time.After(dialBackoff(attempt)):
		}

		conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

		if err != nil {
			errnie.Error(err)

			continue
		}

		ws.mu.Lock()
		ws.conn = conn
		ws.mu.Unlock()

		runstats.WSConnect()
		activate.Boot("kraken/private websocket reconnected")

		if replayErr := ws.replayOutboundFrames(); replayErr != nil {
			ws.dropConnection()

			errnie.Error(replayErr)

			continue
		}

		return nil
	}
}

func (ws *WebSocket) writeOutboundFrame(value any, record bool) error {
	if record {
		ws.recordOutboundFrame(value)
	}

	ws.mu.Lock()
	conn := ws.conn
	ws.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("kraken private websocket: not connected")
	}

	return conn.WriteJSON(value)
}

func dialBackoff(attempt uint64) time.Duration {
	delay := uint64(
		math.Round((math.Pow(
			math.Phi, float64(attempt),
		) + math.Pow(
			math.Phi-1, float64(attempt),
		)) / math.Sqrt(5)),
	)

	if delay == 0 {
		delay = 1
	}

	return time.Duration(delay) * time.Second
}
