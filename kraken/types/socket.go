package types

/*
Socket handles one kraken:private message type and returns the raw bus payload.
*/
type Socket interface {
	Send(message []byte) *SocketMessage
	Observe(sockets ...Socket)
}
