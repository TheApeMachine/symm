package paper

import (
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func (ws *WebSocket) scheduleExchangeDelivery(delivery func()) {
	if delivery == nil {
		return
	}

	roundTrip := public.SharedNetworkLatency().ExchangeRoundTrip()

	if roundTrip <= 0 {
		delivery()

		return
	}

	time.AfterFunc(roundTrip, delivery)
}

func (ws *WebSocket) deliverPrivateResponse(
	message *qpool.QValue[any],
	out public.SocketMessage,
) {
	if out.Channel == "" {
		ws.publishOrderAck(message, out)

		return
	}

	activate.Once("kraken/paper:channel:" + out.Channel)

	if out.Channel == public.BalancesChannel {
		trading.MarkDeskReady()
	}

	if channel := ws.broadcasts["raw"]; channel != nil {
		channel.Send(&qpool.QValue[any]{
			Type:  out.Channel,
			Value: out,
		})
	}

	ws.publishOrderAck(message, out)
}

func (ws *WebSocket) deliverExecution(out public.SocketMessage) {
	if out.Channel == "" {
		return
	}

	activate.Once("kraken/paper:channel:" + out.Channel)

	ws.scheduleExchangeDelivery(func() {
		if channel := ws.broadcasts["raw"]; channel != nil {
			channel.Send(&qpool.QValue[any]{
				Type:  out.Channel,
				Value: out,
			})
		}
	})
}
