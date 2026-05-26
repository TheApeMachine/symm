package leadlag

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestLeadLagMeasure(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewLeadLag(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	signal.symbols[anchorSymbol] = &symbolState{
		pair:      asset.Pair{Wsname: anchorSymbol},
		changePct: 0.08,
	}
	signal.symbols["ALT/EUR"] = &symbolState{
		pair:      asset.Pair{Wsname: "ALT/EUR"},
		changePct: 0.02,
	}

	found := false

	for measurement := range signal.Measure() {
		found = true

		if measurement.Source != leadlagSource || measurement.Confidence <= 0 ||
			measurement.Confidence > 1 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	}

	if !found {
		t.Fatal("expected lag measurement")
	}
}

func BenchmarkLeadLagMeasure(b *testing.B) {
	signal := NewLeadLag(context.Background(), nil)
	signal.symbols[anchorSymbol] = &symbolState{changePct: 0.08}
	signal.symbols["ALT/EUR"] = &symbolState{changePct: 0.02}
	signal.symbols["ETH/EUR"] = &symbolState{changePct: 0.03}

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
