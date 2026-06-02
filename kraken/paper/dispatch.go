package paper

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
dispatchPrivate routes one kraken:private frame through the paper socket model.
*/
func (ws *WebSocket) dispatchPrivate(message *qpool.QValue[any]) {
	if message == nil {
		errnie.Debug("kraken.paper.websocket.dispatchPrivate", "nil message")

		return
	}

	socket, registered := ws.sockets[message.Type]

	if !registered {
		errnie.Debug(
			"kraken.paper.websocket.dispatchPrivate",
			"unregistered message type",
			message.Type,
		)

		return
	}

	out := socket.Send(message)

	if out.Channel == "" {
		ws.publishOrderAck(message, out)

		return
	}

	activate.Once("kraken/paper:channel:" + out.Channel)

	if out.Channel == public.BalancesChannel {
		trading.MarkDeskReady()
	}

	if ch := ws.broadcasts["raw"]; ch != nil {
		ch.Send(&qpool.QValue[any]{
			Type:  out.Channel,
			Value: out,
		})
	}

	ws.publishOrderAck(message, out)
}

func (ws *WebSocket) runPrivate() {
	for {
		select {
		case <-ws.ctx.Done():
			return
		case message, ok := <-ws.subscribers["kraken:private"].Incoming:
			if !ok {
				errnie.Debug("kraken.paper.websocket.runPrivate", "no ok")

				return
			}

			ws.dispatchPrivate(message)
		}
	}
}
