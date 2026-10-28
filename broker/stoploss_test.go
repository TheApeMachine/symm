package broker

import (
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestVirtualPositionUsesAdaptiveExitWhilePaperKeepsBaseline(t *testing.T) {
	t.Parallel()

	virtual := &Position{
		Symbol:      "TEST/USD",
		Virtual:     true,
		PnL:         decimal.NewFromFloat64(-3),
		PnLPct:      decimal.NewFromFloat64(-3),
		frictionPct: decimal.NewFromFloat64(0.01),
		stopLoss:    NewStopLoss(),
	}
	trigger, exit := virtual.stopLoss.Evaluate(virtual)
	if !exit || !strings.Contains(trigger, "adaptive invalidation") {
		t.Fatalf("virtual position should use adaptive invalidation, got exit=%v trigger=%q", exit, trigger)
	}

	paper := &Position{
		Symbol:      "TEST/USD",
		OpenedAt:    time.Now(),
		Virtual:     false,
		PnL:         decimal.NewFromFloat64(-3),
		PnLPct:      decimal.NewFromFloat64(-3),
		frictionPct: decimal.NewFromFloat64(0.01),
		stopLoss:    NewStopLoss(),
	}
	trigger, exit = paper.stopLoss.Evaluate(paper)
	if exit {
		t.Fatalf("paper position should retain the baseline policy at -3%%, trigger=%q", trigger)
	}
}
