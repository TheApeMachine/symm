package broker

import (
	"github.com/theapemachine/symm/types"
)

/*
bind registers the single fan-out handlers used by every live Position. Per
position On registration was unbounded and raced under concurrent Buy.
*/
func (desk *Desk) bind() {
	if desk.api == nil || !desk.bound.CompareAndSwap(false, true) {
		return
	}

	desk.api.On("add_order", desk.orderAck)
	desk.api.On("executions", desk.executionAck)
}

func (desk *Desk) orderAck(buf []byte) {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)
		position.OrderAck(buf)

		if position.Status() == types.ERROR {
			desk.evict(position.pair.Symbol)
		}

		return true
	})
}

func (desk *Desk) executionAck(buf []byte) {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)
		position.ExecutionAck(buf)
		status := position.Status()

		if status == types.CLOSED || status == types.ERROR {
			desk.evict(position.pair.Symbol)
		}

		return true
	})
}

func (desk *Desk) evict(symbol string) {
	if symbol == "" {
		return
	}

	desk.positions.Delete(symbol)
}
