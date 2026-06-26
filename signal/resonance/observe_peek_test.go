package resonance

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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
	signal.HydrateMarketFromTree()

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

func TestHydrateMarketFromTreeUsesTimestampCursor(t *testing.T) {
	viper.Set("signals.feed_ring_capacity", 64)

	signal := NewSignal(context.Background(), resonanceTestPool(t), dmt.NewTree(""), nil, 0.02, 8)

	defer func() {
		_ = signal.Close()
	}()

	insertTicker := func(scope string, last float64, stamp time.Time) {
		artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
		artifact.WithRole("ticker")
		artifact.WithScope(scope)
		artifact.WithPayload([]byte(`{"channel":"ticker","data":[{"symbol":"` + scope + `","last":` + strconv.FormatFloat(last, 'f', -1, 64) + `,"volume":1,"change_pct":0.1}]}`))
		artifact.SetTimestamp(stamp.UnixNano())

		signal.tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
	}

	insertTicker("OLD/USD", 1, time.Now().UTC().Add(-2*time.Hour))
	insertTicker("NOW/USD", 42, time.Now().UTC())

	signal.HydrateMarketFromTree()

	if got := signal.ticker.Snapshot("OLD/USD").Last; got != 0 {
		t.Fatalf("hydrated stale cold-start frame: %v", got)
	}

	if got := signal.ticker.Snapshot("NOW/USD").Last; got != 42 {
		t.Fatalf("current frame not hydrated: %v", got)
	}
}
