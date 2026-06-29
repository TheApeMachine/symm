package types

import "github.com/theapemachine/datura"

/*
Socket handles one websocket message type on the internal artifact bus.
*/
type Socket interface {
	Send(artifact *datura.Artifact) *datura.Artifact
	Observe(sockets ...Socket)
}
