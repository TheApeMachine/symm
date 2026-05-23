package ticker

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/theapemachine/symm/kraken/market"
)

func TestOnQuote(t *testing.T) {
	ticker := &Ticker{
		ctx:    context.Background(),
		quotes: make(map[string]quoteRow),
		ready:  make(map[string]bool),
	}

	var count atomic.Int32

	ticker.OnQuote(func(symbol string, last, bid, ask, changePct float64, timestamp string) {
		if symbol != "BTC/EUR" || last != 50000 {
			t.Fatalf("unexpected quote: %s %f", symbol, last)
		}

		count.Add(1)
	})

	ticker.store([]market.TickerRow{{
		Symbol:     "BTC/EUR",
		Last:       50000,
		Bid:        49999,
		Ask:        50001,
		ChangePct:  1.2,
		Volume:     100,
		Timestamp:  "2026-05-23T12:00:00Z",
	}})

	if count.Load() != 1 {
		t.Fatalf("expected one quote callback, got %d", count.Load())
	}

	if ticker.ReadyCount() != 1 {
		t.Fatalf("expected ready count 1, got %d", ticker.ReadyCount())
	}
}
