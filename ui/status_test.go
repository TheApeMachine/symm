package ui

import (
	"context"
	"testing"
)

func TestMarketStreamStatus(t *testing.T) {
	hub, err := NewHub(context.Background(), nil)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	stream := NewMarketStream(hub)
	stream.Status(map[string]any{
		"equity_eur":     200,
		"cash_eur":       200,
		"closed_pnl_eur": 0,
		"trade_count":    0,
		"win_rate":       0,
		"open_count":     0,
		"positions":      []map[string]any{},
	})

	replay := hub.replayEvents()
	if len(replay) != 1 {
		t.Fatalf("expected one replay event, got %d", len(replay))
	}

	if replay[0]["event"] != "status" {
		t.Fatalf("expected status replay, got %v", replay[0]["event"])
	}
}

func TestMarketStreamDecisionTrace(t *testing.T) {
	hub, err := NewHub(context.Background(), nil)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	stream := NewMarketStream(hub)
	stream.DecisionTrace(map[string]any{
		"line":        1.1,
		"median":      0.9,
		"mad":         0.2,
		"scored":      1,
		"in_play":     1,
		"allowed":     1,
		"decisions":   []map[string]any{},
		"evaluations": []map[string]any{},
	})

	replay := hub.replayEvents()
	if len(replay) != 1 {
		t.Fatalf("expected one replay event, got %d", len(replay))
	}

	if replay[0]["event"] != "decision_trace" {
		t.Fatalf("expected decision_trace, got %v", replay[0]["event"])
	}
}
