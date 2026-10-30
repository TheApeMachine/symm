package signal

import (
	"context"
	"runtime"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"golang.org/x/sync/errgroup"
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
	errnie.Debug("[trades] DistinctSymbols", "trades", trades)

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

// RunTrades runs fn concurrently over trades with WorkerLimit fan-out.
func RunTrades(ctx context.Context, trades []kraken.TradeData, fn func(kraken.TradeData)) error {
	errnie.Debug("[trades] RunTrades", "trades", trades)

	if len(trades) == 0 {
		return nil
	}

	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(runtime.GOMAXPROCS(0))

	for i := range trades {
		trade := trades[i]

		group.Go(func() error {
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}

			fn(trade)
			return nil
		})
	}

	return group.Wait()
}

// RunSymbols runs fn concurrently over symbols with WorkerLimit fan-out.
func RunSymbols(ctx context.Context, symbols []string, fn func(string)) error {
	errnie.Debug("[trades] RunSymbols", "symbols", symbols)

	if len(symbols) == 0 {
		return nil
	}

	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(runtime.GOMAXPROCS(0))

	for i := range symbols {
		symbol := symbols[i]

		group.Go(func() error {
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}

			fn(symbol)
			return nil
		})
	}

	return group.Wait()
}
