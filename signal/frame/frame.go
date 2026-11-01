package frame

import (
	"sync"

	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

/*
Symbols groups one trade frame by symbol while keeping each symbol's prints
in frame order, then fans symbols across the shared pool.
*/
type Symbols struct {
	groups [][]*market.TradeData
}

/*
NewSymbols builds per-symbol batches from a trade frame.
*/
func NewSymbols(trades []*market.TradeData) *Symbols {
	if len(trades) == 0 {
		return &Symbols{}
	}

	index := make(map[string]int, len(trades))
	groups := make([][]*market.TradeData, 0)

	for _, trade := range trades {
		if trade == nil || trade.Symbol == "" {
			continue
		}

		at, ok := index[trade.Symbol]

		if !ok {
			at = len(groups)
			index[trade.Symbol] = at
			groups = append(groups, nil)
		}

		groups[at] = append(groups[at], trade)
	}

	return &Symbols{groups: groups}
}

/*
Each runs work once per symbol with that symbol's ordered prints.
*/
func (symbols *Symbols) Each(work func(trades []*market.TradeData)) {
	if symbols == nil || len(symbols.groups) == 0 || work == nil {
		return
	}

	var wg sync.WaitGroup

	for _, trades := range symbols.groups {
		trades := trades
		wg.Add(1)

		system.Q.Submit(func() {
			defer wg.Done()

			work(trades)
		})
	}

	wg.Wait()
}
