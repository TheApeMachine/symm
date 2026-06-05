package private

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

type outboundOrder struct {
	symbol string
	side   trading.Side
}

/*
trackOutbound records add_order params so method rejections can clear trader pending state.
*/
func (websocketClient *WebSocket) trackOutbound(value any) {
	params, ok := addParamsFromOutbound(value)

	if !ok || params.ClOrdID == "" || params.Symbol == "" {
		return
	}

	websocketClient.outboundMu.Lock()
	websocketClient.outbound[params.ClOrdID] = outboundOrder{
		symbol: params.Symbol,
		side:   params.Side,
	}
	websocketClient.outboundMu.Unlock()
}

func addParamsFromOutbound(value any) (trading.AddParams, bool) {
	switch frame := value.(type) {
	case map[string]any:
		if frame["method"] != trading.MethodAddOrder {
			return trading.AddParams{}, false
		}

		params, ok := frame["params"].(trading.AddParams)

		return params, ok
	default:
		return trading.AddParams{}, false
	}
}

/*
handleMethodResponse publishes live Kraken method/result/error frames that carry no channel.
Successful add_order acks are logged on raw; failures emit the same derived rejection
envelope paper uses so trader/crypto clears its in-flight marker.
*/
func (websocketClient *WebSocket) handleMethodResponse(raw json.RawMessage) {
	websocketClient.publishMethodRaw(raw)

	var ack trading.Ack

	if err := sonic.Unmarshal(raw, &ack); err != nil {
		return
	}

	switch ack.Method {
	case trading.MethodAddOrder:
		websocketClient.handleAddOrderAck(ack)
	case trading.MethodCancelOrder, trading.MethodCancelAll:
		if !ack.Success && ack.Error != "" {
			websocketClient.forgetOutboundClOrdIDs(ack.Result.ClOrdID)
		}
	}
}

func (websocketClient *WebSocket) publishMethodRaw(raw json.RawMessage) {
	if websocketClient.raw == nil {
		return
	}

	websocketClient.raw.Send(&qpool.QValue[any]{Value: map[string]any{
		"channel": "order_method",
		"frame":   append(json.RawMessage(nil), raw...),
	}})
}

func (websocketClient *WebSocket) handleAddOrderAck(ack trading.Ack) {
	clOrdID := ack.Result.ClOrdID

	if ack.Success {
		return
	}

	order, ok := websocketClient.lookupOutbound(clOrdID)

	if !ok {
		return
	}

	reason := ack.Error

	if reason == "" {
		reason = "exchange rejected order"
	}

	user.PublishExecutionRejectDerived(
		websocketClient.raw,
		order.symbol,
		string(order.side),
		reason,
	)
	websocketClient.forgetOutbound(clOrdID)
}

func (websocketClient *WebSocket) lookupOutbound(clOrdID string) (outboundOrder, bool) {
	if clOrdID == "" {
		return outboundOrder{}, false
	}

	websocketClient.outboundMu.Lock()
	defer websocketClient.outboundMu.Unlock()

	order, ok := websocketClient.outbound[clOrdID]

	return order, ok
}

func (websocketClient *WebSocket) forgetOutbound(clOrdID string) {
	if clOrdID == "" {
		return
	}

	websocketClient.outboundMu.Lock()
	delete(websocketClient.outbound, clOrdID)
	websocketClient.outboundMu.Unlock()
}

func (websocketClient *WebSocket) forgetOutboundClOrdIDs(clOrdIDs ...string) {
	websocketClient.outboundMu.Lock()
	defer websocketClient.outboundMu.Unlock()

	for _, clOrdID := range clOrdIDs {
		if clOrdID != "" {
			delete(websocketClient.outbound, clOrdID)
		}
	}
}
