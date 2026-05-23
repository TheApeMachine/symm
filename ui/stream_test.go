package ui

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/fluid"
)

func TestMarketStreamPriceTickNonBlocking(t *testing.T) {
	hub, err := NewHub(context.Background(), nil)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	stream := NewMarketStream(hub)

	for index := 0; index < 512; index++ {
		stream.PriceTick("BTC/EUR", 50000, 49999, 50001, 1.2, "")
	}
}

func TestMarketStreamFieldUpdate(t *testing.T) {
	hub, err := NewHub(context.Background(), nil)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	stream := NewMarketStream(hub)
	stream.FieldUpdate(fluid.FieldSnapshot{
		SymbolCount: 1,
		Field: fluid.FieldAggregate{
			Re: 1.5,
		},
		Symbols: []fluid.SymbolSnapshot{{
			Symbol: "BTC/EUR",
			Re:     1.5,
		}},
	})
}

func BenchmarkMarketStreamPriceTick(b *testing.B) {
	hub, err := NewHub(context.Background(), nil)

	if err != nil {
		b.Fatal(err)
	}

	stream := NewMarketStream(hub)
	b.ReportAllocs()

	for b.Loop() {
		stream.PriceTick("BTC/EUR", 50000, 49999, 50001, 1.2, time.Now().UTC().Format(time.RFC3339Nano))
	}
}
