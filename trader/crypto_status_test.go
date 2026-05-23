package trader

import (
	"sync"
	"testing"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type capturePublisher struct {
	mu     sync.Mutex
	events []map[string]any
}

func (publisher *capturePublisher) Emit(event map[string]any) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()

	publisher.events = append(publisher.events, event)
}

func (publisher *capturePublisher) lastEvent(name string) map[string]any {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()

	for index := len(publisher.events) - 1; index >= 0; index-- {
		if publisher.events[index]["event"] == name {
			return publisher.events[index]
		}
	}

	return nil
}

func TestPublishEnginePulse(t *testing.T) {
	publisher := &capturePublisher{}
	crypto := &Crypto{
		publisher: publisher,
		wallet:    &Wallet{Balance: 200},
		holds:     make(map[string]position),
	}

	crypto.SetEngineStats(NewEngineStats(
		func() int { return 10 },
		func() int { return 12 },
		func() int { return 3 },
		func() int { return 5 },
	))

	batch := []engine.Measurement{{
		Type:       engine.Pump,
		Source:     "pumpdump",
		Regime:     "pump",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "ETH/EUR"}},
		Confidence: 0.8,
	}}

	crypto.publishEnginePulse(batch, nil)

	event := publisher.lastEvent("engine_pulse")
	if event == nil {
		t.Fatal("expected engine_pulse event")
	}

	if event["ticker_ready"] != 10 {
		t.Fatalf("ticker_ready: %v", event["ticker_ready"])
	}

	if event["fluid_warming"] != 5 {
		t.Fatalf("fluid_warming: %v", event["fluid_warming"])
	}

	signals, ok := event["signals"].([]map[string]any)
	if !ok || len(signals) != 1 {
		t.Fatalf("signals: %v", event["signals"])
	}
}
