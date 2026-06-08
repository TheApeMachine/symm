package types

import (
	"github.com/theapemachine/qpool"
)

/*
Socket handles one kraken:private message type and returns the raw bus payload.
*/
type Socket interface {
	Send(message *qpool.QValue[any]) *SocketMessage
	Observe(sockets ...Socket)
}
