package ui

import (
	"context"
	"testing"
)

func TestListenAddrBindsLocalhostByDefault(t *testing.T) {
	addr, ok := ListenAddr(":8765")

	if !ok || addr != "127.0.0.1:8765" {
		t.Fatalf("expected localhost bind, got ok=%v addr=%q", ok, addr)
	}
}

func TestHubStoresReplayEvents(t *testing.T) {
	ctx := context.Background()
	hub, err := NewHub(ctx, nil)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	fieldEvent := map[string]any{
		"event":        "field_snapshot",
		"symbol_count": 12,
	}
	pulseEvent := map[string]any{
		"event": "engine_pulse",
		"seq":   42,
	}

	hub.Emit(fieldEvent)
	hub.Emit(pulseEvent)
	hub.Emit(map[string]any{"event": "status", "open_count": 1})

	replay := hub.replayEvents()

	if len(replay) != 3 {
		t.Fatalf("expected three replay events, got %d", len(replay))
	}

	if replay[0]["event"] != "field_snapshot" {
		t.Fatalf("expected field_snapshot first, got %v", replay[0]["event"])
	}

	if replay[1]["event"] != "engine_pulse" {
		t.Fatalf("expected engine_pulse second, got %v", replay[1]["event"])
	}

	if replay[2]["event"] != "status" {
		t.Fatalf("expected status third, got %v", replay[2]["event"])
	}
}

func TestHubBootstrapReturnsMultipleEvents(t *testing.T) {
	ctx := context.Background()

	hub, err := NewHub(ctx, func() []map[string]any {
		return []map[string]any{
			{"event": "status", "open_count": 1},
			{"event": "trade_enter", "symbol": "CLOUD/EUR"},
		}
	})

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	events := hub.bootstrap()

	if len(events) != 2 {
		t.Fatalf("expected two bootstrap events, got %d", len(events))
	}
}
