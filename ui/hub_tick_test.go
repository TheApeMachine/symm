package ui

import (
	"context"
	"testing"
	"time"
)

func TestHubCoalescesPriceTicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub, err := NewHub(ctx, nil)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	client := &wsClient{
		send: make(chan []byte, 4),
	}
	client.subscribe([]string{"BTC/EUR"})
	hub.clients.Store(client, struct{}{})

	hub.coalescePriceTick("BTC/EUR", []byte(`{"event":"price_tick","symbol":"BTC/EUR","last":1}`))
	hub.coalescePriceTick("BTC/EUR", []byte(`{"event":"price_tick","symbol":"BTC/EUR","last":2}`))
	hub.coalescePriceTick("ETH/EUR", []byte(`{"event":"price_tick","symbol":"ETH/EUR","last":3}`))

	hub.flushPendingTicks()

	select {
	case payload := <-client.send:
		if string(payload) != `{"event":"price_tick","symbol":"BTC/EUR","last":2}` {
			t.Fatalf("expected latest coalesced tick, got %s", payload)
		}
	default:
		t.Fatal("expected one coalesced price tick")
	}

	select {
	case payload := <-client.send:
		t.Fatalf("expected only one coalesced tick, got %s", payload)
	default:
	}
}

func TestHubPriceTickFlushInterval(t *testing.T) {
	if priceTickFlushEvery != 50*time.Millisecond {
		t.Fatalf("expected 20Hz tick flush, got %v", priceTickFlushEvery)
	}
}

func BenchmarkHubCoalescePriceTick(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub, err := NewHub(ctx, nil)

	if err != nil {
		b.Fatalf("new hub: %v", err)
	}

	client := &wsClient{send: make(chan []byte, 256)}
	client.subscribe([]string{"BTC/EUR"})
	hub.clients.Store(client, struct{}{})

	payload := []byte(`{"event":"price_tick","symbol":"BTC/EUR","last":1}`)

	b.ReportAllocs()

	for b.Loop() {
		hub.coalescePriceTick("BTC/EUR", payload)
	}
}
