package strategy

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
)

func TestMomentumReversalTrigger(t *testing.T) {
	symbol := "RLS/USD"
	position := broker.NewPosition(symbol, "R07/R08", true, nil)
	position.Qty = decimal.NewFromInt64(1)
	position.PnL = decimal.NewFromInt64(1)
	position.PnLPct = decimal.NewFromInt64(4)

	momentum := NewMomentum()
	momentum.tracks.Store(position.ID, &momentumSample{
		seeded:        true,
		lastPnLPct:    7.2,
		lastForward:   0.2,
		lastSupported: 4,
		curveAligned:  true,
	})

	tick := data.NewValue("test", "tick", symbol)
	scope := symbolScope(symbol)
	tick.Append(1.0, "resonance", scope, "calibrated")
	tick.Append(4.0, "resonance", scope, "supported_horizon")
	tick.Append(0.25, "resonance", scope, "forward_curve_0")
	tick.Append(0.2, "resonance", scope, "forward_curve_1")

	resonance := NewResonance()
	trigger, fired := momentum.trigger(resonance, tick, symbol, position)
	if !fired || trigger != "momentum reversal" {
		t.Fatalf("trigger = (%q, %v), want momentum reversal", trigger, fired)
	}
}

func TestVirtualShouldExitSkipsTrailingGiveback(t *testing.T) {
	position := broker.NewPosition("BTC/USD", "R07", true, nil)
	position.PnL = decimal.NewFromInt64(1)
	position.PnLPct = decimal.NewFromFloat64(10)
	position.TickHold()
	position.PnLPct = decimal.NewFromFloat64(6)

	trigger, ok := position.ShouldExit()
	if ok {
		t.Fatalf("virtual ShouldExit = (%q, %v), want no book winner exit", trigger, ok)
	}
}
