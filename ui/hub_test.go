package ui

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
)

func TestNewHub(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	hub, err := NewHub(ctx, pool)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	if hub.subscriptions["subscriptions"] == nil {
		t.Fatal("expected ui subscription")
	}
}
