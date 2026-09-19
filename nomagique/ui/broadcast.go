package ui

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Broadcast provides an execution offramp that broadcasts incoming frames or evaluations
via the configured WebSocketServer, WebRTCServer, or downstream consumers.
No hub application hack, pure Value closure.
*/
type Broadcast types.Value[any, any]

func NewBroadcast(server types.Value[any, any]) Broadcast {
	return func(in any) any {
		if server != nil && in != nil {
			if res := server(in); res != nil {
				return res
			}
		}
		return in
	}
}
