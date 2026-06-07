package private

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

func (websocketClient *WebSocket) markDisconnected() {
	if websocketClient.conn != nil {
		_ = websocketClient.conn.Close()
		websocketClient.conn = nil
	}
}

func (websocketClient *WebSocket) ensureConnected(endpoint public.EndpointType) error {
	if websocketClient.conn != nil {
		return nil
	}

	delay := reconnectDelay(websocketClient.reconnectAttempt)

	if delay > 0 {
		select {
		case <-websocketClient.ctx.Done():
			return websocketClient.ctx.Err()
		case <-time.After(delay):
		}
	}

	conn, _, err := websocketClient.dialer.Dial(string(endpoint), nil)

	if err != nil {
		websocketClient.reconnectAttempt++
		websocketClient.err = err

		return err
	}

	websocketClient.conn = conn
	websocketClient.reconnectAttempt = 0
	websocketClient.err = nil

	return nil
}

func (websocketClient *WebSocket) resubscribeLevel3() {
	if websocketClient.conn == nil || len(websocketClient.l3Symbols) == 0 {
		return
	}

	symbols := make([]string, 0, len(websocketClient.l3Symbols))

	for symbol := range websocketClient.l3Symbols {
		symbols = append(symbols, symbol)
	}

	if err := websocketClient.subscribeLevel3(symbols); err != nil {
		errnie.Error(err, "kraken/private: level3 resubscribe")
	}
}
