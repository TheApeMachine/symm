package signal

import (
	"context"
	"runtime"
	"sync"

	"github.com/theapemachine/symm/kraken"
)

// LastTradePerSymbol keeps one print per pair (last in frame order).
func LastTradePerSymbol(trades []kraken.TradeData) []kraken.TradeData {
	if len(trades) == 0 {
		return nil
	}

	latest := make(map[string]kraken.TradeData, len(trades))
	order := make([]string, 0, len(trades))

	for _, trade := range trades {
		if _, seen := latest[trade.Symbol]; !seen {
			order = append(order, trade.Symbol)
		}

		latest[trade.Symbol] = trade
	}

	out := make([]kraken.TradeData, 0, len(order))

	for _, symbol := range order {
		out = append(out, latest[symbol])
	}

	return out
}

// DistinctSymbols lists each pair once in first-seen order.
func DistinctSymbols(trades []kraken.TradeData) []string {
	if len(trades) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(trades))
	order := make([]string, 0, len(trades))

	for _, trade := range trades {
		if _, ok := seen[trade.Symbol]; ok {
			continue
		}

		seen[trade.Symbol] = struct{}{}
		order = append(order, trade.Symbol)
	}

	return order
}

/*
tradesBySymbol groups prints in frame order while preserving first-seen
symbol order. RunTrades parallelizes across symbols only so per-symbol
state (volume clocks, Hawkes chains) is never updated concurrently.
*/
func tradesBySymbol(trades []kraken.TradeData) [][]kraken.TradeData {
	if len(trades) == 0 {
		return nil
	}

	index := make(map[string]int, len(trades))
	groups := make([][]kraken.TradeData, 0, len(trades))

	for _, trade := range trades {
		i, ok := index[trade.Symbol]
		if !ok {
			i = len(groups)
			index[trade.Symbol] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], trade)
	}

	return groups
}

func parallelFor(ctx context.Context, count int, fn func(int)) error {
	if count == 0 {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if count == 1 {
		fn(0)
		return ctx.Err()
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > count {
		workers = count
	}

	chunk := (count + workers - 1) / workers
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		start := w * chunk
		end := start + chunk
		if end > count {
			end = count
		}

		if start >= end {
			break
		}

		wg.Add(1)

		go func(start, end int) {
			defer wg.Done()

			for i := start; i < end; i++ {
				if ctx.Err() != nil {
					return
				}

				fn(i)
			}
		}(start, end)
	}

	wg.Wait()
	return ctx.Err()
}

// RunTrades runs fn over every print; symbols are processed in parallel,
// prints for one symbol run sequentially in frame order.
func RunTrades(ctx context.Context, trades []kraken.TradeData, fn func(kraken.TradeData)) error {
	if len(trades) == 0 {
		return nil
	}

	groups := tradesBySymbol(trades)

	return parallelFor(ctx, len(groups), func(i int) {
		for _, trade := range groups[i] {
			fn(trade)
		}
	})
}

// RunSymbols runs fn concurrently over symbols with WorkerLimit fan-out.
func RunSymbols(ctx context.Context, symbols []string, fn func(string)) error {
	if len(symbols) == 0 {
		return nil
	}

	return parallelFor(ctx, len(symbols), func(i int) {
		fn(symbols[i])
	})
}

// PruneSyncMap drops keys not present in keep. An empty keep is a no-op.
func PruneSyncMap(m *sync.Map, keep map[string]struct{}) {
	if m == nil || len(keep) == 0 {
		return
	}

	m.Range(func(key, _ any) bool {
		symbol, ok := key.(string)
		if !ok {
			return true
		}

		if _, retain := keep[symbol]; !retain {
			m.Delete(key)
		}

		return true
	})
}
