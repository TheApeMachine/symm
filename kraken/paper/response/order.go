package response

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
Orders simulates Kraken private order methods and streams fills on executions.
*/
type Orders struct {
	ctx       context.Context
	pool      *qpool.Q
	quotes    *broker.QuoteCache
	client    *trading.OrderClient
	observers []types.Socket
}

func NewOrders(ctx context.Context, pool *qpool.Q) *Orders {
	return &Orders{
		ctx:    ctx,
		pool:   pool,
		client: trading.NewOrderClient(ctx, pool),
	}
}

func (orders *Orders) Send(message *qpool.QValue[any]) map[string]any {
	frame, ok := message.Value.(map[string]any)

	if !ok {
		return nil
	}

	out := map[string]any{
		"method":   frame["method"],
		"req_id":   frame["req_id"],
		"success":  true,
		"time_in":  frame["time_in"],
		"time_out": time.Now(),
	}

	switch frame["method"] {
	case trading.MethodAddOrder:
		var update trading.OrderUpdate
		sonic.Unmarshal(frame["data"].(json.RawMessage), &update)
		out["result"] = update
	case trading.MethodAmendOrder:
		var update trading.OrderUpdate
		sonic.Unmarshal(frame["data"].(json.RawMessage), &update)
		out["result"] = update
	}

	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{
			Type:  "kraken:private",
			Value: out,
		})
	}

	return out
}

func (orders *Orders) Observe(socket types.Socket) {
	orders.observers = append(orders.observers, socket)
}
