package paper

import (
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

func rejectedExecution(clOrdID, reason string) public.SocketMessage {
	payload, err := sonic.Marshal([]user.Execution{{
		ClOrdID:     clOrdID,
		ExecType:    "rejected",
		OrderStatus: reason,
	}})

	if err != nil {
		return public.SocketMessage{}
	}

	return public.SocketMessage{
		Channel: public.ExecutionsChannel,
		Type:    "update",
		Data:    payload,
	}
}

func (ws *WebSocket) publishOrderAck(
	message *qpool.QValue[any], out public.SocketMessage,
) {
	if message == nil || message.Type != public.OrdersChannel {
		return
	}

	if out.Channel != "" {
		trading.PublishLedgerAck(ws.pool, out)

		return
	}

	clOrdID := clOrdIDFromOrder(message.Value)

	if clOrdID == "" {
		return
	}

	trading.PublishLedgerAck(ws.pool, rejectedExecution(clOrdID, "paper rejected"))
}
