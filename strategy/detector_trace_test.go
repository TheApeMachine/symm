package strategy

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
)

func TestTraceFlushKeepsStackedMarks(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, nil)
	price.SetReferenceCash(decimal.NewFromFloat64(200))
	detector := NewDetector(context.Background(), price)
	detector.label = "ES/USD"
	detector.EnableTrace()

	detector.Flush(
		"up",
		103050, 103050,
		10, 40,
		decimal.NewFromFloat64(1.25), decimal.NewFromFloat64(1.31),
		nil, nil,
	)

	path := t.TempDir() + "/trace.json"
	if err := detector.WriteTrace(path); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var body struct {
		Decisions []tapeDecision `json:"decisions"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(body.Decisions) != 1 {
		t.Fatalf("decisions = %d", len(body.Decisions))
	}

	got := body.Decisions[0]
	if got.Action != "flush" || got.ATick != 103050 || got.BTick != 103050 || got.CTick != 103050 || got.DTick != 103050 {
		t.Fatalf("marks = %+v", got)
	}
	if got.AIdx == got.DIdx {
		t.Fatalf("seq marks collapsed with the ticks: %+v", got)
	}
}
