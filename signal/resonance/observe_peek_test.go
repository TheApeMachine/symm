package resonance

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
)

func TestObserveBookSpreadFromTree(t *testing.T) {
	viper.Set("signals.feed_ring_capacity", 64)

	signal := NewSignal(context.Background(), resonanceTestPool(t), dmt.NewTree(""), nil, 0.02, 8)

	defer func() {
		_ = signal.Close()
	}()

	scope := "FLOW/EUR"
	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	seedMarketFixture(signal, scope, 1, 1, -2, 0.001, observedAt)
	signal.hydrateMarketFromTree()

	spread := signal.book.Spread(scope)

	if spread <= 0 {
		t.Fatalf("expected positive spread from tree hydration, got %v", spread)
	}

	window, ok := signal.book.Window(scope)

	if !ok || len(window.Spreads) == 0 {
		t.Fatal("book window spreads missing")
	}

	if window.Spreads[len(window.Spreads)-1] <= 0 {
		t.Fatalf("expected bps spread history, got %v", window.Spreads)
	}

	features, ok := signal.featureContext(scope)

	if !ok || features.spread <= 0 {
		t.Fatalf("feature spread unavailable: ok=%v spread=%v", ok, features.spread)
	}
}
