package paper

import (
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func (ws *WebSocket) scheduleExchangeDelivery(delivery func()) {
	if delivery == nil {
		return
	}

	delivery()
}

func (ws *WebSocket) broadcastExecution(out map[string]any) {
	channel, _ := out["channel"].(string)

	if channel == "" {
		return
	}

	activate.Once("kraken/paper:channel:" + channel)

	if raw := ws.broadcasts["raw"]; raw != nil {
		raw.Send(&qpool.QValue[any]{
			Type:  channel,
			Value: out,
		})
	}
}

func (ws *WebSocket) deliverPrivateResponse(
	message *qpool.QValue[any],
	out map[string]any,
) {
	channel, _ := out["channel"].(string)

	if channel == "" {
		ws.publishOrderAck(message, out)

		return
	}

	activate.Once("kraken/paper:channel:" + channel)

	if channel == public.BalancesChannel {
		trading.MarkDeskReady()
	}

	if raw := ws.broadcasts["raw"]; raw != nil {
		raw.Send(&qpool.QValue[any]{
			Type:  channel,
			Value: out,
		})
	}

	ws.publishOrderAck(message, out)
}

func (ws *WebSocket) deliverExecution(out map[string]any) {
	channel, _ := out["channel"].(string)

	if channel == "" {
		return
	}

	ws.scheduleExchangeDelivery(func() {
		ws.broadcastExecution(out)
	})
}
