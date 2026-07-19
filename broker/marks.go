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

	prior := holding.Mark
	_ = position.price.Mark(position.pair, holding)

	if holding.Mark != nil {
		position.ObserveMark(holding.Mark.Float64())
	}

	if prior != nil && holding.Mark != nil && prior.Cmp(holding.Mark) == 0 {
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
PendingSymbols returns symbols with outstanding entry/exit/reduce intents.
*/
func (marks *Marks) PendingSymbols() map[string]string {
	pending := map[string]string{}

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

		pending[symbol] = "pending"

		return true
	})

	return pending
}
