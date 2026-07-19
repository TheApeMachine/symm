package broker

import (
	"sync"

	"github.com/theapemachine/symm/types"
)

/*
Marks fans a single Price ticker decode onto open Position lots and reports
pending-order symbols for recovery snapshots.
*/
type Marks struct {
	positions *sync.Map
}

/*
Apply refreshes the Holding mark for symbol when a lot is open.
*/
func (marks *Marks) Apply(symbol string) {
	if marks == nil || marks.positions == nil || symbol == "" {
		return
	}

	value, ok := marks.positions.Load(symbol)

	if !ok {
		return
	}

	position, ok := value.(*Position)

	if !ok || position == nil ||
		position.balance == nil || position.pair == nil || position.price == nil {
		return
	}

	holdingValue, ok := position.balance.holdings.Load(position.pair.Symbol)

	if !ok {
		return
	}

	holding, ok := holdingValue.(*types.Holding)

	if !ok || holding == nil || holding.Status == types.CLOSED {
		return
	}

	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return
	}

	priorBid := holding.Mark
	priorStop := holding.StopMark
	_ = position.price.Mark(position.pair, holding)

	if holding.StopMark != nil {
		position.ObserveMark(holding.StopMark.Float64())
	} else if holding.Mark != nil {
		position.ObserveMark(holding.Mark.Float64())
	}

	bidSame := priorBid != nil && holding.Mark != nil && priorBid.Cmp(holding.Mark) == 0
	stopSame := priorStop != nil && holding.StopMark != nil &&
		priorStop.Cmp(holding.StopMark) == 0

	if bidSame && stopSame {
		return
	}

	position.balance.Publish()
}

/*
Pending reports whether symbol has an outstanding broker order intent.
*/
func (marks *Marks) Pending(position *Position) bool {
	return position != nil && position.Pending()
}

/*
PendingSymbols returns outstanding entry/exit/reduce intents with order
identity for compact recovery snapshots.
*/
func (marks *Marks) PendingSymbols() map[string]types.PendingOrderWire {
	pending := map[string]types.PendingOrderWire{}

	if marks == nil || marks.positions == nil {
		return pending
	}

	marks.positions.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		position, ok := value.(*Position)

		if !ok || position == nil || !position.Pending() {
			return true
		}

		pending[symbol] = position.PendingWire()

		return true
	})

	return pending
}
