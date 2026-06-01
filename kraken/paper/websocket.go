package paper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

const methodAddOrder = "add_order"

var _ public.WebSocketClient = (*WebSocket)(nil)

/*
WebSocket simulates the authenticated Kraken WebSocket v2 trading socket.
*/
type WebSocket struct {
	ctx     context.Context
	cancel  context.CancelFunc
	handler func(payload []byte)
	mu      sync.Mutex
	streams map[string]chan *public.SocketMessage
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:     ctx,
		cancel:  cancel,
		streams: make(map[string]chan *public.SocketMessage),
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if _, ok := ws.streams[channel]; ok {
		return nil
	}

	ws.streams[channel] = make(chan *public.SocketMessage, 256)

	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	if channel != public.OrdersChannel {
		return nil
	}

	payload, err := sonic.Marshal(message)

	if err != nil {
		return fmt.Errorf("paper websocket encode: %w", err)
	}

	var frame struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return fmt.Errorf("paper websocket decode: %w", err)
	}

	if frame.Method != methodAddOrder {
		return fmt.Errorf("paper websocket: unsupported method %q", frame.Method)
	}

	return ws.simulateAddOrder(frame.Params)
}

func (ws *WebSocket) Stream(channel string) (<-chan *public.SocketMessage, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	stream, ok := ws.streams[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	return stream, nil
}

func (ws *WebSocket) Close(channel string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	stream, ok := ws.streams[channel]

	if !ok {
		return fmt.Errorf("channel %s not found", channel)
	}

	close(stream)
	delete(ws.streams, channel)

	return nil
}

func (ws *WebSocket) OnMessage(handler func(payload []byte)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handler = handler
}

func (ws *WebSocket) simulateAddOrder(params map[string]any) error {
	symbol, _ := params["symbol"].(string)
	orderQty, _ := params["order_qty"].(float64)

	if symbol == "" {
		return fmt.Errorf("paper order symbol is required")
	}

	if orderQty <= 0 {
		return fmt.Errorf("paper order quantity must be positive")
	}

	clOrdID, _ := params["cl_ord_id"].(string)

	if clOrdID == "" {
		clOrdID = nextPaperClOrdID()
	}

	orderID := nextPaperOrderID()
	fillPrice, _ := params["limit_price"].(float64)

	if fillPrice <= 0 {
		if triggers, ok := params["triggers"].(map[string]any); ok {
			fillPrice, _ = triggers["price"].(float64)
		}
	}

	if fillPrice <= 0 {
		return fmt.Errorf("paper fill price is required")
	}

	ackPayload, err := sonic.Marshal(map[string]any{
		"method":  methodAddOrder,
		"success": true,
		"result": map[string]any{
			"order_id":  orderID,
			"cl_ord_id": clOrdID,
		},
	})

	if err != nil {
		return err
	}

	execPayload, err := sonic.Marshal(map[string]any{
		"channel": "executions",
		"type":    "update",
		"data": []map[string]any{{
			"exec_type":    "trade",
			"order_id":     orderID,
			"cl_ord_id":    clOrdID,
			"symbol":       symbol,
			"side":         params["side"],
			"order_type":   params["order_type"],
			"order_qty":    orderQty,
			"last_qty":     orderQty,
			"last_price":   fillPrice,
			"cum_qty":      orderQty,
			"order_status": "filled",
			"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
			"exec_id":      nextPaperExecID(),
		}},
	})

	if err != nil {
		return err
	}

	ws.deliver(ackPayload)
	ws.deliver(execPayload)

	return nil
}

func (ws *WebSocket) deliver(payload []byte) {
	ws.mu.Lock()
	handler := ws.handler
	ws.mu.Unlock()

	if handler != nil {
		handler(payload)
	}

	var message public.SocketMessage

	if err := sonic.Unmarshal(payload, &message); err != nil {
		return
	}

	if message.Channel == "" {
		return
	}

	ws.mu.Lock()
	stream, ok := ws.streams[message.Channel]
	ws.mu.Unlock()

	if !ok {
		return
	}

	row := message

	select {
	case stream <- &row:
	case <-ws.ctx.Done():
	}
}

func nextPaperOrderID() string {
	return "PAPER-" + randomHex(8)
}

func nextPaperExecID() string {
	return randomHex(16)
}

func nextPaperClOrdID() string {
	return "p" + randomHex(8)
}

func randomHex(byteCount int) string {
	buf := make([]byte, byteCount)

	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}

	return hex.EncodeToString(buf)
}
