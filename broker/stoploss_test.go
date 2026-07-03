package broker

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

func TestStoplossUsesConfiguredTrailingOffset(t *testing.T) {
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Cleanup(func() {
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	order := datura.Acquire("test", datura.APPJSON).
		WithScope("MATIC/USD").
		Poke(100.0, "last_price")
	t.Cleanup(order.Release)

	stoploss := NewStoploss(order, "MATIC/USD")
	if stoploss == nil {
		t.Fatal("expected stoploss")
	}

	if stop := datura.Peek[float64](order, "stoploss", "stop"); stop != 99.0 {
		t.Fatalf("stop = %v, want 99", stop)
	}
	if offset := datura.Peek[float64](order, "stoploss", "offset"); offset != 0.01 {
		t.Fatalf("offset = %v, want 0.01", offset)
	}

	stoploss.Ratchet(105.0)
	if stop := datura.Peek[float64](order, "stoploss", "stop"); stop != 103.95 {
		t.Fatalf("ratcheted stop = %v, want 103.95", stop)
	}
	if stoploss.State != ARMED {
		t.Fatalf("state = %v, want ARMED", stoploss.State)
	}

	stoploss.Ratchet(103.0)
	if stoploss.State != TRIGGERED {
		t.Fatalf("state = %v, want TRIGGERED", stoploss.State)
	}
	if state := datura.Peek[float64](order, "stoploss", "trigger"); state != 103.0 {
		t.Fatalf("trigger = %v, want 103", state)
	}
	if mark := datura.Peek[float64](order, "stoploss", "recent_marks", 0); mark <= 0 {
		t.Fatal("recent marks were not published")
	}
}

func TestStoplossRatchetPreservesExitLifecycleState(t *testing.T) {
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Cleanup(func() {
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	order := datura.Acquire("test", datura.APPJSON).
		WithScope("MATIC/USD").
		Poke(100.0, "last_price")
	t.Cleanup(order.Release)

	stoploss := NewStoploss(order, "MATIC/USD")
	if stoploss == nil {
		t.Fatal("expected stoploss")
	}

	state := stoplossState(order)
	state["state"] = int(EXIT_SUBMITTED)
	state["state_label"] = stoplossStateLabel(EXIT_SUBMITTED)
	writeStoplossState(order, state)
	stoploss.State = EXIT_SUBMITTED

	stoploss.Ratchet(98.0)
	state = stoplossState(order)
	if stoploss.State != EXIT_SUBMITTED {
		t.Fatalf("state = %v, want EXIT_SUBMITTED", stoploss.State)
	}
	if got, ok := state["state"].(float64); !ok || int(got) != int(EXIT_SUBMITTED) {
		t.Fatalf("persisted state = %v, want EXIT_SUBMITTED", got)
	}
	if _, exists := state["trigger"]; exists {
		t.Fatal("ratchet should not write a new trigger after exit submission")
	}
}

func TestWriteStoplossStatePreservesInvalidAttributes(t *testing.T) {
	order := datura.Acquire("test", datura.APPJSON)
	t.Cleanup(order.Release)

	if err := order.SetAttributes([]byte(`{"stoploss":`)); err != nil {
		t.Fatal(err)
	}

	writeStoplossState(order, map[string]any{
		"state": int(ARMED),
	})

	attributes, err := order.Attributes()
	if err != nil {
		t.Fatal(err)
	}
	if string(attributes) != `{"stoploss":` {
		t.Fatalf("attributes = %q, want invalid original preserved", string(attributes))
	}
}
