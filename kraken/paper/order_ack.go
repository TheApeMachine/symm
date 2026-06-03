package paper

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

func clOrdIDFromOrder(value any) string {
	frame := paramsMap(value)

	if frame == nil {
		return ""
	}

	params, ok := addParamsFromAny(frame["params"])

	if !ok {
		return ""
	}

	return params.ClOrdID
}

func rejectedExecution(clOrdID, reason string) map[string]any {
	payload, err := sonic.Marshal([]user.Execution{{
		ClOrdID:     clOrdID,
		ExecType:    "rejected",
		OrderStatus: reason,
	}})

	if err != nil {
		return nil
	}

	return map[string]any{
		"channel": public.ExecutionsChannel,
		"type":    "update",
		"data":    json.RawMessage(payload),
	}
}

func (ws *WebSocket) publishOrderAck(
	message *qpool.QValue[any], out map[string]any,
) {
	if message == nil || message.Type != public.OrdersChannel {
		return
	}

	channel, _ := out["channel"].(string)

	if channel != "" {
		trading.PublishLedgerAck(ws.pool, out)

		return
	}

	clOrdID := clOrdIDFromOrder(message.Value)

	if clOrdID == "" {
		return
	}

	trading.PublishLedgerAck(ws.pool, rejectedExecution(clOrdID, "paper rejected"))
}
