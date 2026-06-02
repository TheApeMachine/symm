package public

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

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
				return fmt.Errorf("kraken/public websocket: market.subscribe_pace must be positive")
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
