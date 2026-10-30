package signal

import (
	"context"
	"sync"
	"testing"

	"github.com/theapemachine/symm/kraken"
)

func TestTradesBySymbolPreservesOrder(t *testing.T) {
	trades := []kraken.TradeData{
		{Symbol: "BTC/USD", TradeID: 1},
		{Symbol: "ETH/USD", TradeID: 2},
		{Symbol: "BTC/USD", TradeID: 3},
	}

	groups := tradesBySymbol(trades)
	if len(groups) != 2 {
		t.Fatalf("expected 2 symbol groups, got %d", len(groups))
	}
	if len(groups[0]) != 2 || groups[0][0].TradeID != 1 || groups[0][1].TradeID != 3 {
		t.Fatalf("BTC group order wrong: %+v", groups[0])
	}
	if len(groups[1]) != 1 || groups[1][0].TradeID != 2 {
		t.Fatalf("ETH group wrong: %+v", groups[1])
	}
}

func TestRunTradesSerializesSameSymbol(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	active := map[string]int{}
	var races int

	trades := []kraken.TradeData{
		{Symbol: "LIGHTER/USD", TradeID: 1},
		{Symbol: "LIGHTER/USD", TradeID: 2},
		{Symbol: "LIGHTER/USD", TradeID: 3},
	}

	err := RunTrades(ctx, trades, func(trade kraken.TradeData) {
		mu.Lock()
		active[trade.Symbol]++
		if active[trade.Symbol] > 1 {
			races++
		}
		mu.Unlock()

		mu.Lock()
		active[trade.Symbol]--
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("RunTrades: %v", err)
	}
	if races > 0 {
		t.Fatalf("expected no concurrent handlers for one symbol, got %d overlaps", races)
	}
}
