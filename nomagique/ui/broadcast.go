package ui

import (
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Broadcast provides an execution offramp that feeds cognitive evaluations into the Hub.
No structs, pure Value closure.
*/
type Broadcast types.Value[any, any]

func NewBroadcast(hub *Hub) Broadcast {
	return func(in any) any {
		if hub == nil || in == nil {
			return in
		}

		if eval, ok := in.(cognition.Evaluation); ok {
			hub.BroadcastEvaluation(eval)
		}

		return in
	}
}
