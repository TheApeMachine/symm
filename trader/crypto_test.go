package trader

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestCryptoBuffersMeasurement(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, wallet)

	if crypto == nil {
		t.Fatal("expected crypto trader")
	}

	crypto.broadcasts["measurements"].Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
			Confidence: 0.8,
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(crypto.measurements) != 1 {
		t.Fatalf("expected one buffered measurement, got %d", len(crypto.measurements))
	}
}

func TestCryptoCloseCancelsContext(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26))

	if err := crypto.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
