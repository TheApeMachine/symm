package types

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestStoplossManualOverrideUsesRegulatorBoundary(t *testing.T) {
	mark := decimal.NewFromFloat64(101.25)
	stoploss := &Stoploss{Status: ARMED, Mark: mark}

	if err := stoploss.TriggerManualOverride(); err != nil {
		t.Fatalf("manual override failed: %v", err)
	}

	if stoploss.Status != TRIGGERED {
		t.Fatalf("manual override should trigger regulator, got %s", stoploss.Status)
	}

	if stoploss.TriggerReason != TriggerManualOverride {
		t.Fatalf("unexpected trigger reason %q", stoploss.TriggerReason)
	}

	if stoploss.TriggerMark == nil || stoploss.TriggerMark.Cmp(mark) != 0 {
		t.Fatal("manual override should retain the current executable mark")
	}
}
