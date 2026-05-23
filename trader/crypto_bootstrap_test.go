package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/asset"
)

func TestConnectSnapshotIncludesStatusAndOpenTrades(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	prices := stubPrices{"CLOUD/EUR": 0.0169}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), &stubSignal{})

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	enteredAt := time.Now().Add(-2 * time.Minute)

	crypto.holds["CLOUD/EUR"] = position{
		pair:       asset.Pair{Wsname: "CLOUD/EUR"},
		notional:   10,
		entryPrice: 0.01705,
		entryFee:   0.026,
		enteredAt:  enteredAt,
		confidence: 385.724,
		regime:     "flow",
		reason:     "accumulation",
		trailPct:   0.01,
		stopPrice:  0.016445,
		peakPrice:  0.01705,
	}

	events := crypto.ConnectSnapshot()

	if len(events) != 2 {
		t.Fatalf("expected status + trade_enter, got %d events", len(events))
	}

	status, ok := events[0]["event"].(string)

	if !ok || status != "status" {
		t.Fatalf("expected status event first, got %v", events[0]["event"])
	}

	positions, ok := events[0]["positions"].([]map[string]any)

	if !ok || len(positions) != 1 {
		t.Fatalf("expected one open position in status, got %v", events[0]["positions"])
	}

	if positions[0]["symbol"] != "CLOUD/EUR" {
		t.Fatalf("expected CLOUD/EUR position, got %v", positions[0]["symbol"])
	}

	if events[0]["open_count"] != 1 {
		t.Fatalf("expected open_count 1, got %v", events[0]["open_count"])
	}

	tradeEnter, ok := events[1]["event"].(string)

	if !ok || tradeEnter != "trade_enter" {
		t.Fatalf("expected trade_enter event, got %v", events[1]["event"])
	}

	if events[1]["symbol"] != "CLOUD/EUR" {
		t.Fatalf("expected CLOUD/EUR trade_enter, got %v", events[1]["symbol"])
	}

	if events[1]["regime"] != "flow" {
		t.Fatalf("expected flow regime, got %v", events[1]["regime"])
	}
}

func TestConnectSnapshotStatusOnlyWhenFlat(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{}, NoopPublisher(), &stubSignal{})

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	events := crypto.ConnectSnapshot()

	if len(events) != 1 {
		t.Fatalf("expected status only, got %d events", len(events))
	}

	if events[0]["event"] != "status" {
		t.Fatalf("expected status event, got %v", events[0]["event"])
	}
}
