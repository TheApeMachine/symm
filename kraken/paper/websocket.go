package paper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

const methodAddOrder = "add_order"

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	for _, channel := range []string{"executions", "orders"} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(channel, 128)
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ws.ctx,
		"cancel": ws.cancel,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

func (ws *WebSocket) Tick() error {
	for message := range ws.subscribers["orders"].Incoming {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		default:
		}

		if message == nil || message.Value == nil {
			continue
		}

		payload, err := sonic.Marshal(message.Value)

		if err != nil {
			errnie.Error(err)
			continue
		}

		var frame struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}

		if err := sonic.Unmarshal(payload, &frame); err != nil {
			errnie.Error(err)
			continue
		}

		if frame.Method != methodAddOrder {
			continue
		}

		ws.simulateAddOrder(frame.Params)
	}

	return ws.ctx.Err()
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	return nil
}

func (ws *WebSocket) simulateAddOrder(params map[string]any) {
	symbol, _ := params["symbol"].(string)
	orderQty, _ := params["order_qty"].(float64)

	if symbol == "" || orderQty <= 0 {
		return
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
		return
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
		errnie.Error(err)
		return
	}

	var msg public.SocketMessage

	if err := sonic.Unmarshal(execPayload, &msg); err != nil {
		errnie.Error(err)
		return
	}

	ws.broadcasts["executions"].Send(&qpool.QValue[any]{Value: msg})
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
